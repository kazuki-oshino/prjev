package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strings"
	"unicode"

	"github.com/kazuki-oshino/prjev/internal/model"
)

var ansi = regexp.MustCompile(`\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\x07]*(?:\x07|\x1b\\))`)

func clean(s string) string {
	s = ansi.ReplaceAllString(s, "")
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return ' '
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}
func md(s string) string {
	s = html.EscapeString(clean(s))
	r := strings.NewReplacer("\\", "\\\\", "[", "\\[", "]", "\\]", "*", "\\*", "_", "\\_", "`", "\\`", "|", "\\|", "#", "\\#", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}
func label(s string) string {
	switch s {
	case "suggested":
		return "確認したい"
	case "candidate":
		return "必要に応じて"
	case "unknown":
		return "未判定"
	case "manual":
		return "要手動確認"
	case "first":
		return "先に確認"
	case "normal":
		return "通常確認"
	default:
		return "追加提案なし"
	}
}
func Render(r model.Result, format, view string) ([]byte, error) {
	if format == "json" {
		return json.MarshalIndent(r, "", "  ")
	}
	if format == "html" {
		return renderHTML(r, view)
	}
	if format != "terminal" && format != "markdown" {
		return nil, fmt.Errorf("不明な出力形式: %s", format)
	}
	if format == "markdown" {
		return markdown(r, view), nil
	}
	return terminal(r, view), nil
}
func short(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}
func terminal(r model.Result, view string) []byte { return textReport(r, view, false) }

func targetRange(t model.Target) string {
	parts := []string{}
	if len(t.OldRanges) > 0 {
		parts = append(parts, "旧側"+strings.Join(t.OldRanges, ","))
	}
	if len(t.NewRanges) > 0 {
		parts = append(parts, "新側"+strings.Join(t.NewRanges, ","))
	}
	return strings.Join(parts, " / ")
}
func markdown(r model.Result, view string) []byte { return textReport(r, view, true) }

func textReport(r model.Result, view string, markdown bool) []byte {
	var b bytes.Buffer
	escape := clean
	heading := ""
	if markdown {
		escape, heading = md, "## "
	}
	titlePrefix := ""
	if markdown {
		titlePrefix = "# "
	}
	fmt.Fprintf(&b, "%sPR #%d %s\n\n%s\n%s\n\n", titlePrefix, r.PR.Number, escape(r.PR.Title), escape(r.PR.Repository), escape(r.PR.URL))
	summary := summarize(r)
	fmt.Fprintf(&b, "人の確認: 必須 %d / 注意 %d / 不要 %d ファイル\n%s\n\n", summary.Required, summary.Caution, summary.Unnecessary, reviewHelp)
	fmt.Fprintf(&b, "分析範囲: %dファイル中%dファイルの差分を解析 / %dファイル未解析\n", r.PR.ChangedFiles, r.Scope.AnalyzedFiles, max(0, r.PR.ChangedFiles-r.Scope.AnalyzedFiles))
	if r.Status != "complete" {
		b.WriteString("処理状態: 一部未解析\n")
	} else {
		b.WriteString("処理状態: 解析完了\n")
	}
	fmt.Fprintf(&b, "base: %s / head: %s\n%s\n\n", escape(short(r.PR.BaseSHA)), escape(short(r.PR.HeadSHA)), confidenceHelp)
	if summary.Missing > 0 {
		fmt.Fprintf(&b, "一覧を取得できなかった%dファイルは確認必須です。GitHubで確認してください。\n\n", summary.Missing)
	}
	if view != "checklist" {
		fmt.Fprintf(&b, "%sファイルごとの確認の要否\n\n", heading)
		for _, g := range summary.Groups {
			if len(g.Files) == 0 {
				continue
			}
			if markdown && g.Level == "unnecessary" {
				fmt.Fprintf(&b, "<details>\n<summary>不要: %dファイル（確認の省略候補）</summary>\n\n", len(g.Files))
			} else {
				fmt.Fprintf(&b, "%s — %s (%dファイル)\n\n", g.Label, g.Description, len(g.Files))
			}
			for _, f := range g.Files {
				fmt.Fprintf(&b, "- [%s] %s — %s\n", g.Label, escape(f.Item.Path), escape(f.Confidence))
				if g.Level == "unnecessary" && !markdown {
					continue
				}
				fmt.Fprintf(&b, "  %s\n", escape(f.Reason))
				if f.Item.PreviousPath != "" {
					fmt.Fprintf(&b, "  変更前: %s\n", escape(f.Item.PreviousPath))
				}
				if f.Item.ContextPartial {
					b.WriteString("  差分を分割して解析しました。変更同士のつながりは確認が必要です。\n")
				}
				for _, tag := range f.Item.Tags {
					fmt.Fprintf(&b, "  - %s\n", escape(tagLabel(tag)))
				}
				for _, reason := range f.Item.Reasons {
					fmt.Fprintf(&b, "  - %s\n", escape(reasonLabel(reason)))
				}
				for _, t := range f.Item.Targets {
					fmt.Fprintf(&b, "  - %s\n", escape(targetRange(t)))
				}
				if len(f.Item.Review.Judgments) > 1 {
					for _, j := range f.Item.Review.Judgments {
						fmt.Fprintf(&b, "  - %s: %s\n", escape(targetRange(j.Target)), escape(confidenceSummary([]model.ReviewJudgment{j})))
					}
				}
			}
			if markdown && g.Level == "unnecessary" {
				b.WriteString("\n</details>\n")
			}
			b.WriteByte('\n')
		}
	}
	if view != "files" {
		fmt.Fprintf(&b, "%s確認する観点\n\n", heading)
		shown := 0
		for _, x := range r.Checklist {
			if x.State == "no_signal" {
				continue
			}
			shown++
			fmt.Fprintf(&b, "- [%s] %s", label(x.State), escape(x.Title))
			if x.Unknown {
				b.WriteString("（未解析範囲あり）")
			}
			b.WriteByte('\n')
			for _, t := range x.Targets {
				fmt.Fprintf(&b, "  - %s — %s\n", escape(t.Path), escape(targetRange(t)))
			}
		}
		if shown == 0 {
			if r.Status != "complete" {
				b.WriteString("解析済み範囲では追加提案なし。未解析の差分があります\n")
			} else {
				b.WriteString("今回の差分から追加提案なし\n")
			}
		}
		b.WriteByte('\n')
	}
	fmt.Fprintf(&b, "経過時間: GitHub %dms / Jev %dms / ローカル %dms / 全体 %dms / Jev照会 %d回\n", r.Metrics.GitHubMS, r.Metrics.JevMS, r.Metrics.LocalMS, r.Metrics.TotalMS, r.Metrics.HTTPAttempts)
	if r.Metrics.Usage != nil {
		fmt.Fprintf(&b, "入力トークン: %d\n", r.Metrics.Usage.InputTokens)
	}
	for _, w := range r.Warnings {
		fmt.Fprintf(&b, "補足: %s\n", escape(w))
	}
	b.WriteString("この結果は差分に基づく確認の目安です。バグ検出・テスト実行・マージ承認ではありません。\n")
	if markdown {
		// Preserve the concise terminal layout as readable lines in Markdown.
		lines := strings.Split(b.String(), "\n")
		for i, line := range lines {
			if line != "" && !strings.HasPrefix(line, "<") && !strings.HasPrefix(line, "#") {
				lines[i] += "  "
			}
		}
		return []byte(strings.Join(lines, "\n"))
	}
	return b.Bytes()
}
