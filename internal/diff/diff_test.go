package diff

import (
	"strings"
	"testing"

	"github.com/kazuki-oshino/prjev/internal/github"
)

func TestParseAndNormalize(t *testing.T) {
	patch := "@@ -10 +10 @@\n-old\n+new\n@@ -20,0 +21,2 @@\n+a\n+b\n"
	hs, a, d, e := ParseHunks(patch)
	if e != nil || len(hs) != 2 || a != 3 || d != 1 || hs[1].OldCount != 0 {
		t.Fatalf("hunks=%+v a=%d d=%d err=%v", hs, a, d, e)
	}
	files, units := Normalize([]github.APIFile{{Filename: "a.go", Status: "modified", Additions: 3, Deletions: 1, Changes: 4, Patch: &patch}})
	if len(units) != 1 || files[0].PatchState != "complete" || len(units[0].HunkIDs) != 2 {
		t.Fatalf("files=%+v units=%+v", files, units)
	}
}
func TestUnknownAndSecret(t *testing.T) {
	patch := "@@ -1 +1 @@\n-old\n+new\n"
	files, units := Normalize([]github.APIFile{{Filename: ".env.local", Status: "modified", Additions: 1, Deletions: 1, Changes: 2, Patch: &patch}, {Filename: "broken.go", Status: "modified", Additions: 2, Deletions: 1, Changes: 3, Patch: &patch}, {Filename: "image.png", Status: "modified"}})
	if len(units) != 0 || files[0].PatchState != "unknown" || files[1].PatchState != "unknown" || files[2].PatchState != "unknown" {
		t.Fatalf("files=%+v units=%+v", files, units)
	}
}
func TestLargeHunkStaysUnknown(t *testing.T) {
	patch := "@@ -0,0 +1 @@\n+" + strings.Repeat("a", 13<<10) + "\n"
	files, units := Normalize([]github.APIFile{{Filename: "big.go", Status: "added", Additions: 1, Changes: 1, Patch: &patch}})
	if len(units) != 0 || files[0].PatchState != "unknown" {
		t.Fatal("large hunk should not be sent")
	}
}
func TestMalformedHunk(t *testing.T) {
	if _, _, _, e := ParseHunks("@@ -1 +1 @@\n-old\n"); e == nil {
		t.Fatal("missing line was accepted")
	}
}
