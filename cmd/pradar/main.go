package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/kazuki-oshino/prjev/internal/app"
	"github.com/kazuki-oshino/prjev/internal/config"
	"github.com/kazuki-oshino/prjev/internal/github"
	"github.com/kazuki-oshino/prjev/internal/jev"
	"github.com/kazuki-oshino/prjev/internal/model"
	"github.com/kazuki-oshino/prjev/internal/report"
	"github.com/kazuki-oshino/prjev/internal/store"
)

type options struct {
	format, output, envFile, config, record, model string
	timeout                                        time.Duration
	noColor, version                               bool
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }
func run(args []string, stdout, stderr io.Writer) int {
	opts, view, pos, err := parse(args)
	if err != nil {
		fmt.Fprintln(stderr, "エラー:", err)
		return 1
	}
	if opts.version {
		fmt.Fprintln(stdout, model.Version)
		return 0
	}
	if view == "replay" {
		if len(pos) != 1 {
			return usage(stderr)
		}
		rec, e := store.Read(pos[0])
		if e != nil {
			fmt.Fprintln(stderr, "エラー:", e)
			return 1
		}
		r := app.Aggregate(rec.Evidence, rec.Result.Model.Requested)
		r.Metrics = rec.Result.Metrics
		r.Warnings = append(r.Warnings, "保存済み判定の再集計です。外部通信と新しいモデル判断は行っていません")
		return output(r, opts, view, stdout, stderr)
	}
	if len(pos) != 1 {
		return usage(stderr)
	}
	ref, e := github.ParseRef(pos[0])
	if e != nil {
		fmt.Fprintln(stderr, "エラー:", e)
		return 1
	}
	c, e := config.Load(opts.config, opts.envFile, opts.envFile != "")
	if e != nil {
		fmt.Fprintln(stderr, "エラー:", e)
		return 1
	}
	if opts.model != "" {
		c.Model = opts.model
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, opts.timeout)
	defer cancel()
	client := jev.New(c.Key, c.RequestTimeout)
	evidence, e := app.Scan(ctx, ref, c, github.DefaultReader(), client, func(s string) { fmt.Fprintln(stderr, s) })
	if e != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return 130
		}
		fmt.Fprintln(stderr, "エラー:", e)
		return 1
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return 130
	}
	r := app.Aggregate(evidence, c.Model)
	if opts.record != "" {
		if e = store.Save(opts.record, store.Record{RecordSchemaVersion: 1, Evidence: evidence, Result: r}); e != nil {
			fmt.Fprintln(stderr, "記録エラー:", e)
			return 1
		}
	}
	return output(r, opts, view, stdout, stderr)
}
func output(r model.Result, o options, view string, stdout, stderr io.Writer) int {
	if view == "replay" {
		view = "scan"
	}
	b, e := report.Render(r, o.format, view)
	if e != nil {
		fmt.Fprintln(stderr, "エラー:", e)
		return 1
	}
	b = append(b, '\n')
	if o.output != "" {
		if e = store.SaveBytes(o.output, b); e != nil {
			fmt.Fprintln(stderr, "出力エラー:", e)
			return 1
		}
	} else {
		if _, e = stdout.Write(b); e != nil {
			fmt.Fprintln(stderr, "出力エラー:", e)
			return 1
		}
	}
	if r.Status == "partial" {
		return 2
	}
	return 0
}
func usage(w io.Writer) int {
	fmt.Fprintln(w, "使い方: pradar [flags] [scan|checklist|files] PR_URL | pradar replay [flags] RECORD_JSON")
	return 1
}
func parse(args []string) (options, string, []string, error) {
	o := options{format: "terminal", timeout: 30 * time.Second}
	view := "scan"
	if len(args) > 0 && isCommand(args[0]) {
		view = args[0]
		args = args[1:]
	}
	fs := flag.NewFlagSet("pradar", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&o.format, "format", o.format, "")
	fs.StringVar(&o.output, "output", "", "")
	fs.StringVar(&o.envFile, "env-file", "", "")
	fs.StringVar(&o.config, "config", "", "")
	fs.StringVar(&o.record, "record", "", "")
	fs.DurationVar(&o.timeout, "timeout", o.timeout, "")
	fs.StringVar(&o.model, "model", "", "")
	fs.BoolVar(&o.noColor, "no-color", false, "")
	fs.BoolVar(&o.version, "version", false, "")
	if e := fs.Parse(args); e != nil {
		return o, view, nil, e
	}
	pos := fs.Args()
	if len(pos) > 0 && isCommand(pos[0]) && view == "scan" {
		view = pos[0]
		pos = pos[1:]
	}
	if o.format != "terminal" && o.format != "markdown" && o.format != "json" && o.format != "html" {
		return o, view, nil, fmt.Errorf("--formatが不正です")
	}
	if o.timeout <= 0 {
		return o, view, nil, fmt.Errorf("--timeoutが不正です")
	}
	if view == "replay" && (o.envFile != "" || o.config != "" || o.record != "" || o.model != "" || o.timeout != 30*time.Second) {
		return o, view, nil, fmt.Errorf("replayでは出力関連フラグのみ利用できます")
	}
	if len(pos) > 1 || len(pos) == 1 && strings.HasPrefix(pos[0], "-") {
		return o, view, nil, fmt.Errorf("引数が不正です")
	}
	return o, view, pos, nil
}
func isCommand(s string) bool {
	return s == "scan" || s == "checklist" || s == "files" || s == "replay"
}
