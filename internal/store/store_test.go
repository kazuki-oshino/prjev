package store

import (
	"github.com/kazuki-oshino/prjev/internal/app"
	"github.com/kazuki-oshino/prjev/internal/model"
	"github.com/kazuki-oshino/prjev/internal/rules"
	"os"
	"path/filepath"
	"testing"
)

func TestReadRejectsUnknownReviewPolicy(t *testing.T) {
	e := app.Evidence{ReviewPolicyVersion: 3, Rules: rules.Standard()}
	r := Record{RecordSchemaVersion: 1, Evidence: e, Result: model.Result{SchemaVersion: 1}}
	p := filepath.Join(t.TempDir(), "future.json")
	if err := Save(p, r); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(p); err == nil {
		t.Fatal("unknown policy silently reinterpreted")
	}
}

func TestAtomicNoOverwriteAndPermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested")
	path := filepath.Join(dir, "report.json")
	if e := SaveBytes(path, []byte("first")); e != nil {
		t.Fatal(e)
	}
	if e := SaveBytes(path, []byte("second")); e == nil {
		t.Fatal("overwrote file")
	}
	b, e := os.ReadFile(path)
	if e != nil || string(b) != "first" {
		t.Fatalf("%s %v", b, e)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatalf("mode=%o", info.Mode().Perm())
	}
	d, _ := os.Stat(dir)
	if d.Mode().Perm() != 0700 {
		t.Fatalf("dir mode=%o", d.Mode().Perm())
	}
}
