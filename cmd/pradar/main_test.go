package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kazuki-oshino/prjev/internal/app"
	"github.com/kazuki-oshino/prjev/internal/jev"
	"github.com/kazuki-oshino/prjev/internal/model"
	"github.com/kazuki-oshino/prjev/internal/rules"
	"github.com/kazuki-oshino/prjev/internal/store"
)

func TestReplayNoExternalDependencies(t *testing.T) {
	t.Setenv("TYPESAFE_API_KEY", "")
	path := filepath.Join(t.TempDir(), "record.json")
	ev := app.Evidence{PR: model.PR{Number: 1, ChangedFiles: 1, URL: "https://github.com/o/r/pull/1"}, Rules: rules.Standard(), Files: []model.File{{ID: "f1", Path: "a.png", PatchState: "unknown", Reasons: []string{"patchなし"}}}}
	result := app.Aggregate(ev, "jev-latest")
	if e := store.Save(path, store.Record{RecordSchemaVersion: 1, Evidence: ev, Result: result}); e != nil {
		t.Fatal(e)
	}
	var out, err bytes.Buffer
	code := run([]string{"replay", "--format", "json", path}, &out, &err)
	if code != 2 || !strings.Contains(out.String(), `"status": "partial"`) || err.Len() != 0 {
		t.Fatalf("code=%d out=%s err=%s", code, out.String(), err.String())
	}
	htmlPath := filepath.Join(t.TempDir(), "report.html")
	out.Reset()
	err.Reset()
	code = run([]string{"replay", "--format", "html", "--output", htmlPath, path}, &out, &err)
	if code != 2 || out.Len() != 0 || err.Len() != 0 {
		t.Fatalf("html code=%d out=%s err=%s", code, out.String(), err.String())
	}
	html, readErr := os.ReadFile(htmlPath)
	if readErr != nil || !strings.Contains(string(html), "<!doctype html>") || !strings.Contains(string(html), "a.png") {
		t.Fatalf("html output=%s err=%v", html, readErr)
	}
}
func TestMissingKeyBeforeGitHub(t *testing.T) {
	t.Setenv("TYPESAFE_API_KEY", "")
	dir := t.TempDir()
	t.Chdir(dir)
	var out, err bytes.Buffer
	code := run([]string{"https://github.com/o/r/pull/1"}, &out, &err)
	if code != 1 || !strings.Contains(err.String(), "TYPESAFE_API_KEY") || out.Len() != 0 {
		t.Fatalf("code=%d out=%s err=%s", code, out.String(), err.String())
	}
}

func TestReplayPreservesChoiceWithoutExternalDependencies(t *testing.T) {
	t.Setenv("TYPESAFE_API_KEY", "")
	t.Setenv("PATH", "")
	t.Chdir(t.TempDir())
	e := app.Evidence{PR: model.PR{ChangedFiles: 1}, Rules: rules.Standard(), Files: []model.File{{ID: "f1", Path: "format.go", PatchState: "complete", Units: []string{"u1"}}}, Units: []model.Unit{{ID: "u1", FileID: "f1", Path: "format.go"}}}
	c := .98
	o := jev.Outcome{Request: jev.MakeRequest(e.PR, e.Units, e.Rules, "jev-latest"), Values: map[string]float64{}, Choices: map[string]model.ChoiceAnswer{jev.ReviewQuestionID("u1"): {Choice: "unnecessary", Confidence: &c, Probabilities: map[string]float64{"required": .01, "caution": .01, "unnecessary": .98}}}}
	for _, rule := range e.Rules {
		o.Values["u1__"+rule.ID] = .01
	}
	e.Outcomes = []jev.Outcome{o}
	path := filepath.Join(t.TempDir(), "record.json")
	if err := store.Save(path, store.Record{RecordSchemaVersion: 1, Evidence: e, Result: app.Aggregate(e, "jev-latest")}); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	code := run([]string{"replay", "--format", "json", path}, &out, &stderr)
	var r model.Result
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	if code != 0 || stderr.Len() != 0 || r.Files[0].Review.Level != "unnecessary" || *r.Files[0].Review.Judgments[0].Confidence != c {
		t.Fatalf("code=%d result=%+v stderr=%s", code, r, stderr.String())
	}
}
