package app

import (
	"encoding/json"
	"testing"

	"github.com/kazuki-oshino/prjev/internal/jev"
	"github.com/kazuki-oshino/prjev/internal/model"
	"github.com/kazuki-oshino/prjev/internal/rules"
)

func reviewV2Evidence(choice string, confidence float64) Evidence {
	e := reviewEvidence(choice, confidence)
	e.ReviewPolicyVersion = 2
	for _, risk := range rules.ReviewRisks() {
		e.Outcomes[0].Values[jev.RiskQuestionID("u1", risk.ID)] = .01
	}
	return e
}

func TestReviewV2SeparatesTopicsFromPriority(t *testing.T) {
	for _, choice := range []string{"caution", "unnecessary"} {
		e := reviewV2Evidence(choice, .98)
		// Even a high-priority custom checklist match is a topic, not risk evidence.
		e.Rules = append(e.Rules, model.Rule{ID: "custom", Title: "独自の確認項目", Tag: "custom", Priority: "high", SuggestAt: .75, CandidateAt: .35})
		for _, r := range e.Rules {
			e.Outcomes[0].Values["u1__"+r.ID] = .99
		}
		r := Aggregate(e, "test")
		if r.Files[0].Review.Level != choice || r.Checklist[0].State != "suggested" || len(r.Files[0].Review.Checks) != 0 {
			t.Fatalf("topics changed priority or disappeared: %+v", r)
		}
	}
}

func TestReviewV2ConfidenceRiskAndCoverage(t *testing.T) {
	tests := []struct {
		name, choice         string
		confidence           float64
		change               func(*Evidence)
		level, basis, status string
	}{
		{"clear skip", "unnecessary", .85, nil, "unnecessary", "skip_eligible", "complete"},
		{"uncertain skip", "unnecessary", .8499, nil, "caution", "low_confidence", "complete"},
		{"uncertain priority", "required", .28, nil, "caution", "uncertain_priority", "complete"},
		{"priority boundary below", "required", .8499, nil, "caution", "uncertain_priority", "complete"},
		{"clear priority", "required", .85, nil, "required", "ai_required", "complete"},
		{"real consequence", "caution", .98, func(e *Evidence) { e.Outcomes[0].Values[jev.RiskQuestionID("u1", "runtime_impact")] = .75 }, "required", "risk_signal", "complete"},
		{"lost assertion overrides skip", "unnecessary", .99, func(e *Evidence) { e.Outcomes[0].Values[jev.RiskQuestionID("u1", "verification_loss")] = .9 }, "required", "risk_signal", "complete"},
		{"possible consequence", "unnecessary", .99, func(e *Evidence) { e.Outcomes[0].Values[jev.RiskQuestionID("u1", "runtime_impact")] = .7499 }, "caution", "risk_candidate", "complete"},
		{"candidate boundary", "unnecessary", .99, func(e *Evidence) { e.Outcomes[0].Values[jev.RiskQuestionID("u1", "verification_loss")] = .35 }, "caution", "risk_candidate", "complete"},
		{"below candidate", "unnecessary", .99, func(e *Evidence) { e.Outcomes[0].Values[jev.RiskQuestionID("u1", "verification_loss")] = .3499 }, "unnecessary", "skip_eligible", "complete"},
		{"missing risk", "unnecessary", .99, func(e *Evidence) { delete(e.Outcomes[0].Values, jev.RiskQuestionID("u1", "runtime_impact")) }, "required", "incomplete", "partial"},
		{"invalid risk", "unnecessary", .99, func(e *Evidence) { e.Outcomes[0].Values[jev.RiskQuestionID("u1", "runtime_impact")] = 1.1 }, "required", "incomplete", "partial"},
		{"missing choice and request", "unnecessary", .99, func(e *Evidence) { e.Outcomes[0].Choices = nil; e.Outcomes[0].Request.Questions = nil }, "required", "incomplete", "partial"},
		{"missing topic", "unnecessary", .99, func(e *Evidence) { delete(e.Outcomes[0].Values, "u1__behavior_change") }, "required", "incomplete", "partial"},
		{"missing patch", "unnecessary", .99, func(e *Evidence) { e.Files[0].PatchState = "unknown" }, "required", "incomplete", "partial"},
		{"batch failure", "unnecessary", .99, func(e *Evidence) { e.Outcomes[0].ErrorCode = "api_error" }, "required", "incomplete", "partial"},
		{"split", "unnecessary", .99, func(e *Evidence) { e.Units[0].FileContextPartial = true }, "caution", "split_context", "complete"},
		{"body omitted", "unnecessary", .99, func(e *Evidence) { e.BodyOmitted = true }, "caution", "omitted_description", "partial"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := reviewV2Evidence(tt.choice, tt.confidence)
			if tt.change != nil {
				tt.change(&e)
			}
			r := Aggregate(e, "test")
			d := r.Files[0].Review
			if d.Level != tt.level || d.Basis != tt.basis || r.Status != tt.status {
				t.Fatalf("review=%+v status=%s", d, r.Status)
			}
		})
	}
}

func TestReviewV2KeepsRiskEvidenceAcrossRangesAndReplay(t *testing.T) {
	e := reviewV2Evidence("unnecessary", .99)
	u := model.Unit{ID: "u2", FileID: "f1", Path: "a.go", HunkIDs: []string{"h002"}, FileContextPartial: true}
	e.Units = append(e.Units, u)
	e.Files[0].Units = append(e.Files[0].Units, u.ID)
	e.Files[0].Hunks = append(e.Files[0].Hunks, model.Hunk{ID: "h002", NewStart: 50, NewCount: 2})
	for _, r := range e.Rules {
		e.Outcomes[0].Values["u2__"+r.ID] = .01
	}
	for _, r := range rules.ReviewRisks() {
		e.Outcomes[0].Values[jev.RiskQuestionID("u2", r.ID)] = .01
	}
	e.Outcomes[0].Values[jev.RiskQuestionID("u2", "verification_loss")] = .95
	e.Outcomes[0].Choices[jev.ReviewQuestionID("u2")] = e.Outcomes[0].Choices[jev.ReviewQuestionID("u1")]
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	var restored Evidence
	if err := json.Unmarshal(b, &restored); err != nil {
		t.Fatal(err)
	}
	r := Aggregate(restored, "test")
	d := r.Files[0].Review
	if r.ReviewPolicy.Version != 2 || d.Level != "required" || len(d.Risks) != 4 || len(d.Checks) != 1 || d.Risks[3].Value != .95 || d.Risks[3].Target.NewRanges[0] != "L50-L51" {
		t.Fatalf("%+v", r)
	}
}

func TestLegacyReviewPolicyIsNotReinterpreted(t *testing.T) {
	for _, version := range []int{0, 1} {
		e := reviewV2Evidence("caution", .99)
		e.ReviewPolicyVersion = version
		e.Outcomes[0].Values["u1__api_contract"] = .95
		r := Aggregate(e, "test")
		if r.ReviewPolicy.Version != 1 || r.Files[0].Review.Basis != "priority_check" || r.Files[0].Review.Level != "required" || len(r.Files[0].Review.Risks) != 0 {
			t.Fatalf("legacy changed: %+v", r)
		}
	}
}
