package diff

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/kazuki-oshino/prjev/internal/github"
	"github.com/kazuki-oshino/prjev/internal/model"
)

const maxPatch = 12 << 10

var headerRE = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

func ParseHunks(patch string) ([]model.Hunk, int, int, error) {
	lines := strings.SplitAfter(patch, "\n")
	hunks := []model.Hunk{}
	var current *model.Hunk
	adds, dels := 0, 0
	oldSeen, newSeen := 0, 0
	finish := func() error {
		if current == nil {
			return nil
		}
		if oldSeen != current.OldCount || newSeen != current.NewCount {
			return fmt.Errorf("hunkの行数が不一致です")
		}
		hunks = append(hunks, *current)
		return nil
	}
	for _, line := range lines {
		if line == "" {
			continue
		}
		s := strings.TrimSuffix(line, "\n")
		s = strings.TrimSuffix(s, "\r")
		if m := headerRE.FindStringSubmatch(s); m != nil {
			if e := finish(); e != nil {
				return nil, 0, 0, e
			}
			nums := []int{}
			for i := 1; i <= 4; i++ {
				if m[i] == "" {
					nums = append(nums, 1)
				} else {
					n, _ := strconv.Atoi(m[i])
					nums = append(nums, n)
				}
			}
			if !strings.HasPrefix(s, "@@ -") {
				return nil, 0, 0, fmt.Errorf("hunk headerが不正です")
			}
			current = &model.Hunk{ID: fmt.Sprintf("h%03d", len(hunks)+1), OldStart: nums[0], OldCount: nums[1], NewStart: nums[2], NewCount: nums[3], Patch: line}
			oldSeen, newSeen = 0, 0
			continue
		}
		if current == nil {
			return nil, 0, 0, fmt.Errorf("hunk headerがありません")
		}
		current.Patch += line
		switch {
		case strings.HasPrefix(s, "+"):
			adds++
			newSeen++
		case strings.HasPrefix(s, "-"):
			dels++
			oldSeen++
		case strings.HasPrefix(s, " "):
			oldSeen++
			newSeen++
		case strings.HasPrefix(s, `\ No newline at end of file`):
		default:
			return nil, 0, 0, fmt.Errorf("不正なpatch行です")
		}
	}
	if e := finish(); e != nil {
		return nil, 0, 0, e
	}
	if len(hunks) == 0 {
		return nil, 0, 0, fmt.Errorf("hunkがありません")
	}
	return hunks, adds, dels, nil
}
func secretPath(path string) bool {
	for _, part := range strings.Split(strings.ToLower(path), "/") {
		if part == ".env" || strings.HasPrefix(part, ".env.") || strings.HasSuffix(part, ".pem") || strings.HasSuffix(part, ".key") || part == "id_rsa" || part == "id_ed25519" || strings.HasSuffix(part, ".p12") || strings.HasSuffix(part, ".p8") {
			return true
		}
	}
	return false
}
func Normalize(raw []github.APIFile) ([]model.File, []model.Unit) {
	files := make([]model.File, 0, len(raw))
	units := []model.Unit{}
	next := 1
	for i, f := range raw {
		mf := model.File{ID: fmt.Sprintf("f%04d", i+1), Path: f.Filename, PreviousPath: f.PreviousFilename, Status: f.Status, Additions: f.Additions, Deletions: f.Deletions, PatchState: "complete", Attributes: attributes(f.Filename)}
		reason := ""
		switch {
		case secretPath(f.Filename) || secretPath(f.PreviousFilename):
			reason = "秘密ファイルの可能性があるため差分を外部送信しません"
		case f.Status != "added" && f.Status != "modified" && f.Status != "removed" && f.Status != "renamed" && f.Status != "copied" && f.Status != "changed":
			reason = "未知の変更status"
		case f.Patch == nil || *f.Patch == "":
			reason = "patchなし（バイナリ、rename-only、mode変更等）"
		case f.Changes != f.Additions+f.Deletions:
			reason = "変更行数の統計が不一致"
		}
		if reason != "" {
			mf.PatchState = "unknown"
			mf.Reasons = append(mf.Reasons, reason)
			files = append(files, mf)
			continue
		}
		hs, a, d, e := ParseHunks(*f.Patch)
		if e != nil || a != f.Additions || d != f.Deletions {
			mf.PatchState = "unknown"
			mf.Reasons = append(mf.Reasons, "patch構造または変更行数が不一致")
			files = append(files, mf)
			continue
		}
		mf.Hunks = hs
		groups := [][]model.Hunk{}
		current := []model.Hunk{}
		size := 0
		for _, h := range hs {
			if len(h.Patch) > maxPatch {
				mf.Reasons = append(mf.Reasons, h.ID+": 1つのhunkが解析上限超過")
				continue
			}
			if size+len(h.Patch) > maxPatch && len(current) > 0 {
				groups = append(groups, current)
				current = nil
				size = 0
			}
			current = append(current, h)
			size += len(h.Patch)
		}
		if len(current) > 0 {
			groups = append(groups, current)
		}
		if len(groups) == 0 {
			mf.PatchState = "unknown"
		} else if len(mf.Reasons) > 0 {
			mf.PatchState = "partial"
		}
		for _, g := range groups {
			u := model.Unit{ID: fmt.Sprintf("u%04d", next), FileID: mf.ID, Path: mf.Path, FileContextPartial: len(groups) > 1 || len(mf.Reasons) > 0}
			next++
			for _, h := range g {
				u.HunkIDs = append(u.HunkIDs, h.ID)
				u.Patch += h.Patch
			}
			units = append(units, u)
			mf.Units = append(mf.Units, u.ID)
		}
		files = append(files, mf)
	}
	return files, units
}

func attributes(path string) []string {
	p := strings.ToLower(path)
	out := []string{}
	if strings.Contains(p, "test") || strings.HasSuffix(p, "_spec.go") {
		out = append(out, "test")
	}
	if strings.HasPrefix(p, "docs/") || strings.HasSuffix(p, ".md") {
		out = append(out, "docs")
	}
	if strings.Contains(p, "generated") || strings.Contains(p, "mock") || strings.HasSuffix(p, ".pb.go") || strings.HasSuffix(p, "go.sum") {
		out = append(out, "generated-looking")
	}
	return out
}
