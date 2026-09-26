package github

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/kazuki-oshino/prjev/internal/model"
)

const maxOutput = 8 << 20

var ownerRE = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?$`)
var repoRE = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

type Ref struct {
	URL, Owner, Repo string
	Number           int
}

func ParseRef(raw string) (Ref, error) {
	u, e := url.Parse(raw)
	if e != nil {
		return Ref{}, e
	}
	p := strings.Split(strings.Trim(u.Path, "/"), "/")
	if u.Scheme != "https" || u.Host != "github.com" || u.User != nil || len(p) != 4 || p[2] != "pull" || !ownerRE.MatchString(p[0]) || !repoRE.MatchString(p[1]) || p[1] == "." || p[1] == ".." {
		return Ref{}, fmt.Errorf("GitHub.comのPR URLを指定してください")
	}
	n, e := strconv.Atoi(p[3])
	if e != nil || n <= 0 {
		return Ref{}, fmt.Errorf("PR番号が不正です")
	}
	return Ref{URL: fmt.Sprintf("https://github.com/%s/%s/pull/%d", p[0], p[1], n), Owner: p[0], Repo: p[1], Number: n}, nil
}

type Meta struct {
	Number       int    `json:"number"`
	URL          string `json:"url"`
	Title        string `json:"title"`
	Body         string `json:"body"`
	BaseRefOid   string `json:"baseRefOid"`
	HeadRefOid   string `json:"headRefOid"`
	ChangedFiles int    `json:"changedFiles"`
	Additions    int    `json:"additions"`
	Deletions    int    `json:"deletions"`
}
type APIFile struct {
	Filename         string  `json:"filename"`
	PreviousFilename string  `json:"previous_filename"`
	Status           string  `json:"status"`
	Additions        int     `json:"additions"`
	Deletions        int     `json:"deletions"`
	Changes          int     `json:"changes"`
	Patch            *string `json:"patch"`
}
type Reader struct {
	Run func(context.Context, ...string) ([]byte, error)
}

func DefaultReader() Reader { return Reader{Run: runGH} }
func runGH(ctx context.Context, args ...string) ([]byte, error) {
	c := exec.CommandContext(ctx, "gh", args...)
	c.Env = filteredEnv()
	c.Stdin = nil
	out := &limitedWriter{limit: maxOutput}
	errOut := &limitedWriter{limit: 4096}
	c.Stdout = out
	c.Stderr = errOut
	e := c.Run()
	if errors.Is(out.err, errLimit) {
		return nil, fmt.Errorf("gh出力が8 MiBを超えました")
	}
	if e != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("gh取得に失敗しました。認証とPRの参照権限を確認してください")
	}
	return out.Bytes(), nil
}
func filteredEnv() []string {
	env := []string{}
	for _, v := range os.Environ() {
		k, _, _ := strings.Cut(v, "=")
		if k != "TYPESAFE_API_KEY" {
			env = append(env, v)
		}
	}
	return append(env, "GH_PROMPT_DISABLED=1", "GIT_TERMINAL_PROMPT=0")
}

var errLimit = errors.New("output limit")

type limitedWriter struct {
	bytes.Buffer
	limit int
	err   error
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if w.Len()+len(p) > w.limit {
		w.err = errLimit
		return 0, errLimit
	}
	return w.Buffer.Write(p)
}
func (r Reader) metadata(ctx context.Context, ref Ref) (Meta, error) {
	b, e := r.Run(ctx, "pr", "view", ref.URL, "--json", "number,url,title,body,baseRefName,baseRefOid,headRefName,headRefOid,changedFiles,additions,deletions,isDraft,state")
	if e != nil {
		return Meta{}, e
	}
	var m Meta
	if e = json.Unmarshal(b, &m); e != nil {
		return m, fmt.Errorf("PRメタデータが不正です: %w", e)
	}
	if m.Number != ref.Number || m.BaseRefOid == "" || m.HeadRefOid == "" {
		return m, fmt.Errorf("PRメタデータの必須項目がありません")
	}
	return m, nil
}
func Same(a, b Meta) bool {
	return a.Number == b.Number && a.Title == b.Title && a.Body == b.Body && a.BaseRefOid == b.BaseRefOid && a.HeadRefOid == b.HeadRefOid && a.ChangedFiles == b.ChangedFiles && a.Additions == b.Additions && a.Deletions == b.Deletions
}
func (r Reader) Read(ctx context.Context, ref Ref) (model.PR, []APIFile, error) {
	a, e := r.metadata(ctx, ref)
	if e != nil {
		return model.PR{}, nil, e
	}
	pr := model.PR{URL: ref.URL, Repository: ref.Owner + "/" + ref.Repo, Number: ref.Number, Title: a.Title, Body: a.Body, BaseSHA: a.BaseRefOid, HeadSHA: a.HeadRefOid, ChangedFiles: a.ChangedFiles}
	metaBytes, _ := json.Marshal(a)
	metaHash := sha256.Sum256(metaBytes)
	pr.Hash = hex.EncodeToString(metaHash[:])
	if a.ChangedFiles > 100 {
		pr.FetchedAt = time.Now().UTC()
		return pr, nil, nil
	}
	endpoint := fmt.Sprintf("repos/%s/%s/pulls/%d/files?per_page=100", ref.Owner, ref.Repo, ref.Number)
	b, e := r.Run(ctx, "api", "--hostname", "github.com", "--method", "GET", "--paginate", "--slurp", endpoint)
	if e != nil {
		return pr, nil, e
	}
	var pages [][]APIFile
	if e = json.Unmarshal(b, &pages); e != nil {
		return pr, nil, fmt.Errorf("変更ファイル一覧が不正です: %w", e)
	}
	files := []APIFile{}
	seen := map[string]bool{}
	for _, page := range pages {
		for _, f := range page {
			if f.Filename == "" || seen[f.Filename] {
				return pr, nil, fmt.Errorf("変更ファイルのパスが不正または重複しています")
			}
			seen[f.Filename] = true
			files = append(files, f)
		}
	}
	z, e := r.metadata(ctx, ref)
	if e != nil {
		return pr, nil, e
	}
	if !Same(a, z) {
		return pr, nil, fmt.Errorf("pr_changed_during_fetch: PRが取得中に更新されました。再実行してください")
	}
	if len(files) != a.ChangedFiles {
		return pr, nil, fmt.Errorf("変更ファイル数がメタデータと一致しません")
	}
	pr.FetchedAt = time.Now().UTC()
	payload, _ := json.Marshal(struct {
		M Meta
		F []APIFile
	}{a, files})
	sum := sha256.Sum256(payload)
	pr.Hash = hex.EncodeToString(sum[:])
	return pr, files, nil
}

var _ io.Writer = (*limitedWriter)(nil)
