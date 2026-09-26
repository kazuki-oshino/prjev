package rules

import "github.com/kazuki-oshino/prjev/internal/model"

// Initial policy thresholds, not calibrated correctness probabilities.
const RiskSuggestAt = 0.75
const RiskCandidateAt = 0.35

// ReviewRisks is separate from the configurable checklist: a topic match is not severity.
func ReviewRisks() []model.Rule {
	return []model.Rule{
		{ID: "runtime_impact", Title: "利用者・データ・外部サービスへの重大な影響", Instructions: "Do the added or removed lines introduce a materially consequential change to actual product or operational behavior?",
			Criteria: map[string]string{
				"true":  "The change affects authorization or sensitive data handling, broadens destructive operations, changes consequential external side effects or failure recovery, or breaks compatibility. Renaming or removing a serialized response field (for example json:user_id to json:id), changing its type, or removing an API parameter is a compatibility break; downstream caller code need not be shown. A concrete serious correctness concern also counts. Real remote operations in tests and dangerous operational instructions count.",
				"false": "No materially consequential effect is shown. Merely touching an API or storage topic is insufficient. Mock JSON updates, isolated temporary fixtures, assertions describing risky behavior, compatible additions without sensitive effects, and ordinary local presentation changes do not qualify by themselves. Distinguish mock payloads from actual serialized declarations.",
			}},
		{ID: "verification_loss", Title: "重要なテストの検証が失われる可能性", Instructions: "Do the added or removed lines remove, bypass, or materially weaken an important automated test protection?",
			Criteria: map[string]string{
				"true":  "An important assertion or test is deleted, skipped, disabled, relaxed to accept incorrect behavior, or made unable to fail. Also count changes to test helpers, discovery, or CI configuration that disable important coverage, regardless of filename.",
				"false": "Coverage is only added or strengthened, equivalent assertions are preserved, or changes are limited to fixtures, names, or formatting without weakened pass/fail conditions. A product implementation change alone does not demonstrate lost test protection.",
			}},
	}
}
