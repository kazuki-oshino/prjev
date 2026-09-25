package github

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestParseRef(t *testing.T) {
	r, e := ParseRef("https://github.com/o/r/pull/12/?foo=1#x")
	if e != nil || r.URL != "https://github.com/o/r/pull/12" {
		t.Fatalf("ref=%+v err=%v", r, e)
	}
	for _, s := range []string{"http://github.com/o/r/pull/1", "https://evil.com/o/r/pull/1", "https://github.com:443/o/r/pull/1", "https://github.com/o/r/pull/0", "https://github.com/o/r/pull/1/extra"} {
		if _, e := ParseRef(s); e == nil {
			t.Errorf("accepted %s", s)
		}
	}
}
func TestReaderPagesAndChanged(t *testing.T) {
	ref, _ := ParseRef("https://github.com/o/r/pull/1")
	m := Meta{Number: 1, Title: "hi", BaseRefOid: "a", HeadRefOid: "b", ChangedFiles: 2, Additions: 2}
	meta, _ := json.Marshal(m)
	pages := `[[{"filename":"a","status":"added","additions":1,"changes":1,"patch":"@@ -0,0 +1 @@\n+x\n"}],[{"filename":"b","status":"added","additions":1,"changes":1,"patch":"@@ -0,0 +1 @@\n+y\n"}]]`
	calls := 0
	r := Reader{Run: func(_ context.Context, args ...string) ([]byte, error) {
		calls++
		if args[0] == "api" {
			return []byte(pages), nil
		}
		return meta, nil
	}}
	pr, files, e := r.Read(context.Background(), ref)
	if e != nil || len(files) != 2 || pr.Hash == "" || calls != 3 {
		t.Fatalf("pr=%+v files=%+v err=%v calls=%d", pr, files, e, calls)
	}
	changed := m
	changed.HeadRefOid = "c"
	z, _ := json.Marshal(changed)
	calls = 0
	r.Run = func(_ context.Context, args ...string) ([]byte, error) {
		calls++
		if args[0] == "api" {
			return []byte(pages), nil
		}
		if calls == 3 {
			return z, nil
		}
		return meta, nil
	}
	_, _, e = r.Read(context.Background(), ref)
	if e == nil || !strings.Contains(e.Error(), "pr_changed_during_fetch") {
		t.Fatalf("err=%v", e)
	}
}
func TestKeyFilteredFromChild(t *testing.T) {
	t.Setenv("TYPESAFE_API_KEY", "secret-test-value")
	for _, v := range filteredEnv() {
		if strings.Contains(v, "secret-test-value") {
			t.Fatal("key leaked")
		}
	}
}
