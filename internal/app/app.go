package app

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kazuki-oshino/prjev/internal/config"
	"github.com/kazuki-oshino/prjev/internal/diff"
	"github.com/kazuki-oshino/prjev/internal/github"
	"github.com/kazuki-oshino/prjev/internal/jev"
	"github.com/kazuki-oshino/prjev/internal/model"
	"github.com/kazuki-oshino/prjev/internal/rules"
)

type Evidence struct {
	ReviewPolicyVersion int              `json:"review_policy_version,omitempty"`
	Budgets             map[string]int64 `json:"budgets"`
	PR                  model.PR         `json:"pr"`
	Files               []model.File     `json:"files"`
	Units               []model.Unit     `json:"units"`
	Rules               []model.Rule     `json:"rules"`
	Outcomes            []jev.Outcome    `json:"outcomes"`
	Skipped             []string         `json:"skipped"`
	Metrics             model.Metrics    `json:"metrics"`
	Warnings            []string         `json:"warnings"`
	BodyOmitted         bool             `json:"body_omitted"`
}

func Scan(ctx context.Context, ref github.Ref, c config.Config, reader github.Reader, client *jev.Client, notice func(string)) (Evidence, error) {
	start := time.Now()
	e := Evidence{ReviewPolicyVersion: 2, Rules: c.Rules, Outcomes: []jev.Outcome{}, Skipped: []string{}, Warnings: []string{}, Budgets: map[string]int64{
		"max_files": 100, "gh_stdout_bytes": 8 << 20, "unit_patch_bytes": 12 << 10, "state_bytes": 24 << 10, "request_bytes": 64 << 10, "questions_per_request": 128, "max_batches": 12, "max_http_attempts": 24, "concurrency": int64(c.Concurrency), "request_timeout_ms": c.RequestTimeout.Milliseconds(),
	}}
	if deadline, ok := ctx.Deadline(); ok {
		e.Budgets["total_timeout_ms"] = time.Until(deadline).Milliseconds()
	}
	ghStart := time.Now()
	pr, raw, err := reader.Read(ctx, ref)
	e.Metrics.GitHubMS = time.Since(ghStart).Milliseconds()
	if err != nil {
		if pr.Number == 0 || strings.Contains(err.Error(), "pr_changed_during_fetch") || errors.Is(ctx.Err(), context.Canceled) {
			return e, err
		}
		e.PR = pr
		e.Warnings = append(e.Warnings, "GitHubの変更ファイル取得が完了しませんでした。メタデータのみ表示します")
		e.Metrics.TotalMS = time.Since(start).Milliseconds()
		return e, nil
	}
	e.PR = pr
	if pr.ChangedFiles > 100 {
		e.Warnings = append(e.Warnings, "PR変更ファイル数が100を超えたため差分を取得せず、メタデータのみ表示します")
		e.Metrics.TotalMS = time.Since(start).Milliseconds()
		return e, nil
	}
	if len([]byte(pr.Title+pr.Body)) > 8<<10 {
		e.PR.Body = ""
		e.BodyOmitted = true
		e.Warnings = append(e.Warnings, "PRのtitle/bodyが8 KiBを超えたためbodyを送信しません")
	}
	e.Files, e.Units = diff.Normalize(raw)
	batches, skipped := jev.Plan(e.PR, e.Units, c.Rules, c.Model)
	for _, u := range skipped {
		e.Skipped = append(e.Skipped, u.ID)
	}
	if len(batches) == 0 && len(e.Units) > 0 {
		e.Warnings = append(e.Warnings, "解析予算を超えたためJev照会を行いません")
	}
	if len(batches) > 0 {
		notice(fmt.Sprintf("TypeSafe Jev (%s) へPR本文と検査済みテキスト差分、確認観点を送信します。%d単位・%dバッチ", "api.typesafe.ai", len(e.Units)-len(skipped), len(batches)))
		jevStart := time.Now()
		e.Outcomes = client.Run(ctx, batches, c.Concurrency)
		e.Metrics.JevMS = time.Since(jevStart).Milliseconds()
		e.Metrics.HTTPAttempts = int(client.Attempts.Load())
	}
	e.Metrics.TotalMS = time.Since(start).Milliseconds()
	e.Metrics.LocalMS = max(0, e.Metrics.TotalMS-e.Metrics.GitHubMS-e.Metrics.JevMS)
	return e, nil
}
func Aggregate(e Evidence, requestedModel string) model.Result {
	r := model.Result{SchemaVersion: 1, ToolVersion: model.Version, PR: e.PR, RulesHash: rules.Hash(e.Rules), Warnings: append([]string{}, e.Warnings...)}
	r.Model.Requested = requestedModel
	r.ReviewPolicy = model.ReviewPolicy{Version: 1, SkipConfidenceAt: skipConfidenceAt}
	if e.ReviewPolicyVersion == 2 {
		r.ReviewPolicy = model.ReviewPolicy{Version: 2, SkipConfidenceAt: skipConfidenceAt, RequiredConfidenceAt: requiredConfidenceAt, RiskSuggestAt: rules.RiskSuggestAt, RiskCandidateAt: rules.RiskCandidateAt}
	}
	r.Model.Actual = []string{}
	r.Scope = model.Scope{FetchedFiles: len(e.Files), AnalysisUnits: len(e.Units), Unanalyzed: []model.Unanalyzed{}, BodyOmitted: e.BodyOmitted}
	r.Metrics = e.Metrics
	r.Checklist = []model.ChecklistItem{}
	r.Files = []model.FileResult{}
	for _, rule := range e.Rules {
		r.ThresholdProfile = append(r.ThresholdProfile, model.RuleThreshold{ID: rule.ID, SuggestAt: rule.SuggestAt, CandidateAt: rule.CandidateAt})
	}
	values := map[string]float64{}
	choices := map[string]model.ChoiceAnswer{}
	reviewRequested := map[string]bool{}
	models := map[string]bool{}
	failed := map[string]string{}
	var usage model.Usage
	hasUsage := false
	for _, o := range e.Outcomes {
		if o.Model != "" && !models[o.Model] {
			models[o.Model] = true
			r.Model.Actual = append(r.Model.Actual, o.Model)
		}
		for id, q := range o.Request.Questions {
			if q.Type == "choice" {
				reviewRequested[id] = true
			}
		}
		if o.ErrorCode == "" {
			for k, v := range o.Values {
				if v >= 0 && v <= 1 {
					values[k] = v
				}
			}
			for k, v := range o.Choices {
				if jev.ValidReviewAnswer(v) {
					choices[k] = v
				}
			}
		}
		if o.ErrorCode != "" {
			for id := range o.Request.Questions {
				failed[id] = o.ErrorCode
			}
		}
		if o.Usage != nil {
			usage.InputTokens += o.Usage.InputTokens
			usage.OutputTokens += o.Usage.OutputTokens
			hasUsage = true
		}
	}
	if hasUsage {
		r.Metrics.Usage = &usage
	}
	sort.Strings(r.Model.Actual)
	skipped := map[string]bool{}
	for _, id := range e.Skipped {
		skipped[id] = true
	}
	unitByID := map[string]model.Unit{}
	for _, u := range e.Units {
		unitByID[u.ID] = u
	}
	signals := map[string]map[string]model.Signal{}
	for _, u := range e.Units {
		signals[u.ID] = map[string]model.Signal{}
		for _, rule := range e.Rules {
			id := u.ID + "__" + rule.ID
			s := model.Signal{UnitID: u.ID, RuleID: rule.ID, State: "unknown"}
			if v, ok := values[id]; ok {
				s.Value = &v
				s.State = state(v, rule)
			} else if skipped[u.ID] {
				s.ErrorCode = "budget_exceeded"
			} else {
				s.ErrorCode = failed[id]
				if s.ErrorCode == "" {
					s.ErrorCode = "unknown"
				}
			}
			signals[u.ID][rule.ID] = s
		}
	}
	fileRuleRank := map[string]int{}
	for _, f := range e.Files {
		fileRuleRank[f.Path] = len(e.Rules)
		fr := model.FileResult{Path: f.Path, PreviousPath: f.PreviousPath, Status: f.Status, Tags: []string{}, Targets: []model.Target{}, Reasons: append([]string{}, f.Reasons...)}
		unknown := f.PatchState != "complete" || len(f.Units) == 0
		high := false
		attention := false
		var priorityChecks, attentionChecks []string
		for _, uid := range f.Units {
			u, exists := unitByID[uid]
			if !exists {
				unknown = true
			}
			qid := jev.ReviewQuestionID(uid)
			if _, ok := choices[qid]; !ok && (reviewRequested[qid] || e.ReviewPolicyVersion == 2) {
				unknown = true
				fr.Reasons = appendUnique(fr.Reasons, "review_unavailable")
			}
			if e.ReviewPolicyVersion == 2 {
				for _, risk := range rules.ReviewRisks() {
					if _, ok := values[jev.RiskQuestionID(uid, risk.ID)]; !ok {
						unknown = true
						fr.Reasons = appendUnique(fr.Reasons, "risk_unavailable")
					}
				}
			}
			if u.FileContextPartial {
				fr.ContextPartial = true
			}
			for rank, rule := range e.Rules {
				s := signals[uid][rule.ID]
				if s.State == "unknown" {
					unknown = true
					fr.Reasons = appendUnique(fr.Reasons, s.ErrorCode)
					continue
				}
				if s.State == "suggested" || s.State == "candidate" {
					if rank < fileRuleRank[f.Path] {
						fileRuleRank[f.Path] = rank
					}
					fr.Tags = appendUnique(fr.Tags, rule.Tag)
					fr.Targets = appendTarget(fr.Targets, target(f, u))
					attention = true
					attentionChecks = appendUnique(attentionChecks, rule.Title)
					if s.State == "suggested" && rule.Priority == "high" {
						high = true
						priorityChecks = appendUnique(priorityChecks, rule.Title)
					}
				}
			}
		}
		if unknown {
			fr.Group = "manual"
		} else if high {
			fr.Group = "first"
		} else if attention {
			fr.Group = "normal"
		} else {
			fr.Group = "no_signal"
		}
		if fr.Group != "manual" {
			r.Scope.AnalyzedFiles++
		} else if len(fr.Reasons) == 0 {
			fr.Reasons = append(fr.Reasons, "unknown")
		}
		if e.ReviewPolicyVersion == 2 {
			fr.Review = reviewDecisionV2(f, fr, choices, values, unitByID, e.BodyOmitted)
		} else {
			fr.Review = reviewDecision(f, fr, choices, unitByID, e.BodyOmitted)
		}
		switch fr.Review.Basis {
		case "priority_check":
			fr.Review.Checks = priorityChecks
		case "attention_check":
			fr.Review.Checks = attentionChecks
		}
		for _, reason := range fr.Reasons {
			r.Scope.Unanalyzed = append(r.Scope.Unanalyzed, model.Unanalyzed{Path: f.Path, Reason: reason})
		}
		r.Files = append(r.Files, fr)
	}
	if e.PR.ChangedFiles > len(e.Files) {
		r.Scope.Unanalyzed = append(r.Scope.Unanalyzed, model.Unanalyzed{Path: "*", Reason: "metadata_only"})
	}
	if e.PR.ChangedFiles > 100 {
		r.Scope.Unanalyzed = append(r.Scope.Unanalyzed, model.Unanalyzed{Path: "*", Reason: "file_limit_exceeded"})
	}
	order := map[string]int{"required": 0, "caution": 1, "unnecessary": 2}
	sort.SliceStable(r.Files, func(i, j int) bool {
		a, b := r.Files[i], r.Files[j]
		if order[a.Review.Level] != order[b.Review.Level] {
			return order[a.Review.Level] < order[b.Review.Level]
		}
		if fileRuleRank[a.Path] != fileRuleRank[b.Path] {
			return fileRuleRank[a.Path] < fileRuleRank[b.Path]
		}
		return a.Path < b.Path
	})
	for _, rule := range e.Rules {
		item := model.ChecklistItem{RuleID: rule.ID, Title: rule.Title, State: "no_signal", Targets: []model.Target{}}
		if e.PR.ChangedFiles > len(e.Files) {
			item.Unknown = true
		}
		for _, f := range e.Files {
			if f.PatchState != "complete" {
				item.Unknown = true
			}
			for _, uid := range f.Units {
				s := signals[uid][rule.ID]
				if s.State == "unknown" {
					item.Unknown = true
					continue
				}
				if s.Value != nil && (item.MaxSignal == nil || *s.Value > *item.MaxSignal) {
					v := *s.Value
					item.MaxSignal = &v
				}
				if s.State == "suggested" {
					item.State = "suggested"
					item.Targets = appendTarget(item.Targets, target(f, unitByID[uid]))
				} else if s.State == "candidate" {
					if item.State != "suggested" {
						item.State = "candidate"
					}
					item.Targets = appendTarget(item.Targets, target(f, unitByID[uid]))
				}
			}
		}
		if item.Unknown && item.State == "no_signal" {
			item.State = "unknown"
		}
		r.Checklist = append(r.Checklist, item)
	}
	r.Status = "complete"
	if len(r.Scope.Unanalyzed) > 0 || e.BodyOmitted || e.PR.ChangedFiles > 100 {
		r.Status = "partial"
	}
	for _, item := range r.Checklist {
		if item.Unknown {
			r.Status = "partial"
			break
		}
	}
	r.Warnings = append(r.Warnings, "判定は提示された差分の局所的な分類です。複数ファイルを合わせた挙動や未変更コードは評価していません")
	return r
}
func state(v float64, r model.Rule) string {
	if v >= r.SuggestAt {
		return "suggested"
	}
	if v >= r.CandidateAt {
		return "candidate"
	}
	return "no_signal"
}
func appendUnique(s []string, v string) []string {
	if v == "" {
		return s
	}
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}
func target(f model.File, u model.Unit) model.Target {
	t := model.Target{Path: f.Path, HunkIDs: append([]string{}, u.HunkIDs...), OldRanges: []string{}, NewRanges: []string{}}
	for _, h := range f.Hunks {
		for _, id := range u.HunkIDs {
			if h.ID == id {
				if h.OldCount > 0 {
					t.OldRanges = append(t.OldRanges, fmt.Sprintf("L%d-L%d", h.OldStart, h.OldStart+h.OldCount-1))
				}
				if h.NewCount > 0 {
					t.NewRanges = append(t.NewRanges, fmt.Sprintf("L%d-L%d", h.NewStart, h.NewStart+h.NewCount-1))
				}
			}
		}
	}
	return t
}
func appendTarget(ts []model.Target, t model.Target) []model.Target {
	for _, x := range ts {
		if x.Path == t.Path && strings.Join(x.HunkIDs, ",") == strings.Join(t.HunkIDs, ",") {
			return ts
		}
	}
	return append(ts, t)
}
