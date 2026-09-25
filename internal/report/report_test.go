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
