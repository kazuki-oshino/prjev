package jev

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kazuki-oshino/prjev/internal/model"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(code int, body string) *http.Response {
	return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}
}
func TestClientValidatesAllAnswers(t *testing.T) {
	pr := model.PR{Title: "test"}
	units := []model.Unit{{ID: "u1", Path: "a", Patch: "+x"}}
	rules := []model.Rule{{ID: "behavior", Instructions: "change?", Criteria: map[string]string{"true": "yes", "false": "no"}}}
	request := MakeRequest(pr, units, rules, "jev-latest")
	c := New("private", time.Second)
	c.HTTP.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "Bearer private" || r.URL.Host != "api.typesafe.ai" {
			t.Fatal("request target/auth")
		}
		var v map[string]any
		_ = json.NewDecoder(r.Body).Decode(&v)
		if v["state"] == nil {
			t.Fatal("missing state")
		}
		return response(200, `{"model":"jev-1","answers":{"u1__behavior":{"type":"noul","noul":0.8},"u1___risk_runtime_impact":{"type":"noul","noul":0.1},"u1___risk_verification_loss":{"type":"noul","noul":0.1},"u1___review":{"type":"choice","choice":"caution","confidence":0.7,"probabilities":{"required":0.1,"caution":0.85,"unnecessary":0.05}}},"usage":{"input_tokens":10,"output_tokens":1}}`), nil
	})
	out, e := c.one(context.Background(), request)
	if e != nil || out.Model != "jev-1" {
		t.Fatalf("%+v %v", out, e)
	}
	c.HTTP.Transport = roundTrip(func(*http.Request) (*http.Response, error) {
		return response(200, `{"model":"jev-1","answers":{"u1__behavior":{"type":"noul"}},"usage":{"input_tokens":10,"output_tokens":1}}`), nil
	})
	if _, e = c.one(context.Background(), request); e == nil {
		t.Fatal("missing value accepted")
	}
}
func TestPlanLimits(t *testing.T) {
	rs := []model.Rule{{ID: "r", Instructions: "x", Criteria: map[string]string{"true": "x", "false": "x"}}}
	units := []model.Unit{{ID: "u1", Path: "a", Patch: strings.Repeat("x", 25<<10)}}
	b, skipped := Plan(model.PR{}, units, rs, "jev-latest")
	if len(b) != 0 || len(skipped) != 1 {
		t.Fatalf("batches=%d skipped=%d", len(b), len(skipped))
	}
}

func TestChoiceValidationAndPersistence(t *testing.T) {
	req := MakeRequest(model.PR{}, []model.Unit{{ID: "u1", Path: "a.go"}}, nil, "jev-latest")
	valid := `{"type":"choice","choice":"unnecessary","confidence":0.93,"probabilities":{"required":0.01,"caution":0.02,"unnecessary":0.97}}`
	tests := []struct {
		name, answer string
		valid        bool
	}{
		{"valid", valid, true},
		{"zero confidence", strings.Replace(valid, "0.93", "0", 1), true},
		{"missing confidence", strings.Replace(valid, `"confidence":0.93,`, "", 1), false},
		{"null confidence", strings.Replace(valid, "0.93", "null", 1), false},
		{"out of range", strings.Replace(valid, "0.93", "1.1", 1), false},
		{"wrong choice", strings.Replace(valid, `"choice":"unnecessary"`, `"choice":"other"`, 1), false},
		{"wrong type", strings.Replace(valid, `"type":"choice"`, `"type":"noul"`, 1), false},
		{"missing option", strings.Replace(valid, `"caution":0.02,`, "", 1), false},
		{"bad total", strings.Replace(valid, "0.97", "0.90", 1), false},
		{"wrong winner", strings.Replace(valid, `"choice":"unnecessary"`, `"choice":"caution"`, 1), false},
		{"negative probability", strings.Replace(valid, "0.01", "-0.01", 1), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New("test", time.Second)
			c.HTTP.Transport = roundTrip(func(*http.Request) (*http.Response, error) {
				return response(200, `{"model":"jev-test","answers":{"u1___risk_runtime_impact":{"type":"noul","noul":0.1},"u1___risk_verification_loss":{"type":"noul","noul":0.1},"u1___review":`+tt.answer+`},"usage":{"input_tokens":1,"output_tokens":1}}`), nil
			})
			out := c.Run(context.Background(), []Batch{{ID: "b1", Units: []model.Unit{{ID: "u1"}}, Request: req}}, 1)[0]
			if (out.ErrorCode == "") != tt.valid {
				t.Fatalf("%+v", out)
			}
			if tt.valid && out.Choices[ReviewQuestionID("u1")].Confidence == nil {
				t.Fatal("lost confidence")
			}
			if !tt.valid && (len(out.Choices) > 0 || len(out.Values) > 0) {
				t.Fatal("invalid answers persisted")
			}
		})
	}
}

func TestMixedBatchSplitKeepsChoiceQuestions(t *testing.T) {
	units := []model.Unit{{ID: "u1", Path: "a.go"}, {ID: "u2", Path: "b.go"}}
	rs := []model.Rule{{ID: "review", Instructions: "change?", Criteria: map[string]string{"true": "yes", "false": "no"}}}
	req := MakeRequest(model.PR{}, units, rs, "jev-latest")
	if len(req.Questions) != 8 {
		t.Fatal("review rule collided with triage")
	}
	c := New("test", time.Second)
	c.HTTP.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		var wire Request
		if err := json.NewDecoder(r.Body).Decode(&wire); err != nil {
			t.Fatal(err)
		}
		if len(wire.State.Units) > 1 {
			return response(413, `{}`), nil
		}
		id := wire.State.Units[0].ID
		if len(wire.Questions) != 4 || wire.Questions[ReviewQuestionID(id)].Type != "choice" {
			t.Fatal("lost choice on split")
		}
		answers := map[string]any{id + "__review": map[string]any{"type": "noul", "noul": .1}, ReviewQuestionID(id): map[string]any{"type": "choice", "choice": "unnecessary", "confidence": .9, "probabilities": map[string]float64{"required": .01, "caution": .01, "unnecessary": .98}}}
		for qid, q := range wire.Questions {
			if !strings.HasPrefix(q.Instructions, unitScope(0)) || strings.Contains(q.Instructions, "`units[1]") {
				t.Fatalf("stale unit reference after split: %s", q.Instructions)
			}
			if q.Type == "noul" {
				answers[qid] = map[string]any{"type": "noul", "noul": .1}
			}
		}
		b, _ := json.Marshal(map[string]any{"model": "jev-test", "answers": answers, "usage": model.Usage{InputTokens: 1}})
		return response(200, string(b)), nil
	})
	out := c.Run(context.Background(), []Batch{{ID: "b1", Units: units, Request: req}}, 1)
	if len(out) != 2 || c.Attempts.Load() != 3 {
		t.Fatalf("%+v", out)
	}
	for _, o := range out {
		if o.ErrorCode != "" || len(o.Choices) != 1 || len(o.Values) != 3 {
			t.Fatalf("%+v", o)
		}
	}
}

func TestPlanIncludesReviewQuestionInBudget(t *testing.T) {
	var units []model.Unit
	for i := 0; i < 100; i++ {
		units = append(units, model.Unit{ID: fmt.Sprintf("u%03d", i), Path: "a.go"})
	}
	batches, skipped := Plan(model.PR{}, units, []model.Rule{{ID: "r"}}, "jev-latest")
	if len(skipped) != 0 || len(batches) < 2 {
		t.Fatalf("batches=%d skipped=%d", len(batches), len(skipped))
	}
	for _, b := range batches {
		if len(b.Request.Questions) > 128 || len(b.Request.Questions) != 4*len(b.Units) {
			t.Fatal("question budget bypassed")
		}
	}
}
