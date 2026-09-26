package app

import (
	"testing"

	"github.com/kazuki-oshino/prjev/internal/jev"
	"github.com/kazuki-oshino/prjev/internal/model"
	"github.com/kazuki-oshino/prjev/internal/rules"
)

func TestPartialNeverBecomesNoSignal(t *testing.T) {
	patch := "@@ -1 +1 @@\n-old\n+new\n"
	_ = patch
	rs := rules.Standard()
	e := Evidence{PR: model.PR{ChangedFiles: 2}, Rules: rs, Files: []model.File{{ID: "f1", Path: "a.go", PatchState: "complete", Units: []string{"u1"}, Hunks: []model.Hunk{{ID: "h001", NewStart: 1, NewCount: 1}}}, {ID: "f2", Path: "b.png", PatchState: "unknown", Reasons: []string{"patchなし"}}}, Units: []model.Unit{{ID: "u1", FileID: "f1", Path: "a.go", HunkIDs: []string{"h001"}}}}
	v := .9
	e.Outcomes = []jev.Outcome{{BatchID: "b1", Values: map[string]float64{"u1__external_integration": v}, ErrorCode: "api_error"}}
	r := Aggregate(e, "jev-latest")
	if r.Status != "partial" || r.Files[0].Group != "manual" || r.Scope.AnalyzedFiles != 0 {
		t.Fatalf("result=%+v", r)
	}
	for _, x := range r.Checklist {
		if x.State == "no_signal" {
			t.Fatalf("unknown became no_signal: %+v", x)
		}
	}
}

func TestMetadataOnlyIsUnknown(t *testing.T) {
	e := Evidence{PR: model.PR{ChangedFiles: 3}, Rules: rules.Standard(), Files: []model.File{}, Units: []model.Unit{}}
	r := Aggregate(e, "jev-latest")
	if r.Status != "partial" || r.Scope.FetchedFiles != 0 || len(r.Scope.Unanalyzed) == 0 {
		t.Fatalf("scope=%+v status=%s", r.Scope, r.Status)
	}
	for _, item := range r.Checklist {
		if item.State != "unknown" || !item.Unknown {
			t.Fatalf("item=%+v", item)
		}
	}
}
