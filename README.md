# pradar

GitHubのPR URLから、人が確認するファイルを「必須／注意／不要」に分け、確認する箇所とJevの確信度を表示するGo CLIです。読むべき差分に集中し、確認を省略できそうな差分を見つけるために使います。バグ検出、テスト実行、マージ可否の判定は行いません。

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

`just`がある場合は、PRのURLを渡すだけでCLIをビルドし、日時付きのHTMLを生成してブラウザで開けます。

```bash
just html https://github.com/kazuki-oshino/prjev/pull/1
```

一部未解析でpradarが終了コード`2`を返しても、HTMLが生成されていれば開きます。保存先は`./.pradar/`で、実行時に表示されます。

フラグは位置引数より前に指定します。`replay`は保存した判定値を再集計し、GitHub・Jevへ接続しません。`--output`と`--record`は既存ファイルを上書きしません。記録にはPR本文と差分が含まれるため、保存先の扱いに注意してください。

終了コードは成功が`0`、致命的エラーが`1`、未解析範囲のある結果が`2`、ユーザー中断が`130`です。進捗とエラーはstderr、結果はstdoutへ出します。

## 結果の読み方

最初にファイルの件数と確認の要否、次に具体的な確認観点を表示します。
HTMLとMarkdownでは「不要」の一覧を折りたたみ、必須・注意のファイルから読み始められます。
ターミナルでは不要のファイル名と確信度を簡潔に表示します。

| 判定 | 確認すること |
| --- | --- |
| 必須 | 影響の大きい変更、Jevが重点確認を求めた変更、解析できなかった差分を確認する |
| 注意 | 関連する変更を確認する。判定に迷いがある場合や、文脈が不足している場合も含む |
| 不要 | 詳細確認を省略する候補。無欠陥やマージ可の保証ではない |

たとえば「注意 / Jev: 不要 / 確信度 58.0%」は、Jev自身は不要を選んだものの、判定が曖昧なためアプリが注意にしたことを示します。
確信度（confidence）は選択肢への確率分布からJevが返す値で、コードが正しい確率ではありません。
既存の観点判定（Noul）の数値から確信度を作ることはしません。[TypeSafeの仕様](https://docs.typesafe.ai/confidence)

「不要」は以下をすべて満たす場合に限ります。

- JevのChoice判定が、差分全体を機械的で動作に影響しない変更として `unnecessary` に分類した。
- 返されたconfidenceが **0.85以上**。これは初期の保守的な基準で、実PRで精度を校正した値ではない。
- 標準・追加のすべての確認観点が候補の閾値未満で、判定漏れがない。
- 差分を欠落なく1つの単位で解析でき、PR本文の省略がない。

ドキュメント、テスト、生成ファイルらしい名前という理由だけでは不要にしません。
分割した差分は必須または注意に留め、確信度は平均せず最低値を表示します。詳細には各範囲の判定値を残します。
一覧を取得できないファイルも必須件数に含め、取得できなかった件数を明示します。

保存記録とJSONには、最終判定の `files[].review`、判定基準の `review_policy`、元のChoice・confidence・確率分布を追加しています。
既存の `group` とChecklistの判定は残しています。
古い記録を `replay` した場合、新しいChoiceがないため不要にはせず、確信度は「未取得」と表示します。再照会は行いません。

## 開発

```bash
go test ./...
go vet ./...
# キーがある環境で、非機密の固定入力によるTypeSafe接続を任意で確認
PRADAR_LIVE_PROBE=1 go test ./internal/jev -run '^TestLiveWire$' -v -count=1
```

GitHub、Jevの障害・境界条件は通信をモックしたテストで検査できます。実際のPRを使う評価と閾値の校正は、利用環境で別途行ってください。設計と対象外の範囲は[設計書](docs/pradar-design.md)に記載しています。
