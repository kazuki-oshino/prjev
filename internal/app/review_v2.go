package app

import (
	"github.com/kazuki-oshino/prjev/internal/jev"
	"github.com/kazuki-oshino/prjev/internal/model"
	"github.com/kazuki-oshino/prjev/internal/rules"
)

// Only a clear Choice can determine priority on its own. Independent risk evidence
// still escalates consequential changes even when the Choice is uncertain.
const requiredConfidenceAt = 0.85

func reviewDecisionV2(f model.File, fr model.FileResult, choices map[string]model.ChoiceAnswer, values map[string]float64, units map[string]model.Unit, bodyOmitted bool) model.ReviewDecision {
	d := model.ReviewDecision{Level: "caution", Judgments: []model.ReviewJudgment{}}
	strongRequired, uncertainRequired, caution, lowConfidence := false, false, false, false
	var priorityChecks, candidateChecks []string
	for _, uid := range f.Units {
		if a, ok := choices[jev.ReviewQuestionID(uid)]; ok {
			d.Judgments = append(d.Judgments, model.ReviewJudgment{UnitID: uid, ChoiceAnswer: a, Target: target(f, units[uid])})
			strongRequired = strongRequired || (a.Choice == "required" && *a.Confidence >= requiredConfidenceAt)
			uncertainRequired = uncertainRequired || a.Choice == "required"
			caution = caution || a.Choice == "caution"
			lowConfidence = lowConfidence || *a.Confidence < skipConfidenceAt
		}
		for _, risk := range rules.ReviewRisks() {
			if v, ok := values[jev.RiskQuestionID(uid, risk.ID)]; ok {
				d.Risks = append(d.Risks, model.ReviewRisk{UnitID: uid, ID: risk.ID, Title: risk.Title, Value: v, Target: target(f, units[uid])})
				if v >= rules.RiskSuggestAt {
					priorityChecks = appendUnique(priorityChecks, risk.Title)
				} else if v >= rules.RiskCandidateAt {
					candidateChecks = appendUnique(candidateChecks, risk.Title)
				}
			}
		}
	}
	switch {
	case fr.Group == "manual":
		d.Level, d.Basis, d.Reason = "required", "incomplete", "解析できていない差分、または取得できなかった判定があります。人が内容を確認してください。"
	case len(priorityChecks) > 0:
		d.Level, d.Basis, d.Checks = "required", "risk_signal", priorityChecks
		d.Reason = "実際の重大な影響、または重要なテストの検証が失われる可能性があります。重点的に確認してください。"
	case strongRequired:
		d.Level, d.Basis, d.Reason = "required", "ai_required", "Jevが高い確信度で重点的な確認を勧めています。対象の変更を確認してください。"
	case len(d.Judgments) != len(f.Units) || len(d.Judgments) == 0:
		d.Basis, d.Reason = "missing_assessment", "確認の要否を判断した記録がありません。確認を省略せず、差分を確認してください。"
	case len(candidateChecks) > 0:
		d.Basis, d.Checks = "risk_candidate", candidateChecks
		d.Reason = "重大な影響や検証の弱体化がないか、判断が分かれています。関連する変更を確認してください。"
	case uncertainRequired:
		d.Basis, d.Reason = "uncertain_priority", "Jevは重点確認を選びましたが、確信度が基準に届かず、重大な影響の明確な根拠もないため注意としています。"
	case fr.ContextPartial || len(f.Units) > 1:
		d.Basis, d.Reason = "split_context", "差分を分割して解析しています。変更同士のつながりを確認してください。"
	case bodyOmitted:
		d.Basis, d.Reason = "omitted_description", "PRの説明を省いて解析しています。変更の意図と合わせて確認してください。"
	case caution:
		d.Basis, d.Reason = "ai_caution", "Jevが確認を勧めています。変更の意図と影響を確認してください。"
	case lowConfidence:
		d.Basis, d.Reason = "low_confidence", "Jevは不要を選びましたが、判定の確かさが基準に届かないため注意としています。"
	default:
		d.Level, d.Basis = "unnecessary", "skip_eligible"
		d.Reason = "Jevが高い確信度で影響の限られた変更と判断し、重大な影響や検証の弱体化も示されていないため、省略候補としています。"
	}
	return d
}
