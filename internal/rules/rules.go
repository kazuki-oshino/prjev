package rules

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/kazuki-oshino/prjev/internal/model"
)

var idRE = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

func Standard() []model.Rule {
	definitions := []struct{ id, title, tag, priority, question, yes, no string }{
		{"behavior_change", "動作の変更と、それを確かめるテストを確認", "behavior", "normal", "Do the added or removed lines change product conditions, calculations, or state transitions?", "Actual implementation behavior changes. In tests, count only shared implementation code, not assertions or fixture setup.", "Formatting, naming, or changes only to test cases, mocks, assertions, or examples; testing behavior is not changing that behavior."},
		{"external_integration", "外部サービスとのやり取りを確認", "integration", "high", "Do the added or removed lines change real external-service requests or response handling?", "Real request content, send conditions, ordering, or response handling changes. A test that actually contacts an external service can count.", "Only mock transports, simulated responses, fixtures, or assertions change. Exercising a client in a test does not itself change the real integration."},
		{"persistence_change", "データの保存・削除が意図どおりか確認", "persistence", "high", "Do the added or removed lines change actual persistent data writes, deletes, or transactions?", "Product or operational storage behavior changes, including tests that operate on real shared data.", "Only test fixtures in isolated temporary storage, mocks, or assertions about saved data change; these do not themselves change product persistence."},
		{"api_contract", "APIの変更が利用側に与える影響を確認", "contract", "high", "Do the added or removed lines change the actual implementation or declaration of an externally consumed API, serialized output, or persisted schema?", "Actual public request, response, schema, or API declarations change, including shared declarations used by tests and the product.", "Only mock JSON responses, test expectations, fixtures, samples, or assertions describing a contract change; these do not themselves change that contract."},
		{"error_recovery", "失敗したときや、やり直したときの動作を確認", "recovery", "high", "Do the added or removed lines change actual product or operational error handling, retries, timeouts, or recovery?", "The implementation of failure handling or recovery changes, including real operations initiated by a test.", "Only failure simulations, mock errors, assertions, or tests of existing recovery behavior change."},
		{"test_guarantee", "テストが意図した動作を確かめているか確認", "test-contract", "normal", "Do the added or removed lines change what automated tests verify?", "Test assertions, expected results, coverage, skips, or pass/fail conditions are added or changed. This identifies a review topic, not a defect or weakened guarantee.", "No test verification changes are shown; only test names, formatting, or incidental fixture setup change. Product code changes alone are insufficient."},
		{"docs_impact", "関連する説明や手順も更新する必要があるか確認", "docs-impact", "normal", "Do the added or removed lines introduce or change a user or developer facing contract, setting, or procedure that needs explanation?", "Actual usage, configuration, or operational instructions change. For tests, changed commands, prerequisites, or real environment setup can count.", "Only test cases, mocks, or assertions change without changing how users or developers use the system; prose typos and formatting alone do not count."},
		{"general_attention", "その他の動作の変更を確認", "review", "normal", "Do the added or removed lines show another substantive implementation or verification change worth examining?", "A concrete behavioral or verification change, including meaningful assertions or execution changes in tests. Identify relevance, not mandatory priority.", "Only mechanical maintenance, routine fixture additions, test names, formatting, or prose typos change; a filename or mention of risky behavior alone does not count."},
	}
	out := make([]model.Rule, 0, len(definitions))
	for _, d := range definitions {
		out = append(out, model.Rule{ID: d.id, Title: d.title, Tag: d.tag, Priority: d.priority, Instructions: d.question,
			Criteria: map[string]string{"true": d.yes, "false": d.no}, SuggestAt: .75, CandidateAt: .35})
	}
	return out
}
func Validate(rs []model.Rule) error {
	if len(rs) > 20 {
		return fmt.Errorf("確認観点は最大20件です")
	}
	seen := map[string]bool{}
	for _, r := range rs {
		if !idRE.MatchString(r.ID) || seen[r.ID] {
			return fmt.Errorf("不正または重複した観点ID: %q", r.ID)
		}
		seen[r.ID] = true
		if r.Title == "" || r.Tag == "" || r.Instructions == "" || r.Criteria["true"] == "" || r.Criteria["false"] == "" {
			return fmt.Errorf("観点 %s の必須項目がありません", r.ID)
		}
		if r.Priority != "high" && r.Priority != "normal" {
			return fmt.Errorf("観点 %s のpriorityが不正です", r.ID)
		}
		if r.CandidateAt < 0 || r.CandidateAt >= r.SuggestAt || r.SuggestAt > 1 {
			return fmt.Errorf("観点 %s の閾値が不正です", r.ID)
		}
	}
	return nil
}
func Hash(rs []model.Rule) string {
	b, _ := json.Marshal(rs)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
