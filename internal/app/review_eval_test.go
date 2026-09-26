package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/kazuki-oshino/prjev/internal/app"
	"github.com/kazuki-oshino/prjev/internal/config"
	"github.com/kazuki-oshino/prjev/internal/diff"
	"github.com/kazuki-oshino/prjev/internal/github"
	"github.com/kazuki-oshino/prjev/internal/jev"
	"github.com/kazuki-oshino/prjev/internal/model"
	"github.com/kazuki-oshino/prjev/internal/rules"
	"github.com/kazuki-oshino/prjev/internal/store"
)

type reviewCase struct {
	ID    string   `json:"id"`
	Path  string   `json:"path"`
	Patch string   `json:"patch"`
	Want  []string `json:"want"`
}

func evaluationEvidence(t *testing.T) (app.Evidence, []reviewCase) {
	t.Helper()
	b, err := os.ReadFile("testdata/review_cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []reviewCase
	if err := json.Unmarshal(b, &cases); err != nil {
		t.Fatal(err)
	}
	e := app.Evidence{ReviewPolicyVersion: 2, PR: model.PR{Title: "変更差分のレビュー優先度評価", ChangedFiles: len(cases)}, Rules: rules.Standard()}
	for i, c := range cases {
		hs, adds, dels, err := diff.ParseHunks(c.Patch)
		if err != nil {
			t.Fatalf("%s: %v", c.ID, err)
		}
		// Normalize separately so repeated paths are independent evaluation cases.
		fs, us := diff.Normalize([]github.APIFile{{Filename: c.Path, Status: "modified", Patch: &c.Patch, Additions: adds, Deletions: dels, Changes: adds + dels}})
		if len(us) != 1 || fs[0].PatchState != "complete" || len(hs) == 0 {
			t.Fatalf("%s: invalid fixture", c.ID)
		}
		fs[0].ID, us[0].FileID, us[0].ID = fmt.Sprintf("f%04d", i+1), fmt.Sprintf("f%04d", i+1), fmt.Sprintf("u%04d", i+1)
		fs[0].Units = []string{us[0].ID}
		// Result paths identify cases without leaking expectations to the request state.
		fs[0].Path = c.ID
		e.Files = append(e.Files, fs[0])
		e.Units = append(e.Units, us[0])
		if len(c.Want) == 0 {
			t.Fatalf("case %d has no expectation", i)
		}
	}
	return e, cases
}

func TestReviewEvaluationFixtures(t *testing.T) { evaluationEvidence(t) }

// Opt-in live quality evaluation; ordinary tests do not call TypeSafe.
func TestLiveReviewEvaluation(t *testing.T) {
	if os.Getenv("PRADAR_REVIEW_EVAL") != "1" {
		t.Skip("set PRADAR_REVIEW_EVAL=1 for live evaluation")
	}
	e, cases := evaluationEvidence(t)
	c, err := config.Load("", "../../.env", false)
	if err != nil {
		t.Fatal(err)
	}
	if m := os.Getenv("PRADAR_REVIEW_EVAL_MODEL"); m != "" {
		c.Model = m
	}
	batches, skipped := jev.Plan(e.PR, e.Units, e.Rules, c.Model)
	if len(skipped) != 0 || len(batches) == 0 {
		t.Fatal("evaluation exceeded budget")
	}
	client := jev.New(c.Key, c.RequestTimeout)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	start := time.Now()
	e.Outcomes = client.Run(ctx, batches, c.Concurrency)
	e.Metrics.JevMS = time.Since(start).Milliseconds()
	e.Metrics.HTTPAttempts = int(client.Attempts.Load())
	r := app.Aggregate(e, c.Model)
	if path := os.Getenv("PRADAR_REVIEW_EVAL_RECORD"); path != "" {
		if err := store.Save(path, store.Record{RecordSchemaVersion: 1, Evidence: e, Result: r}); err != nil {
			t.Fatal(err)
		}
	}
	if r.Status != "complete" {
		t.Fatalf("incomplete evaluation: %+v", r.Scope.Unanalyzed)
	}
	wants := map[string][]string{}
	for _, c := range cases {
		wants[c.ID] = c.Want
	}
	for _, f := range r.Files {
		d := f.Review
		t.Logf("%s: %s (%s), choice=%s confidence=%.2f", f.Path, d.Level, d.Basis, d.Judgments[0].Choice, *d.Judgments[0].Confidence)
		if !slices.Contains(wants[f.Path], d.Level) {
			t.Errorf("%s: got %s, want one of %v; risks=%+v", f.Path, d.Level, wants[f.Path], d.Risks)
		}
	}
	t.Logf("models=%v calls=%d usage=%+v", r.Model.Actual, r.Metrics.HTTPAttempts, r.Metrics.Usage)
}
