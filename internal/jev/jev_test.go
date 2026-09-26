package jev

import (
	"context"
	"encoding/json"
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
		return response(200, `{"model":"jev-1","answers":{"u1__behavior":{"type":"noul","noul":0.8}},"usage":{"input_tokens":10,"output_tokens":1}}`), nil
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
