set positional-arguments

# PRのHTMLレポートを生成してブラウザで開く
html url:
    #!/usr/bin/env bash
    set -eu
    report="./.pradar/report-$(date +%Y%m%d-%H%M%S)-$$.html"
    go build -o ./pradar ./cmd/pradar
    set +e
    ./pradar --format html --output "$report" "$1"
    status=$?
    set -e
    if [ "$status" -ne 0 ] && [ "$status" -ne 2 ]; then
        exit "$status"
    fi
    if [ ! -f "$report" ]; then
        printf 'HTMLレポートが生成されませんでした: %s\n' "$report" >&2
        exit 1
    fi
    printf 'HTML: %s\n' "$report"
    if [ "$status" -eq 2 ]; then
        printf '一部未解析の結果です。詳細はHTMLを確認してください。\n' >&2
    fi
    open "$report"
