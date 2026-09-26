package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnvPriorityAndExplicitErrors(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, "local.env")
	if e := os.WriteFile(env, []byte("TYPESAFE_API_KEY=file-value\nOTHER=unused\n"), 0600); e != nil {
		t.Fatal(e)
	}
	t.Setenv("TYPESAFE_API_KEY", "process-value")
	c, e := Load("", env, true)
	if e != nil || c.Key != "process-value" {
		t.Fatalf("config=%+v err=%v", c, e)
	}
	t.Setenv("TYPESAFE_API_KEY", "")
	c, e = Load("", env, true)
	if e != nil || c.Key != "file-value" {
		t.Fatalf("config=%+v err=%v", c, e)
	}
	if _, e = Load("", filepath.Join(dir, "missing"), true); e == nil {
		t.Fatal("explicit missing file accepted")
	}
}
func TestStrictYAML(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.yaml")
	t.Setenv("TYPESAFE_API_KEY", "x")
	for _, body := range []string{"version: 1\nunknown: x\n", "version: 1\nextra_checks:\n - id: behavior_change\n   title: x\n", "version: 1\nextra_checks:\n - id: custom\n   title: x\n   tag: x\n   priority: high\n   instructions: x\n   criteria: {true: yes, false: no}\n   candidate_at: 0.9\n   suggest_at: 0.8\n"} {
		if e := os.WriteFile(p, []byte(body), 0600); e != nil {
			t.Fatal(e)
		}
		if _, e := Load(p, "", false); e == nil {
			t.Errorf("accepted %s", strings.Split(body, "\n")[0])
		}
	}
}
