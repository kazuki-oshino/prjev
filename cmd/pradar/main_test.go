package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kazuki-oshino/prjev/internal/app"
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
