# dogapp-api

`dogapp_flutter`(犬の健康管理アプリ)が話しかける先の実際のバックエンド。
これまでFlutter側のコードに「想定実装」として書かれていたエンドポイント群を、
実際に動くGoサーバーとして実装したもの。

## 構成

```
dogapp-api/
  main.go                    エントリーポイント(サーバー起動、DB/Claudeクライアントの配線)
  docker-compose.yml         ローカル開発用Postgres
  internal/
    model/model.go           Dog, HealthRecord, WalkRoute等(dogapp_flutterのDartモデルとJSON形状を一致させている)
    store/
      store.go                Postgresでの永続化(dogs/records/walks)
      seed.go                 初回起動時にレオ・ノアを投入する(mock_data.dartと同じ内容)
      storetest/               テスト用に使い捨てDBを作成・破棄するヘルパー
    claude/client.go          Claude API(画像/複数フレーム画像)を呼んでAICheckResultを返す
    video/extract.go          ffmpegで動画から静止フレームを抽出(Claudeは動画を直接は扱えないため)
    handlers/                 net/httpのハンドラ群
.github/workflows/ci.yml     push/PR時にPostgresサービスコンテナ相手にビルド・vet・テスト
```

## セットアップ

```bash
docker compose up -d postgres   # ローカル開発用Postgres(localhost:5432)を起動
go build ./...
go test ./...                   # store/handlers のテストは上のPostgresに対して実行される
```

Go 1.22以降のみ依存(標準ライブラリの`http.ServeMux`のメソッド+パスパターン
マッチングを使っており、ルーティング用の外部パッケージは使っていない)。
Postgresドライバは`github.com/lib/pq`。

`go test`実行時にPostgresへ接続できない場合、`store`/`handlers`パッケージの
テストは(失敗ではなく)スキップされる。CIでは常に`postgres`サービスコンテナが
立っているため、そちらでは実際に実行される。

## 実行

```bash
docker compose up -d postgres
export ANTHROPIC_API_KEY=sk-ant-...   # /ai-check, /gait-check に必要
go run .
```

デフォルトでは`:8080`で待ち受け、`postgres://postgres:password@127.0.0.1:5432/dogapp?sslmode=disable`
(`docker-compose.yml`の設定と一致)に接続する(`DOGAPP_ADDR`/
`DOGAPP_POSTGRES_DSN`環境変数で変更可能。`DOGAPP_POSTGRES_DSN`が未設定の場合、
Renderなどマネージド環境が自動注入する`DATABASE_URL`にもフォールバックする)。
初回起動時、DBが空なら`dogapp_flutter`のモックと同じレオ・ノアのデータを
自動で投入する。

`ANTHROPIC_API_KEY`が無くても他のエンドポイント(犬一覧・記録・散歩)は
問題なく動く。`/ai-check`・`/gait-check`だけ、呼び出し時にキー未設定である旨の
明確なエラー(502)を返す。

`/gait-check`(動画による歩行チェック)は`ffmpeg`が`PATH`にある必要がある
(`brew install ffmpeg`)。無い場合は501でその旨のエラーを返す。

## エンドポイント

| メソッド | パス | 用途 |
|---|---|---|
| GET | `/health` | 死活監視用(DB/Claudeには触れない) |
| GET | `/owners/{ownerId}/dogs` | 犬一覧の取得(現状ownerIdは無視し、全犬を返す。認証・複数オーナー対応は未実装) |
| PATCH | `/dogs/{dogId}` | `{"name","breed","color","birthYear"}` → プロフィールを更新 |
| POST | `/dogs/{dogId}/ai-check` | `{"imageBase64": "..."}` → 写真をClaudeの画像入力に渡し皮膚・被毛を判定 |
| POST | `/dogs/{dogId}/gait-check` | `multipart/form-data`(フィールド名`video`)→ ffmpegで抽出した複数フレームをClaudeに渡し歩様を判定 |
| POST | `/dogs/{dogId}/records` | `{"type": "...", "label": "..."}` → 記録を追加 |
| GET | `/dogs/{dogId}/walks` | 散歩記録の一覧(新しい順) |
| POST | `/dogs/{dogId}/walks` | `{"startedAt","durationSeconds","distanceMeters","points":[{"lat","lng","timestamp"}]}` → 散歩記録を保存 |

レスポンスのJSON形状は`dogapp_flutter`の`lib/models/dog.dart` /
`lib/models/walk.dart`の`fromJson`/`toJson`とフィールド名を完全に一致させている。

## Claude連携について

`internal/claude/client.go`が皮膚チェック・歩行チェックそれぞれ専用の
プロンプトを組み立て、Claudeに「診断ではなくJSON1つだけを返す」よう指示し、
レスポンステキストから`{...}`部分を抽出してパースする(コードブロックや
前置きの文章が混ざっても壊れないようにするため)。`internal/claude.Checker`
はインターフェースになっており、`internal/handlers`のテストでは実際のAPIを
叩かないフェイク実装に差し替えている。

歩行チェックはClaudeが動画を直接受け付けないため、`internal/video`で
ffmpegを使い動画から数枚(既定5枚)のJPEGフレームを抽出し、それらを
まとめて1回のリクエストで送っている。

## 堅牢性まわり

- `http.Server`に`ReadHeaderTimeout`等を設定し、`SIGINT`/`SIGTERM`で
  `Shutdown(ctx)`によるgraceful shutdownを行う(接続中のリクエストを
  中断せず、DBハンドルもきちんと閉じる)
- リクエストボディサイズを`ai-check`(15MB)・`gait-check`(50MB)双方で
  `http.MaxBytesReader`により上限を設けている
- Claude呼び出しには60秒のタイムアウトを設定(上流が詰まってハンドラが
  無期限にハングしないように)

## 既知の簡略化

これは`dogapp_flutter`単体のためのバックエンドで、本番のマルチユーザー
サービスとしては以下が未実装。

- 認証・認可(`ownerId`は受け取るが検証しない)
- レート制限・Claude呼び出しのコスト管理
- マイグレーション管理(スキーマは起動時に`CREATE TABLE IF NOT EXISTS`するだけ)
