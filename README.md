# OpenTelemetryで障害を調べる

注文APIから在庫APIへのリクエストを、分散トレースで追う練習です。Goのインストールは不要で、DockerとDocker Composeを使います。

```text
curl → order-api → inventory-api
             両API → Collector → Jaeger
```

Collectorは観測データを受け取り、まとめてJaegerへ転送します。Jaegerはトレースを表示します。この構成はローカル学習用で、データは永続化されません。

## 起動と観察

```bash
cd /home/user/git_wsl/opentelemetry-practice
docker compose up --build -d
curl -i 'http://localhost:8080/orders'
curl -i 'http://localhost:8080/orders?mode=slow'
curl -i 'http://localhost:8080/orders?mode=error'
```

それぞれ `200 order accepted`、約2秒後に `200`、`502 inventory unavailable` が返ります。ブラウザで http://localhost:16686 を開き、Serviceに `order-api` を選んで **Find Traces** を押します。送信はバッチ処理されるため、表示まで数秒かかる場合があります。表示されない場合は少し待って検索し直してください。

トレースを開いて、横棒の長さと入れ子を比較してください。

- 正常：order-apiとinventory-apiのスパンが一つのトレースにつながっているか。
- 遅延：`lookup stock` の長さはどう変わったか。注文API全体の時間とどう関係するか。
- エラー：どのスパンにエラーが付いているか。HTTPの502と在庫API内部の失敗の関係を説明できるか。

`practice.scenario` 属性で実験条件を確認できます。実システムでは、障害を調べるのに必要な属性を選び、個人情報や際限なく種類が増える値の記録には注意します。

## コードを追う

[cmd/server/main.go](cmd/server/main.go) の `otelhttp.NewHandler` が受信側のHTTPスパンを、`otelhttp.NewTransport` が送信側のHTTPスパンを作ります。`TraceContext` がサービス間でトレースの親子関係を伝えます。`Start` で作った `check inventory` と `lookup stock` は業務処理を表す手動スパンです。

次に、在庫APIの遅延を別の処理へ移してみてください。画面だけからどこまで原因を絞れるか、追加のスパンが必要か考えると、計装の設計を練習できます。メトリクスによる異常検知や、ログとトレースの関連付けは次のステップです。

## 停止・確認

```bash
docker compose logs collector
docker compose down
```

ポート8080または16686が使用中の場合は、compose.yamlの左側のポート番号を変更してください。起動順序の指定は準備完了を保証しないため、起動直後に失敗したら少し待って再試行します。

## Dev Containerで開く

VS CodeにDev Containers拡張機能とDockerを用意し、このディレクトリを開いて、コマンドパレットから「Dev Containers: Reopen in Container」を選びます。初回はイメージのビルドとGoモジュールのダウンロードに時間がかかります。開発用コンテナにはGo 1.26、AWS CLI、curl、Git、GitHub CLI、Task、Codex CLI、Claude Codeが入り、GoのモジュールキャッシュとCodex・Claudeの設定はDockerのボリュームに保存されます。各CLIは初回利用時にそれぞれ認証してください。

開くと注文API、在庫API、Collector、Jaegerも起動します。コンテナ内のターミナルからはサービス名を使います。

```bash
curl -i 'http://order-api:8080/orders?mode=slow'
go test ./...
```

Jaegerはホスト側のブラウザで http://localhost:16686 を開いてください。APIのコードを編集した場合、実行中のAPIコンテナには自動反映されません。ホスト側のターミナルで `docker compose up --build -d` を実行すると再ビルドできます。コンテナを停止するにはVS CodeからDev Containerを閉じ、ホスト側で `docker compose -f compose.yaml -f .devcontainer/compose.yaml down` を実行します。

AWSとGitHub CLIは、ホストの `~/.aws` と `~/.config/gh` を読み取り専用で参照します。AWSのプロファイルは `AWS_PROFILE`、リージョンは `AWS_REGION` で指定でき、未指定時はそれぞれ `default` と `ap-northeast-2` です。AWS SSOなどの認証更新はホスト側で行ってください。読み取り専用なのは認証ファイルのマウントであり、AWSやGitHubに対する操作権限は認証情報に依存します。
# opentelemetry_practice
