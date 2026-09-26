package jev

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/joho/godotenv"
	"github.com/kazuki-oshino/prjev/internal/model"
)

// TestLiveWire is opt-in and sends only a synthetic diff to TypeSafe.
func TestLiveWire(t *testing.T) {
	if os.Getenv("PRADAR_LIVE_PROBE") != "1" {
		t.Skip("set PRADAR_LIVE_PROBE=1 to run")
	}
	key := os.Getenv("TYPESAFE_API_KEY")
	if key == "" {
		m, err := godotenv.Read("../../.env")
		if err != nil {
			t.Skip("no local key")
		}
		key = m["TYPESAFE_API_KEY"]
	}
	if key == "" {
		t.Skip("no local key")
	}
	rs := []model.Rule{{ID: "behavior_change", Instructions: "Does the changed code show a behavior change?", Criteria: map[string]string{"true": "The changed lines show a behavior change.", "false": "The changed lines do not show one."}}}
	req := MakeRequest(model.PR{Title: "Synthetic test", Body: "No private data"}, []model.Unit{{ID: "u0001", Path: "sample.go", Patch: "@@ -1 +1 @@\n-old()\n+new()\n"}}, rs, "jev-latest")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, err := New(key, 8*time.Second).one(ctx, req)
	if err != nil {
		t.Fatalf("live wire: %v", err)
	}
	if resp.Model == "" || resp.Usage == nil || resp.Answers["u0001__behavior_change"].Noul == nil {
		t.Fatalf("unexpected response structure")
	}
	t.Logf("model=%s input_tokens=%d", resp.Model, resp.Usage.InputTokens)
}
