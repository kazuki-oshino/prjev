# pradar

GitHubのPR URLから、レビュー時に確認したい観点（Checklist）と読み始めるファイル（Radar）を表示するGo CLIです。提示された差分をTypeSafe Jevで分類します。バグ検出、テスト実行、マージ可否の判定は行いません。

## 準備

- Go 1.26以降
- `gh`をインストールし、対象PRを読める状態で認証する
- `TYPESAFE_API_KEY`を環境変数かカレントディレクトリの`.env`に設定する（`.env.example`を参照）

```bash
go build -o pradar ./cmd/pradar
./pradar https://github.com/owner/repo/pull/123
```

最初のJev送信前に、送信先と送る内容の概要をstderrに表示します。社内PRで使う場合は、外部APIへのデータ送信ルールを確認してください。`.env`や秘密鍵に見えるパスのpatchは送信しませんが、一般的な秘密情報を完全に検出するものではありません。

## コマンド

```bash
pradar checklist https://github.com/owner/repo/pull/123
pradar files https://github.com/owner/repo/pull/123
pradar --format html --output ./.pradar/report.html https://github.com/owner/repo/pull/123
pradar --format markdown --output ./.pradar/report.md https://github.com/owner/repo/pull/123
pradar --format json https://github.com/owner/repo/pull/123
pradar --config ./examples/pradar.yaml --env-file ./local.env https://github.com/owner/repo/pull/123
pradar --record ./.pradar/run.json https://github.com/owner/repo/pull/123
pradar replay --format markdown ./.pradar/run.json
```

HTMLはCSSを含む単一ファイルです。上記のコマンドで保存した`./.pradar/report.html`をブラウザで開けます。`checklist`、`files`、`replay`でも`--format html`を利用できます。

フラグは位置引数より前に指定します。`replay`は保存した判定値を再集計し、GitHub・Jevへ接続しません。`--output`と`--record`は既存ファイルを上書きしません。記録にはPR本文と差分が含まれるため、保存先の扱いに注意してください。

終了コードは成功が`0`、致命的エラーが`1`、未解析範囲のある結果が`2`、ユーザー中断が`130`です。進捗とエラーはstderr、結果はstdoutへ出します。

## 開発

```bash
go test ./...
go vet ./...
# キーがある環境で、非機密の固定入力によるTypeSafe接続を任意で確認
PRADAR_LIVE_PROBE=1 go test ./internal/jev -run '^TestLiveWire$' -v -count=1
```

GitHub、Jevの障害・境界条件は通信をモックしたテストで検査できます。実際のPRを使う評価と閾値の校正は、利用環境で別途行ってください。設計と対象外の範囲は[設計書](docs/pradar-design.md)に記載しています。
