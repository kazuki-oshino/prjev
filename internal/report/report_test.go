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
	for _, want := range []string{"<!doctype html>", "一部未解析", "未解析の範囲があります", "確認する観点", "ファイルを読む順番", "a&lt;b&gt;.go", "L2-L4", "&lt;warning&gt;", "default-src 'none'"} {
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
