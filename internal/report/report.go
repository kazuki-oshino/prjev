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
		return "推奨"
	case "candidate":
		return "候補"
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
func terminal(r model.Result, view string) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "PR #%d %s\n%s  base: %s  head: %s\n%s\n\n", r.PR.Number, clean(r.PR.Title), clean(r.PR.Repository), short(r.PR.BaseSHA), short(r.PR.HeadSHA), clean(r.PR.URL))
	fmt.Fprintf(&b, "分析範囲: %dファイル中%dファイルの差分を解析 / %dファイル未解析\n処理状態: %s\n\n", r.PR.ChangedFiles, r.Scope.AnalyzedFiles, r.PR.ChangedFiles-r.Scope.AnalyzedFiles, strings.ToUpper(r.Status))
	if view != "files" {
		b.WriteString("Review Checklist\n")
		shown := 0
		for _, x := range r.Checklist {
			if x.State == "no_signal" {
				continue
			}
			shown++
			fmt.Fprintf(&b, "[%s] %s", label(x.State), clean(x.Title))
			if x.Unknown {
				b.WriteString("（未解析範囲あり）")
			}
			b.WriteByte('\n')
			for _, t := range x.Targets {
				fmt.Fprintf(&b, "       %s %s\n", clean(t.Path), targetRange(t))
			}
		}
		if shown == 0 {
			b.WriteString("今回の差分から追加提案なし\n")
		}
		b.WriteByte('\n')
	}
	if view != "checklist" {
		b.WriteString("Review Radar\n")
		for _, f := range r.Files {
			fmt.Fprintf(&b, "[%s] %s\n", label(f.Group), clean(f.Path))
			if len(f.Tags) > 0 {
				fmt.Fprintf(&b, "  %s\n", clean(strings.Join(f.Tags, " / ")))
			}
			if f.ContextPartial {
				b.WriteString("  ファイル内の文脈は分割して解析\n")
			}
			for _, reason := range f.Reasons {
				fmt.Fprintf(&b, "  %s\n", clean(reason))
			}
			for _, t := range f.Targets {
				fmt.Fprintf(&b, "  %s %s\n", clean(strings.Join(t.HunkIDs, ",")), targetRange(t))
			}
		}
		b.WriteByte('\n')
	}
	fmt.Fprintf(&b, "経過時間: GitHub %dms / Jev %dms / ローカル %dms / 全体 %dms\nJev照会: %d回", r.Metrics.GitHubMS, r.Metrics.JevMS, r.Metrics.LocalMS, r.Metrics.TotalMS, r.Metrics.HTTPAttempts)
	if r.Metrics.Usage != nil {
		fmt.Fprintf(&b, "  入力tokens: %d", r.Metrics.Usage.InputTokens)
	}
	b.WriteString("\n")
	for _, w := range r.Warnings {
		fmt.Fprintf(&b, "注意: %s\n", clean(w))
	}
	b.WriteString("この結果は差分からの確認候補です。バグ検出・テスト実行・マージ承認ではありません。\n")
	return b.Bytes()
}
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
func markdown(r model.Result, view string) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "# PR #%d %s\n\n%s  \nbase: `%s` / head: `%s`\n\n", r.PR.Number, md(r.PR.Title), md(r.PR.URL), md(r.PR.BaseSHA), md(r.PR.HeadSHA))
	fmt.Fprintf(&b, "分析範囲: %dファイル中%dファイルの差分を解析 / %dファイル未解析  \n処理状態: **%s**\n\n", r.PR.ChangedFiles, r.Scope.AnalyzedFiles, r.PR.ChangedFiles-r.Scope.AnalyzedFiles, strings.ToUpper(r.Status))
	if view != "files" {
		b.WriteString("## Review Checklist\n\n")
		shown := 0
		for _, x := range r.Checklist {
			if x.State == "no_signal" {
				continue
			}
			shown++
			fmt.Fprintf(&b, "- **%s** %s", label(x.State), md(x.Title))
			if x.Unknown {
				b.WriteString("（未解析範囲あり）")
			}
			b.WriteByte('\n')
			for _, t := range x.Targets {
				fmt.Fprintf(&b, "  - %s — %s\n", md(t.Path), md(targetRange(t)))
			}
		}
		if shown == 0 {
			b.WriteString("今回の差分から追加提案なし\n")
		}
		b.WriteByte('\n')
	}
	if view != "checklist" {
		b.WriteString("## Review Radar\n\n")
		for _, f := range r.Files {
			fmt.Fprintf(&b, "- **%s** %s", label(f.Group), md(f.Path))
			if len(f.Tags) > 0 {
				fmt.Fprintf(&b, " — %s", md(strings.Join(f.Tags, " / ")))
			}
			b.WriteByte('\n')
			if f.ContextPartial {
				b.WriteString("  - ファイル内の文脈は分割して解析\n")
			}
			for _, s := range f.Reasons {
				fmt.Fprintf(&b, "  - %s\n", md(s))
			}
			for _, t := range f.Targets {
				fmt.Fprintf(&b, "  - %s %s\n", md(strings.Join(t.HunkIDs, ",")), md(targetRange(t)))
			}
		}
		b.WriteByte('\n')
	}
	fmt.Fprintf(&b, "GitHub %dms / Jev %dms / ローカル %dms / 全体 %dms  \nJev照会 %d回", r.Metrics.GitHubMS, r.Metrics.JevMS, r.Metrics.LocalMS, r.Metrics.TotalMS, r.Metrics.HTTPAttempts)
	if r.Metrics.Usage != nil {
		fmt.Fprintf(&b, " / 入力tokens %d", r.Metrics.Usage.InputTokens)
	}
	b.WriteString("\n\n")
	for _, w := range r.Warnings {
		fmt.Fprintf(&b, "> 注意: %s\n\n", md(w))
	}
	b.WriteString("この結果は差分からの確認候補です。バグ検出・テスト実行・マージ承認ではありません。\n")
	return b.Bytes()
}
