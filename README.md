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
| 必須 | 重大な影響や重要な検証の弱体化、高い確信度で重点確認を求めた変更、未解析の差分を確認する |
| 注意 | 関連する変更を確認する。判定に迷いがある場合や、文脈が不足している場合も含む |
| 不要 | 詳細確認を省略する候補。無欠陥やマージ可の保証ではない |

HTMLでは、各ファイルを「確認区分」「区分を決めた理由」「AIの見立て（参考）」に分けて表示します。
たとえば確認区分が「必須」でAIの見立てが「確認を勧める・確信度28%」の場合は、別に判定した重大な影響や検証の弱体化を優先したことと、その根拠を表示します。
この28%は参考の見立てに対する数値で、最終的な確認区分の確信度ではありません。
確認区分が「注意」、AIの見立てが「詳細確認を省略できそう」の場合にも、確信度不足など、確認を勧める理由を併記します。
ページ内の「確認区分・AIの見立て・確信度の読み方」から、それぞれの意味を確認できます。
確信度（confidence）は選択肢への確率分布からJevが返す値で、コードが正しい確率ではありません。
既存の観点判定（Noul）の数値から確信度を作ることはしません。[TypeSafeの仕様](https://docs.typesafe.ai/confidence)

チェックリストの8観点は「何に関係する変更か」を示し、該当するだけで必須にはしません。
これとは別に「実際の重大な影響」「重要なテストの検証が失われる可能性」をNoulで質問します。
各質問にはテストの場合の該当・除外条件を記述し、mockの変更と実サービスへの操作、通常のテスト追加とassertionの削除を区別します。
重大な影響・検証の弱体化の判定値が0.75以上なら必須、0.35以上なら少なくとも注意です。
Choice単独で必須にする場合もconfidence 0.85以上を要求し、それ未満なら注意に留めます。
これらは未校正の初期値であり、Noulとconfidenceは異なる意味の数値です。

「不要」は以下をすべて満たす場合に限ります。

- JevのChoice判定が、差分全体を影響の限られた変更として `unnecessary` に分類した。
- 返されたconfidenceが **0.85以上**。これは初期の保守的な基準で、実PRで精度を校正した値ではない。
- 重大な影響・検証の弱体化の2判定がともに0.35未満で、チェックリストも含め判定漏れがない。
- 差分を欠落なく1つの単位で解析でき、PR本文の省略がない。

機械的な変更に加え、影響の限られたfixture更新や、期待値が明確で既存の検証を弱めない単純なテスト追加も省略候補になります。
ドキュメント、テスト、生成ファイルらしい名前という理由だけでは不要にしません。
分割した差分は必須または注意に留め、確信度は平均せず最低値を表示します。詳細には各範囲の判定値を残します。
一覧を取得できないファイルも必須件数に含め、取得できなかった件数を明示します。

保存記録とJSONには、最終判定の `files[].review`、判定基準の `review_policy`、元のChoice・confidence・確率分布を追加しています。
`review.basis` と `review.checks` は、確認区分を選んだ理由と根拠の項目名を表示するための情報です。項目名は実行時のチェック定義を使います。
`review.risks` には重大な影響・検証の弱体化の生の判定値と対象範囲を保存します。
既存の `group` とChecklistは残しています。追加チェックの `priority` もこの旧groupに使い、最終的な必須・注意・不要は独立して決めます。
新しい記録は `evidence.review_policy_version: 2` を保存します。`replay` は記録時の方針を使い、version省略・1の旧記録を新方針で読み替えません。Choiceがない記録では確信度を「未取得」と表示し、不要にはしません。再照会は行いません。

## 開発

```bash
go test ./...
go vet ./...
# キーがある環境で、非機密の固定入力によるTypeSafe接続を任意で確認
PRADAR_LIVE_PROBE=1 go test ./internal/jev -run '^TestLiveWire$' -v -count=1
# 非機密の境界事例を実モデルで評価（外部API利用・トークン消費あり）
PRADAR_REVIEW_EVAL=1 PRADAR_REVIEW_EVAL_MODEL=jev-1.13.0 go test ./internal/app -run '^TestLiveReviewEvaluation$' -v -count=1
```

標準構成は1解析単位につき8観点＋2影響判定＋1Choiceの11質問です。追加質問も既存の質問数・バイト数・照会回数の上限に含めます。
境界事例の期待値は `internal/app/testdata/review_cases.json` にあり、通常のテストでは外部APIを呼びません。実評価では期待値をJevへ送りません。
GitHub、Jevの障害・境界条件は通信をモックしたテストで検査できます。実際のPRを使う評価と閾値の校正は、利用環境で別途行ってください。設計と対象外の範囲は[設計書](docs/pradar-design.md)に記載しています。
