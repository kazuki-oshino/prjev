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
	definitions := []struct{ id, title, tag, priority, question string }{
		{"behavior_change", "動作の変更と、それを確かめるテストを確認", "behavior", "normal", "Does the changed code alter conditions, calculations, or state transitions beyond naming or formatting?"},
		{"external_integration", "外部サービスとのやり取りを確認", "integration", "high", "Does the changed code alter external I/O request content, send conditions, ordering, or response handling?"},
		{"persistence_change", "データの保存・削除が意図どおりか確認", "persistence", "high", "Does the changed code alter persisted state, writes, deletes, or transaction behavior?"},
		{"api_contract", "APIの変更が利用側に与える影響を確認", "contract", "high", "Does the changed code alter an externally exposed request, response, schema, or API contract?"},
		{"error_recovery", "失敗したときや、やり直したときの動作を確認", "recovery", "high", "Does the changed code alter error handling, retries, timeouts, or recovery behavior?"},
		{"test_guarantee", "テストが意図した動作を確かめているか確認", "test-contract", "normal", "Does the changed test alter an important verification or suggest a mismatch between its claimed behavior and assertions?"},
		{"docs_impact", "関連する説明や手順も更新する必要があるか確認", "docs-impact", "normal", "Does the changed behavior alter a user or developer facing contract, setting, or procedure that may need explanation?"},
		{"general_attention", "その他の動作の変更を確認", "review", "normal", "Does the changed code show a substantive behavior change worth manual review, including beyond the named categories?"},
	}
	out := make([]model.Rule, 0, len(definitions))
	for _, d := range definitions {
		out = append(out, model.Rule{ID: d.id, Title: d.title, Tag: d.tag, Priority: d.priority, Instructions: d.question,
			Criteria: map[string]string{"true": "The changed lines support this review concern.", "false": "The supplied changed lines do not support it. Names, comments and PR prose alone are insufficient."}, SuggestAt: .75, CandidateAt: .35})
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
