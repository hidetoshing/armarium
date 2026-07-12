# Armarium

Armariumは、自宅や社内などのローカルネットワークで利用する軽量な電子書籍サーバーです。手元の電子書籍をディレクトリへ配置するだけで、OPDS対応リーダーからライブラリを閲覧・ダウンロードできます。

## 主な機能

- ユーザー名とパスワードによるアクセス制限
- 複数の電子書籍ライブラリ
- ディレクトリ階層を使ったフォルダ・シリーズ管理
- PDF、EPUB、CBZ、画像ファイルを収録したZIPに対応
- EPUBのタイトルと著者を自動取得
- ディレクトリの変更を検出するメタデータキャッシュ

## 必要なもの

- Docker Engine
- Docker Compose
- Armariumへ接続するOPDS対応リーダー

## ディレクトリを準備する

任意の作業ディレクトリに、次のファイルとディレクトリを用意します。

```text
armarium/
├── compose.yaml
├── config.json
└── books/
    ├── books/
    │   ├── 小説/
    │   │   └── example.epub
    │   └── example.pdf
    └── comics/
        └── シリーズ名/
            └── volume-01.cbz
```

`books` 以下のディレクトリは、そのままOPDSリーダー上のフォルダとして表示されます。

## Docker Composeを設定する

次の内容を `compose.yaml` として保存します。

```yaml
services:
  armarium:
    image: hidetoshing/armarium:latest
    restart: unless-stopped
    ports:
      - "8080:8080"
    environment:
      ARMARIUM_CONFIG: /config/config.json
    volumes:
      - ./config.json:/config/config.json:ro
      - ./books:/books:ro
      - armarium-cache:/data/cache

volumes:
  armarium-cache:
```

電子書籍と設定ファイルは読み取り専用でコンテナへ渡します。メタデータキャッシュはDockerの名前付きボリュームへ保存されます。

## Armariumを設定する

次の内容を `config.json` として保存し、ユーザー名、パスワード、ライブラリを環境に合わせて変更します。

```json
{
  "listen": ":8080",
  "cache_path": "/data/cache/metadata.json",
  "users": [
    {
      "username": "reader",
      "password_sha256": "e2186dbdb1bb4193608605e84f33208765b5693b55edd4f730a719a100eeea6f"
    }
  ],
  "libraries": [
    {
      "id": "books",
      "name": "書籍",
      "path": "/books/books"
    },
    {
      "id": "comics",
      "name": "コミック",
      "path": "/books/comics"
    }
  ]
}
```

ライブラリの `path` には、コンテナ内の絶対パスを指定します。上記のCompose設定では、ホストの `./books` がコンテナの `/books` に対応します。

上記の `password_sha256` は、初期パスワード `change-me` のSHA-256ハッシュです。運用前に必ず別のパスワードへ変更してください。Linuxでは次のように小文字16進表記のSHA-256ハッシュを生成できます。

```sh
printf '%s' 'ここに安全なパスワードを入力' | sha256sum
```

macOSでは次を使用します。

```sh
printf '%s' 'ここに安全なパスワードを入力' | shasum -a 256
```

生成結果のハッシュ部分だけを `password_sha256` に設定します。平文の `password` も使用できますが、設定ファイルへパスワードを直接保存しない `password_sha256` を標準とします。

## 起動する

`compose.yaml` があるディレクトリで実行します。

```sh
docker compose up --detach
```

起動状態とログは次のコマンドで確認できます。

```sh
docker compose ps
docker compose logs --follow armarium
```

停止する場合は次を実行します。

```sh
docker compose down
```

## OPDSリーダーから接続する

OPDSリーダーへ次の情報を登録します。

| 項目 | 設定値 |
|---|---|
| カタログURL | `http://<Armariumを動かしている端末のIPアドレス>:8080/opds` |
| ユーザー名 | `config.json` の `username` |
| パスワード | `config.json` へ設定したパスワード |

例として、Armariumを動かしている端末のIPアドレスが `192.168.1.20` の場合、カタログURLは `http://192.168.1.20:8080/opds` です。

## 電子書籍を追加する

マウントした `books` ディレクトリへ電子書籍を追加してください。次回のカタログ取得時に反映されるため、通常はコンテナの再起動は不要です。

カタログでディレクトリを開くと、Armariumはそのディレクトリの更新日時を確認します。更新されている場合は直下の書籍だけを照合し、新規または変更された書籍のメタデータを取得して、削除された書籍のキャッシュを除去します。サブディレクトリは、そのサブディレクトリを開いたときに同じ方法で更新されます。

対応する拡張子は次のとおりです。

- `.pdf`
- `.epub`
- `.cbz`
- `.zip`

## イメージを更新する

`compose.yaml` のイメージタグを更新してから、次を実行します。

```sh
docker compose pull
docker compose up --detach
```

## セキュリティ上の注意

ArmariumのBasic認証は通信自体を暗号化しません。信頼できるローカルネットワーク内で使用してください。インターネットからアクセス可能にする場合は、そのまま公開せず、HTTPSを設定したリバースプロキシやVPNを利用してください。

`config.json` には認証情報が含まれるため、第三者が閲覧できないように管理してください。

## 開発者向けドキュメント

- [構成・ビルド・公開仕様](docs/architecture.md)
- [OPDS/API仕様](docs/opds-api.md)
- [コーディングエージェント向け指針](AGENTS.md)

## ライセンス

ArmariumはMIT Licenseのもとで公開しています。詳細は [`LICENSE`](LICENSE) を参照してください。
