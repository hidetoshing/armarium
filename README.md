# Armarium

ローカルイントラネット向けの軽量な OPDS 電子書籍サーバーです。Go 標準ライブラリのみで動作し、単一の Docker コンテナとしてホストできます。

MIT License のもとで公開しています。詳細は [`LICENSE`](LICENSE) を参照してください。

## 機能

- HTTP Basic 認証（平文設定または SHA-256 ダイジェスト）
- OPDS 1.x / Atom ナビゲーションフィード
- 複数ライブラリとディレクトリ階層によるフォルダ・シリーズ管理
- PDF、EPUB、CBZ、画像 ZIP の配信
- EPUB のタイトル・著者、ZIP/CBZ の画像数、全形式のサイズ・更新日時を抽出
- サイズと更新日時をキーにした永続メタデータキャッシュ

## 起動

1. `config.example.json` を `config.json` にコピーし、認証情報とライブラリを編集します。
2. `books/books`、`books/comics` など、設定に対応するディレクトリへ電子書籍を配置します。
3. `make up` を実行します。
4. OPDS クライアントに `http://サーバーのIP:8080/opds` と設定した認証情報を登録します。

設定の `path` はコンテナ内の絶対パスで指定します。compose の既定構成ではホスト側の `./books` がコンテナの `/books` に読み取り専用でマウントされます。

パスワードを設定ファイルへ直接置かない場合は、`password` の代わりに小文字16進表記の `password_sha256` を指定できます。Basic 認証は通信自体を暗号化しないため、信頼できないネットワークで公開する場合は HTTPS リバースプロキシを配置してください。

## エンドポイント

- `GET /` — 案内ページ
- `GET /opds` — ライブラリ一覧
- `GET /opds/{library}?path={folder}` — フォルダまたは書籍一覧
- `GET /download/{library}/{path...}` — 電子書籍の取得

環境変数 `ARMARIUM_CONFIG` で設定ファイルを変更できます。未指定時は `config.json` を読み込みます。

## ビルドとDocker Hubへの公開

`Makefile` の既定値はイメージ名 `armarium`、タグ `latest` です。Docker Hubユーザー名には公開者のアカウントを明示的に指定します。

```sh
# ローカルイメージ armarium:latest をビルド
make build

# Composeでビルドしてバックグラウンド起動
make up

# example/armarium:latest を付与
make tag DOCKERHUB_USER=example

# Docker Hubへログイン
make login DOCKERHUB_USER=example

# ビルド、タグ付与、Docker Hubへのpush
make push DOCKERHUB_USER=example
```

バージョンタグや公開先はMake変数で上書きできます。

```sh
make push DOCKERHUB_USER=example IMAGE_NAME=armarium TAG=v1.0.0
```

`DOCKERHUB_USER` が未指定の場合、`tag`、`login`、`push` はエラーで終了します。`make push` の前に、対象のDocker Hubアカウントへログインし、リポジトリを作成しておく必要があります。認証情報はリポジトリへ保存しません。

## ドキュメント

- [構成仕様書](docs/architecture.md)
- [OPDS/API仕様](docs/opds-api.md)
