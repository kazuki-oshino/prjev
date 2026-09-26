package app

import (
	"encoding/json"
	"testing"

	"github.com/kazuki-oshino/prjev/internal/jev"
	"github.com/kazuki-oshino/prjev/internal/model"
	"github.com/kazuki-oshino/prjev/internal/rules"
)

func reviewEvidence(choice string, confidence float64) Evidence {
	u := model.Unit{ID: "u1", FileID: "f1", Path: "a.go", HunkIDs: []string{"h001"}}
	e := Evidence{PR: model.PR{ChangedFiles: 1}, Rules: rules.Standard(), Units: []model.Unit{u}, Files: []model.File{{ID: "f1", Path: u.Path, PatchState: "complete", Units: []string{u.ID}, Hunks: []model.Hunk{{ID: "h001", NewStart: 1, NewCount: 2}}}}}
	o := jev.Outcome{Request: jev.MakeRequest(e.PR, e.Units, e.Rules, "jev-latest"), Values: map[string]float64{}, Choices: map[string]model.ChoiceAnswer{}}
	for _, r := range e.Rules {
		o.Values[u.ID+"__"+r.ID] = .01
	}
	p := map[string]float64{"required": .01, "caution": .01, "unnecessary": .01}
	p[choice] = .98
	o.Choices[jev.ReviewQuestionID(u.ID)] = model.ChoiceAnswer{Choice: choice, Confidence: &confidence, Probabilities: p}
	e.Outcomes = []jev.Outcome{o}
	return e
}

func TestReviewPolicy(t *testing.T) {
	tests := []struct {
		name, choice  string
		confidence    float64
		change        func(*Evidence)
		level, status string
	}{
		{name: "mechanical", choice: "unnecessary", confidence: .99, level: "unnecessary", status: "complete"},
		{name: "threshold inclusive", choice: "unnecessary", confidence: .85, level: "unnecessary", status: "complete"},
		{name: "below threshold", choice: "unnecessary", confidence: .8499, level: "caution", status: "complete"},
		{name: "ambiguous", choice: "unnecessary", confidence: 0, level: "caution", status: "complete"},
		{name: "explicit caution", choice: "caution", confidence: .99, level: "caution", status: "complete"},
		{name: "explicit required", choice: "required", confidence: .2, level: "required", status: "complete"},
		{name: "important rule overrides skip", choice: "unnecessary", confidence: 1, change: func(e *Evidence) { e.Outcomes[0].Values["u1__persistence_change"] = .9 }, level: "required", status: "complete"},
		{name: "ordinary rule overrides skip", choice: "unnecessary", confidence: 1, change: func(e *Evidence) { e.Outcomes[0].Values["u1__behavior_change"] = .35 }, level: "caution", status: "complete"},
		{name: "split context", choice: "unnecessary", confidence: 1, change: func(e *Evidence) { e.Units[0].FileContextPartial = true }, level: "caution", status: "complete"},
		{name: "omitted body", choice: "unnecessary", confidence: 1, change: func(e *Evidence) { e.BodyOmitted = true }, level: "caution", status: "partial"},
		{name: "missing patch", choice: "unnecessary", confidence: 1, change: func(e *Evidence) { e.Files[0].PatchState = "unknown" }, level: "required", status: "partial"},
		{name: "incomplete patch", choice: "unnecessary", confidence: 1, change: func(e *Evidence) { e.Files[0].PatchState = "partial" }, level: "required", status: "partial"},
		{name: "missing choice", choice: "unnecessary", confidence: 1, change: func(e *Evidence) { e.Outcomes[0].Choices = nil }, level: "required", status: "partial"},
		{name: "malformed saved choice", choice: "unnecessary", confidence: 1, change: func(e *Evidence) {
			a := e.Outcomes[0].Choices[jev.ReviewQuestionID("u1")]
			a.Confidence = nil
			e.Outcomes[0].Choices[jev.ReviewQuestionID("u1")] = a
		}, level: "required", status: "partial"},
		{name: "legacy recording", choice: "unnecessary", confidence: 1, change: func(e *Evidence) {
			e.Outcomes[0].Choices = nil
			delete(e.Outcomes[0].Request.Questions, jev.ReviewQuestionID("u1"))
		}, level: "caution", status: "complete"},
		{name: "failed batch discards answers", choice: "unnecessary", confidence: 1, change: func(e *Evidence) { e.Outcomes[0].ErrorCode = "api_error" }, level: "required", status: "partial"},
		{name: "missing rule", choice: "unnecessary", confidence: 1, change: func(e *Evidence) { delete(e.Outcomes[0].Values, "u1__behavior_change") }, level: "required", status: "partial"},
		{name: "no units", choice: "unnecessary", confidence: 1, change: func(e *Evidence) { e.Files[0].Units = nil }, level: "required", status: "partial"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := reviewEvidence(tt.choice, tt.confidence)
			if tt.change != nil {
				tt.change(&e)
			}
			r := Aggregate(e, "jev-latest")
			if r.Files[0].Review.Level != tt.level || r.Status != tt.status {
				t.Fatalf("review=%+v status=%s", r.Files[0].Review, r.Status)
			}
			if tt.name == "legacy recording" && len(r.Files[0].Review.Judgments) != 0 {
				t.Fatal("invented confidence for legacy evidence")
			}
		})
	}
}

func TestSplitReviewDoesNotAverageAwayConcern(t *testing.T) {
	e := reviewEvidence("unnecessary", 1)
	u := model.Unit{ID: "u2", FileID: "f1", Path: "a.go", FileContextPartial: true, HunkIDs: []string{"h002"}}
	e.Units = append(e.Units, u)
	e.Files[0].Units = append(e.Files[0].Units, u.ID)
	e.Files[0].Hunks = append(e.Files[0].Hunks, model.Hunk{ID: "h002", NewStart: 50, NewCount: 2})
	for _, r := range e.Rules {
		e.Outcomes[0].Values["u2__"+r.ID] = .01
	}
	a := reviewEvidence("required", .4).Outcomes[0].Choices[jev.ReviewQuestionID("u1")]
	e.Outcomes[0].Choices[jev.ReviewQuestionID("u2")] = a
	r := Aggregate(e, "jev-latest")
	d := r.Files[0].Review
	if d.Level != "required" || len(d.Judgments) != 2 || *d.Judgments[1].Confidence != .4 || d.Judgments[1].Target.NewRanges[0] != "L50-L51" {
		t.Fatalf("%+v", d)
	}
	a.Choice = "unnecessary"
	a.Probabilities = map[string]float64{"required": .01, "caution": .01, "unnecessary": .98}
	e.Outcomes[0].Choices[jev.ReviewQuestionID("u2")] = a
	if got := Aggregate(e, "jev-latest").Files[0].Review.Level; got != "caution" {
		t.Fatal(got)
	}
}

func TestReviewRecordRoundTrip(t *testing.T) {
	e := reviewEvidence("unnecessary", .98)
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	var saved Evidence
	if err := json.Unmarshal(b, &saved); err != nil {
		t.Fatal(err)
	}
	r := Aggregate(saved, "jev-latest")
	if r.Files[0].Review.Level != "unnecessary" || *r.Files[0].Review.Judgments[0].Confidence != .98 || r.ReviewPolicy.SkipConfidenceAt != .85 {
		t.Fatalf("%+v", r)
	}
}

func TestMissingFilesMakeChecklistUnknown(t *testing.T) {
	e := reviewEvidence("unnecessary", 1)
	e.PR.ChangedFiles = 3
	r := Aggregate(e, "jev-latest")
	if r.Status != "partial" || !r.Checklist[0].Unknown {
		t.Fatalf("%+v", r)
	}
}

func TestReviewExplanationUsesOnlyDecisiveChecks(t *testing.T) {
	e := reviewEvidence("caution", .28)
	e.Rules = append(e.Rules, model.Rule{ID: "custom", Title: "独自の確認項目", Tag: "custom", Priority: "high", SuggestAt: .75, CandidateAt: .35})
	e.Outcomes[0].Values["u1__custom"] = .9
	// A lower-priority suggestion and a high-priority candidate are not reasons for required.
	e.Outcomes[0].Values["u1__behavior_change"] = .9
	e.Outcomes[0].Values["u1__api_contract"] = .5
	r := Aggregate(e, "jev-latest")
	d := r.Files[0].Review
	if d.Level != "required" || d.Basis != "priority_check" || len(d.Checks) != 1 || d.Checks[0] != "独自の確認項目" || *d.Judgments[0].Confidence != .28 {
		t.Fatalf("%+v", d)
	}
	e.Files[0].PatchState = "unknown"
	d = Aggregate(e, "jev-latest").Files[0].Review
	if d.Basis != "incomplete" || len(d.Checks) != 0 {
		t.Fatalf("wrong decisive reason: %+v", d)
	}
}
