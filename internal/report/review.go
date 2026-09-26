package report

import (
	"fmt"
	"math"
	"strings"

	"github.com/kazuki-oshino/prjev/internal/model"
)

const reviewHelp = "必須: 重点的に確認 / 注意: 関連する変更を確認 / 不要: 詳細確認の省略候補"
const confidenceHelp = "確信度（confidence）はJevの判定の迷いの少なさです。コードが正しい確率ではありません。「不要」も確認の省略候補であり、無欠陥の保証ではありません。"

type reviewFile struct {
	Item                            model.FileResult
	Level, Tone, Reason, Confidence string
	Assessment                      aiAssessment
	DecisionNote                    string
}

type aiAssessment struct {
	Label, Confidence, ConfidenceLabel string
}

func assessment(js []model.ReviewJudgment) aiAssessment {
	if len(js) == 0 {
		return aiAssessment{Label: "見立てを取得できていません"}
	}
	choice, lowest, available := js[0].Choice, 1.0, true
	for _, j := range js {
		if choice != j.Choice {
			choice = "mixed"
		}
		if j.Confidence == nil {
			available = false
		} else {
			lowest = min(lowest, *j.Confidence)
		}
	}
	labels := map[string]string{"required": "重点的な確認が必要", "caution": "確認を勧める", "unnecessary": "詳細確認を省略できそう", "mixed": "範囲によって見立てが異なる"}
	a := aiAssessment{Label: labels[choice], Confidence: "未取得", ConfidenceLabel: "この見立ての確信度"}
	if a.Label == "" {
		a.Label = "見立てを取得できていません"
	}
	if len(js) > 1 {
		a.ConfidenceLabel = "各範囲の確信度の最低値"
	}
	if available {
		a.Confidence = percent(lowest)
	}
	return a
}

func decisionNote(f model.FileResult, level string) string {
	differs := false
	for _, j := range f.Review.Judgments {
		differs = differs || j.Choice != level
	}
	if !differs {
		return ""
	}
	switch f.Review.Basis {
	case "risk_signal", "risk_candidate", "uncertain_priority":
		return f.Review.Reason
	case "priority_check":
		return "重点確認項目への該当を優先し、確認区分を「必須」にしています。"
	case "attention_check":
		return "確認項目に該当するため、AIの見立てにかかわらず確認を勧めています。"
	case "incomplete":
		return "解析できなかった範囲も人が確認する必要があるため、確認区分は「必須」です。"
	case "low_confidence":
		return "省略できそうという見立ての確信度が基準に届かないため、確認区分は「注意」です。"
	case "ai_required":
		return "確認が必要という見立ての範囲があるため、ファイル全体の確認区分は「必須」です。"
	case "split_context":
		return "範囲ごとの見立てだけでは変更同士のつながりを判断できないため、確認を勧めています。"
	case "omitted_description":
		return "PRの説明を省いて解析したため、変更の意図と合わせた確認を勧めています。"
	default:
		return "確認項目や解析範囲の条件も合わせて、最終的な確認区分を決めています。"
	}
}

type reviewGroup struct {
	Level, Label, Description, Tone string
	Files                           []reviewFile
}
type reviewSummary struct {
	Required, Caution, Unnecessary, Missing int
	Groups                                  []reviewGroup
}

func summarize(r model.Result) reviewSummary {
	s := reviewSummary{Missing: max(0, r.PR.ChangedFiles-len(r.Files)), Groups: []reviewGroup{
		{Level: "required", Label: "必須", Description: "重点的に確認してください", Tone: "priority"},
		{Level: "caution", Label: "注意", Description: "関連する変更を確認してください", Tone: "manual"},
		{Level: "unnecessary", Label: "不要", Description: "詳細確認を省略できそうなファイル", Tone: "quiet"},
	}}
	s.Required = s.Missing
	for _, f := range r.Files {
		level, reason := f.Review.Level, f.Review.Reason
		if level != "required" && level != "caution" && level != "unnecessary" {
			level, reason = "caution", "確認の要否を判断した記録がありません。差分を確認してください。"
			if f.Group == "manual" || f.Group == "first" {
				level, reason = "required", "未解析の差分、または影響の大きい確認観点があります。"
			}
		}
		index := 0
		switch level {
		case "required":
			s.Required++
		case "caution":
			s.Caution++
			index = 1
		case "unnecessary":
			s.Unnecessary++
			index = 2
		}
		s.Groups[index].Files = append(s.Groups[index].Files, reviewFile{Item: f, Level: level, Tone: s.Groups[index].Tone, Reason: reason, Confidence: confidenceSummary(f.Review.Judgments), Assessment: assessment(f.Review.Judgments), DecisionNote: decisionNote(f, level)})
	}
	return s
}

func confidenceSummary(js []model.ReviewJudgment) string {
	if len(js) == 0 {
		return "Jevの確信度: 未取得"
	}
	choice, lowest := js[0].Choice, 1.0
	for _, j := range js {
		if j.Confidence == nil {
			return "Jevの確信度: 未取得"
		}
		lowest = min(lowest, *j.Confidence)
		if choice != j.Choice {
			choice = "mixed"
		}
	}
	prefix := "確信度"
	if len(js) > 1 {
		prefix = "最低の確信度"
	}
	return fmt.Sprintf("Jev: %s / %s %s", reviewLabel(choice), prefix, percent(lowest))
}

func percent(v float64) string { return fmt.Sprintf("%.1f%%", math.Floor(v*1000)/10) }
func reviewLabel(s string) string {
	switch s {
	case "required":
		return "必須"
	case "caution":
		return "注意"
	case "unnecessary":
		return "不要"
	case "mixed":
		return "判定が混在"
	default:
		return "未判定"
	}
}
func tagLabel(s string) string {
	labels := map[string]string{"behavior": "動作の変更", "integration": "外部サービスとの連携", "persistence": "データの保存・削除", "contract": "APIの互換性", "recovery": "エラーからの復旧", "test-contract": "テストの検証内容", "docs-impact": "説明・手順への影響", "review": "その他の動作変更"}
	if v, ok := labels[s]; ok {
		return v
	}
	return s
}
func reasonLabel(s string) string {
	if strings.HasSuffix(s, ": 1つのhunkが解析上限超過") {
		return "変更の一部が大きいため解析できませんでした"
	}
	switch s {
	case "未知の変更status":
		return "対応していない種類の変更です"
	case "patchなし（バイナリ、rename-only、mode変更等）":
		return "テキストの差分を取得できませんでした（画像、名前や権限だけの変更など）"
	case "patch構造または変更行数が不一致":
		return "取得した差分が不完全なため解析できませんでした"
	}
	labels := map[string]string{"api_error": "Jevから判定を取得できませんでした", "input_exceeded": "差分が解析できる大きさを超えています", "authentication": "Jevの認証に失敗しました", "rate_limited": "Jevの利用上限に達しました", "timeout": "解析が制限時間内に終わりませんでした", "budget_exceeded": "今回の解析量の上限を超えています", "unknown": "差分の判定を取得できませんでした", "review_unavailable": "確認の要否を判定できませんでした"}
	labels["risk_unavailable"] = "重大な影響やテストの検証について判定を取得できませんでした"
	if v, ok := labels[s]; ok {
		return v
	}
	return s
}
