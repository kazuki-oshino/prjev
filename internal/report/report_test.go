package report

import (
	"strings"
	"testing"

	"github.com/kazuki-oshino/prjev/internal/model"
)

func TestEscapesUntrustedText(t *testing.T) {
	r := model.Result{PR: model.PR{Number: 1, URL: "https://github.com/o/r/pull/1", Title: "<script>alert(1)</script>\x1b[31m"}, Status: "partial", Files: []model.FileResult{{Path: "a|b<script>.go", Group: "manual", Reasons: []string{"patchなし"}}}}
	b, e := Render(r, "markdown", "scan")
	if e != nil {
		t.Fatal(e)
	}
	s := string(b)
	if strings.Contains(s, "<script>") || strings.Contains(s, "\x1b") || !strings.Contains(s, `a\|b`) {
		t.Fatalf("unsafe markdown: %s", s)
	}
	b, e = Render(r, "terminal", "scan")
	if e != nil || strings.Contains(string(b), "\x1b") {
		t.Fatalf("unsafe terminal: %s %v", b, e)
	}
}

func TestHTMLShowsPartialResultAndEscapesPRText(t *testing.T) {
	r := model.Result{
		PR:        model.PR{Number: 7, URL: "javascript:alert(1)", Title: `<script>alert(1)</script>`, ChangedFiles: 2},
		Status:    "partial",
		Scope:     model.Scope{AnalyzedFiles: 1},
		Checklist: []model.ChecklistItem{{Title: `<img src=x onerror=alert(1)>`, State: "suggested", Unknown: true, Targets: []model.Target{{Path: `a<b>.go`, NewRanges: []string{"L2-L4"}}}}},
		Files:     []model.FileResult{{Path: `a<b>.go`, Group: "manual", Reasons: []string{`<unsafe>`}}},
		Warnings:  []string{`<warning>`},
	}
	b, err := Render(r, "html", "scan")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{"<!doctype html>", "一部未解析", "未解析の範囲があります", "確認する観点", "ファイルごとの確認の要否", "a&lt;b&gt;.go", "L2-L4", "&lt;warning&gt;", "default-src 'none'"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, unsafe := range []string{"<script>", "<img src=x", "href=\"javascript:", "<unsafe>", "<warning>"} {
		if strings.Contains(s, unsafe) {
			t.Errorf("unescaped %q", unsafe)
		}
	}
}

func TestHTMLRespectsViewAndEmptyState(t *testing.T) {
	r := model.Result{Status: "complete", Files: []model.FileResult{{Path: "src/main.go", Group: "no_signal"}}}
	b, err := Render(r, "html", "checklist")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "今回の差分から追加提案なし") || strings.Contains(string(b), `id="radar"`) {
		t.Fatalf("unexpected checklist view: %s", b)
	}
	b, err = Render(r, "html", "files")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "src/main.go") || strings.Contains(string(b), `id="checklist"`) {
		t.Fatalf("unexpected files view: %s", b)
	}
}

func TestHTMLPRLinkUsesCanonicalGitHubURL(t *testing.T) {
	if got := safePRURL("https://github.com/o/r/pull/7?token=private#files"); got != "https://github.com/o/r/pull/7" {
		t.Fatalf("canonical URL = %q", got)
	}
	for _, raw := range []string{"javascript:alert(1)", "https://github.com.evil.test/o/r/pull/7", "https://github.com/o/r/issues/7"} {
		if got := safePRURL(raw); got != "" {
			t.Errorf("unsafe URL %q became %q", raw, got)
		}
	}
}

func TestReviewGuidanceAcrossFormats(t *testing.T) {
	c := .58
	r := model.Result{PR: model.PR{ChangedFiles: 4}, Status: "partial", Scope: model.Scope{AnalyzedFiles: 2},
		Files: []model.FileResult{
			{Path: "skip.go", Review: model.ReviewDecision{Level: "unnecessary", Reason: "省略候補"}},
			{Path: "careful.go", Review: model.ReviewDecision{Level: "caution", Reason: "判定の確かさが基準に届かない", Judgments: []model.ReviewJudgment{{ChoiceAnswer: model.ChoiceAnswer{Choice: "unnecessary", Confidence: &c}}}}},
			{Path: "required.go", Review: model.ReviewDecision{Level: "required", Reason: "未解析"}, Reasons: []string{"api_error"}},
		}, Checklist: []model.ChecklistItem{{Title: "動作を確認", State: "suggested"}},
	}
	for _, format := range []string{"terminal", "markdown", "html"} {
		t.Run(format, func(t *testing.T) {
			b, err := Render(r, format, "scan")
			if err != nil {
				t.Fatal(err)
			}
			s := string(b)
			for _, want := range []string{"必須", "注意", "不要", "58.0%", "Jevから判定を取得できませんでした", "コードが正しい確率ではありません", "一覧を取得できなかった1ファイル", "基準に届かない"} {
				if !strings.Contains(s, want) {
					t.Errorf("missing %q", want)
				}
			}
			if !(strings.Index(s, "required.go") < strings.Index(s, "careful.go") && strings.Index(s, "careful.go") < strings.Index(s, "skip.go") && strings.Index(s, "skip.go") < strings.Index(s, "動作を確認")) {
				t.Fatal("reading order is incorrect")
			}
			if format == "html" && (!strings.Contains(s, `<details class="skip-group">`) || strings.Contains(s, `class="skip-group" open`)) {
				t.Fatal("skip candidates should be collapsed")
			}
			if format == "markdown" && !strings.Contains(s, "<summary>不要: 1ファイル") {
				t.Fatal("missing collapsed skip candidates")
			}
		})
	}
	s := summarize(r)
	if s.Required != 2 || s.Caution != 1 || s.Unnecessary != 1 || s.Missing != 1 {
		t.Fatalf("%+v", s)
	}
}

func TestHTMLSeparatesFinalDecisionFromAIConfidence(t *testing.T) {
	c := .28
	r := model.Result{Status: "complete", Files: []model.FileResult{{Path: "app.go", Group: "first", Review: model.ReviewDecision{
		Level: "required", Basis: "priority_check", Reason: "重点確認項目に該当します。", Checks: []string{"独自の契約を確認", "別の確認項目"},
		Judgments: []model.ReviewJudgment{{ChoiceAnswer: model.ChoiceAnswer{Choice: "caution", Confidence: &c}}},
	}}}}
	b, err := Render(r, "html", "files")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{"確認区分</span><strong", "必須</strong>", "AIの見立て（参考）", "確認を勧める", "この見立ての確信度", "28.0%", "重点確認項目への該当を優先", "独自の契約を確認", "ほか1項目", "別の確認項目", "最終的な確認区分の確信度ではなく"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(s, "Jev: 注意") {
		t.Fatal("ambiguous confidence label remains")
	}
	if strings.Contains(s, "APIの互換性") {
		t.Fatal("inferred a hard-coded check instead of using the supplied title")
	}

	// Old rendered results do not contain the new explanation metadata.
	r.Files[0].Review.Basis, r.Files[0].Review.Checks = "", nil
	b, err = Render(r, "html", "files")
	if err != nil || !strings.Contains(string(b), "確認項目や解析範囲の条件も合わせて") {
		t.Fatalf("old result: %s %v", b, err)
	}
}

func TestHTMLAssessmentScopeAndDecisionNotes(t *testing.T) {
	low, high := .2, .98
	js := []model.ReviewJudgment{{ChoiceAnswer: model.ChoiceAnswer{Choice: "required", Confidence: &low}}, {ChoiceAnswer: model.ChoiceAnswer{Choice: "caution", Confidence: &high}}}
	a := assessment(js)
	if a.Label != "範囲によって見立てが異なる" || a.Confidence != "20.0%" || a.ConfidenceLabel != "各範囲の確信度の最低値" {
		t.Fatalf("%+v", a)
	}
	if a := assessment(nil); a.Confidence != "" || a.Label != "見立てを取得できていません" {
		t.Fatalf("%+v", a)
	}
	for _, tt := range []struct{ basis, choice, level, want string }{
		{"incomplete", "unnecessary", "required", "解析できなかった範囲"},
		{"low_confidence", "unnecessary", "caution", "確信度が基準に届かない"},
		{"ai_required", "caution", "required", "確認が必要という見立ての範囲"},
		{"ai_caution", "caution", "caution", ""},
	} {
		f := model.FileResult{Review: model.ReviewDecision{Basis: tt.basis, Judgments: []model.ReviewJudgment{{ChoiceAnswer: model.ChoiceAnswer{Choice: tt.choice}}}}}
		got := decisionNote(f, tt.level)
		if tt.want == "" && got != "" || tt.want != "" && !strings.Contains(got, tt.want) {
			t.Errorf("basis=%s note=%s", tt.basis, got)
		}
	}
}

func TestHTMLCheckTitlesAreEscaped(t *testing.T) {
	r := model.Result{Files: []model.FileResult{{Path: "a.go", Review: model.ReviewDecision{Level: "required", Checks: []string{`<script>do not execute</script>`}}}}}
	b, err := Render(r, "html", "files")
	if err != nil || strings.Contains(string(b), "<script>") || !strings.Contains(string(b), "&lt;script&gt;") {
		t.Fatalf("%s %v", b, err)
	}
}

func TestConfidenceShowsRawChoiceAndLowestValue(t *testing.T) {
	a, b := .98, .20
	js := []model.ReviewJudgment{{ChoiceAnswer: model.ChoiceAnswer{Choice: "unnecessary", Confidence: &a}}, {ChoiceAnswer: model.ChoiceAnswer{Choice: "required", Confidence: &b}}}
	if got := confidenceSummary(js); got != "Jev: 判定が混在 / 最低の確信度 20.0%" {
		t.Fatal(got)
	}
	if got := confidenceSummary(nil); got != "Jevの確信度: 未取得" {
		t.Fatal(got)
	}
	if got := percent(.84999); got != "84.9%" {
		t.Fatal("rounded up past skip threshold", got)
	}
}

func TestLegacyNoSignalIsNotUnnecessary(t *testing.T) {
	s := summarize(model.Result{Files: []model.FileResult{{Path: "old.go", Group: "no_signal"}}})
	if s.Unnecessary != 0 || s.Caution != 1 || s.Groups[1].Files[0].Confidence != "Jevの確信度: 未取得" {
		t.Fatalf("%+v", s)
	}
}

func TestReviewReasonEscapesAllHumanFormats(t *testing.T) {
	r := model.Result{Files: []model.FileResult{{Path: "a.go", Review: model.ReviewDecision{Level: "caution", Reason: `<script>alert(1)</script>`}, Tags: []string{`<img src=x>`}}}}
	for _, format := range []string{"html", "markdown"} {
		b, err := Render(r, format, "files")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "<script>") || strings.Contains(string(b), "<img src=x>") {
			t.Fatalf("unescaped output %s", format)
		}
	}
}
