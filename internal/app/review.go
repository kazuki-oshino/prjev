package app

import (
	"github.com/kazuki-oshino/prjev/internal/jev"
	"github.com/kazuki-oshino/prjev/internal/model"
)

// Initial conservative policy; calibrate against reviewed PRs before lowering it.
const skipConfidenceAt = 0.85

func reviewDecision(f model.File, fr model.FileResult, choices map[string]model.ChoiceAnswer, units map[string]model.Unit, bodyOmitted bool) model.ReviewDecision {
	d := model.ReviewDecision{Level: "caution", Judgments: []model.ReviewJudgment{}}
	required, caution, lowConfidence := false, false, false
	for _, uid := range f.Units {
		a, ok := choices[jev.ReviewQuestionID(uid)]
		if !ok {
			continue
		}
		d.Judgments = append(d.Judgments, model.ReviewJudgment{UnitID: uid, ChoiceAnswer: a, Target: target(f, units[uid])})
		required = required || a.Choice == "required"
		caution = caution || a.Choice == "caution"
		lowConfidence = lowConfidence || *a.Confidence < skipConfidenceAt
	}
	switch {
	case fr.Group == "manual":
		d.Level, d.Reason = "required", "解析できていない差分があります。人が内容を確認してください。"
	case fr.Group == "first":
		d.Level, d.Reason = "required", "影響の大きい確認観点に該当します。対象の変更を確認してください。"
	case required:
		d.Level, d.Reason = "required", "Jevが重点的な確認を必要と判断しました。対象の変更を確認してください。"
	case fr.Group == "normal":
		d.Reason = "確認したい観点があります。関連する変更を確認してください。"
	case len(d.Judgments) != len(f.Units) || len(d.Judgments) == 0:
		d.Reason = "確認の要否を判断した記録がありません。確認を省略せず、差分を確認してください。"
	case fr.ContextPartial || len(f.Units) > 1:
		d.Reason = "差分を分割して解析しています。変更同士のつながりを確認してください。"
	case bodyOmitted:
		d.Reason = "PRの説明を省いて解析しています。変更の意図と合わせて確認してください。"
	case caution:
		d.Reason = "Jevが確認を勧めています。変更の意図と影響を確認してください。"
	case lowConfidence:
		d.Reason = "Jevは不要を選びましたが、判定の確かさが基準に届かないため注意としています。"
	default:
		d.Level, d.Reason = "unnecessary", "Jevが確認の省略候補と判断し、他の確認観点にも該当しません。"
	}
	return d
}
