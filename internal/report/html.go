package report

import (
	"bytes"
	"embed"
	"html/template"

	"github.com/kazuki-oshino/prjev/internal/github"
	"github.com/kazuki-oshino/prjev/internal/model"
)

//go:embed report.html report.css
var htmlAssets embed.FS

type htmlChecklistItem struct {
	Item model.ChecklistItem
	Tone string
}

type htmlReport struct {
	Result           model.Result
	CSS              template.CSS
	PRURL            string
	StatusLabel      string
	StatusTone       string
	UnanalyzedCount  int
	Review           reviewSummary
	ShowChecklist    bool
	ShowFiles        bool
	Checklist        []htmlChecklistItem
	ChecklistMessage string
}

func safePRURL(raw string) string {
	ref, err := github.ParseRef(raw)
	if err != nil {
		return ""
	}
	return ref.URL
}

func renderHTML(r model.Result, view string) ([]byte, error) {
	css, err := htmlAssets.ReadFile("report.css")
	if err != nil {
		return nil, err
	}
	d := htmlReport{
		Result:          r,
		Review:          summarize(r),
		CSS:             template.CSS(css), // 埋め込んだ固定CSSのみを渡す。
		PRURL:           safePRURL(r.PR.URL),
		StatusLabel:     "解析完了",
		StatusTone:      "complete",
		UnanalyzedCount: max(0, r.PR.ChangedFiles-r.Scope.AnalyzedFiles),
		ShowChecklist:   view != "files",
		ShowFiles:       view != "checklist",
	}
	if r.Status != "complete" {
		d.StatusLabel = "一部未解析"
		d.StatusTone = "partial"
	}
	for _, item := range r.Checklist {
		if item.State != "no_signal" {
			d.Checklist = append(d.Checklist, htmlChecklistItem{Item: item, Tone: htmlTone(item.State)})
		}
	}
	if r.Status == "complete" {
		d.ChecklistMessage = "今回の差分から追加提案なし"
	} else {
		d.ChecklistMessage = "解析済み範囲では追加提案なし。未解析の差分があります"
	}
	t, err := template.New("report.html").Funcs(template.FuncMap{
		"clean":             clean,
		"tagLabel":          tagLabel,
		"reasonLabel":       reasonLabel,
		"reviewLabel":       reviewLabel,
		"confidenceSummary": func(j model.ReviewJudgment) string { return confidenceSummary([]model.ReviewJudgment{j}) },
		"confidenceHelp":    func() string { return confidenceHelp },
		"label":             label,
		"short":             short,
		"targetRange":       targetRange,
	}).ParseFS(htmlAssets, "report.html")
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	if err := t.Execute(&b, d); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func htmlTone(state string) string {
	switch state {
	case "suggested", "first":
		return "priority"
	case "candidate", "normal":
		return "candidate"
	case "unknown", "manual":
		return "manual"
	default:
		return "quiet"
	}
}
