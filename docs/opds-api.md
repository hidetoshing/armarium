# Armarium OPDS/API仕様

## 1. 概要

ArmariumはOPDS 1.x互換のAtom XMLフィードと、電子書籍ファイルのHTTPダウンロードを提供します。すべてのパスは同一オリジンを前提とする相対URLでフィードへ出力されます。

## 2. 共通仕様

### 2.1 ベースURL

Docker Composeの既定値は次のとおりです。

```text
http://localhost:8080
```

### 2.2 認証

全エンドポイントでHTTP Basic認証が必要です。

```sh
curl --user reader:change-me http://localhost:8080/opds
```

認証情報がない、または一致しない場合は次を返します。

```http
HTTP/1.1 401 Unauthorized
WWW-Authenticate: Basic realm="armarium", charset="UTF-8"
Content-Type: text/plain; charset=utf-8

認証が必要です
```

### 2.3 OPDSレスポンス

OPDSエンドポイントのContent-Typeは次のとおりです。

```http
Content-Type: application/atom+xml;profile=opds-catalog;kind=navigation; charset=utf-8
```

Atom名前空間は `http://www.w3.org/2005/Atom`、日時はUTCのRFC 3339形式です。フィードとエントリーの `id` はArmarium固有のURNであり、外部URLではありません。

### 2.4 文字コードとURLエンコード

- XMLとテキストレスポンスはUTF-8です。
- ライブラリIDとダウンロードパスの各セグメントはパスエスケープされます。
- `path` クエリはクエリエスケープされます。
- クライアントはフィード内の相対URLを、取得元URLを基準に解決する必要があります。

## 3. エンドポイント一覧

| メソッド | パス | 説明 | 成功時Content-Type |
|---|---|---|---|
| `GET` | `/` | Webブラウザー向け案内ページ | `text/html; charset=utf-8` |
| `GET` | `/opds` | ライブラリ一覧 | OPDS Atom |
| `GET` | `/opds/{library}` | ライブラリまたはフォルダの内容 | OPDS Atom |
| `GET` | `/download/{library}/{path...}` | 電子書籍ファイル取得 | 書籍形式ごとのMIMEタイプ |

Goのメソッド付きルーティングを使用しているため、同じパスへの未対応HTTPメソッドは `405 Method Not Allowed` になります。

## 4. `GET /`

ブラウザー向けの最小案内ページを返します。OPDSカタログ `/opds` へのリンクを含みます。このパスにもBasic認証が必要です。

```sh
curl --user reader:change-me http://localhost:8080/
```

## 5. `GET /opds`

設定済みライブラリをナビゲーションエントリーとして返します。

### 5.1 リクエスト例

```sh
curl --user reader:change-me http://localhost:8080/opds
```

### 5.2 レスポンス例

設定が次の場合を例にします。

```json
{
  "libraries": [
    { "id": "books", "name": "書籍", "path": "/books/books" },
    { "id": "comics", "name": "コミック", "path": "/books/comics" }
  ]
}
```

```xml
<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom"
      xmlns:dc="http://purl.org/dc/elements/1.1/"
      xmlns:dcterms="http://purl.org/dc/terms/">
  <id>urn:armarium:root</id>
  <title>Armarium</title>
  <updated>2026-07-12T01:23:45Z</updated>
  <entry>
    <id>urn:armarium:library:books</id>
    <title>書籍</title>
    <updated>2026-07-12T01:23:45Z</updated>
    <link rel="subsection" href="/opds/books" type="application/atom+xml;profile=opds-catalog;kind=navigation"></link>
  </entry>
  <entry>
    <id>urn:armarium:library:comics</id>
    <title>コミック</title>
    <updated>2026-07-12T01:23:45Z</updated>
    <link rel="subsection" href="/opds/comics" type="application/atom+xml;profile=opds-catalog;kind=navigation"></link>
  </entry>
</feed>
```

ライブラリは設定ファイルの記述順で返します。

## 6. `GET /opds/{library}`

ライブラリ直下、または `path` で指定したサブディレクトリの内容を返します。

### 6.1 パスパラメーター

| 名前 | 必須 | 説明 |
|---|---:|---|
| `library` | はい | 設定の `libraries[].id` |

### 6.2 クエリパラメーター

| 名前 | 必須 | 既定値 | 説明 |
|---|---:|---|---|
| `path` | いいえ | 空文字 | ライブラリルートからの相対ディレクトリパス |

### 6.3 リクエスト例

ライブラリ直下を取得します。

```sh
curl --user reader:change-me http://localhost:8080/opds/books
```

`小説/シリーズA` を取得します。

```sh
curl --user reader:change-me \
  'http://localhost:8080/opds/books?path=%E5%B0%8F%E8%AA%AC%2F%E3%82%B7%E3%83%AA%E3%83%BC%E3%82%BAA'
```

### 6.4 ディレクトリエントリー

サブディレクトリは `rel="subsection"` のリンクを持ちます。

```xml
<entry>
  <id>urn:armarium:dir:books:小説/シリーズA</id>
  <title>シリーズA</title>
  <updated>2026-07-12T01:00:00Z</updated>
  <link rel="subsection"
        href="/opds/books?path=%E5%B0%8F%E8%AA%AC%2F%E3%82%B7%E3%83%AA%E3%83%BC%E3%82%BAA"
        type="application/atom+xml;profile=opds-catalog;kind=navigation"></link>
</entry>
```

### 6.5 書籍エントリー

書籍はOPDS取得リンク `http://opds-spec.org/acquisition` を持ちます。

```xml
<entry>
  <id>urn:armarium:book:books:小説/シリーズA/example.epub</id>
  <title>Example Book.epub</title>
  <updated>2026-07-12T01:00:00Z</updated>
  <author>
    <name>Example Author</name>
  </author>
  <dc:format>application/epub+zip</dc:format>
  <link rel="http://opds-spec.org/acquisition"
        href="/download/books/%E5%B0%8F%E8%AA%AC/%E3%82%B7%E3%83%AA%E3%83%BC%E3%82%BAA/example.epub"
        type="application/epub+zip"
        title="example.epub"
        length="123456"></link>
</entry>
```

書籍の `title` には配信形式の拡張子を付加します。ZIPはCBZとして配信するため `.cbz` を使用し、タイトルが同じ拡張子で終わっている場合は重複して付加しません。`author` はEPUBから著者を取得できた場合だけ出力します。

`dc:format` には配信時のMIMEタイプを出力します。取得リンクの `title` は配信ファイル名、`length` は10進数の正確なバイト数です。

CBZとZIPでは、対応画像ファイルの件数をページ数相当として `dcterms:extent` に `42 pages` の形式で出力します。1件の場合は `1 page` とし、0件の場合は要素を省略します。`content` と `summary` は概要や要約を取得できるようになるまで出力しません。

### 6.6 並び順

ディレクトリとファイルを分離せず、同一一覧内で項目名の大文字・小文字を区別しない昇順に並べます。未対応形式は一覧から除外します。

### 6.7 エラー

| 条件 | ステータス | 本文 |
|---|---:|---|
| `library` が存在しない | `404 Not Found` | Go標準のNot Foundレスポンス |
| `path` が不正、またはライブラリ外を指す | `400 Bad Request` | `不正なパスです` |
| 対象ディレクトリを読み込めない | `404 Not Found` | Go標準のNot Foundレスポンス |

## 7. `GET /download/{library}/{path...}`

指定された対応形式の通常ファイルをダウンロードします。

### 7.1 パスパラメーター

| 名前 | 必須 | 説明 |
|---|---:|---|
| `library` | はい | 設定の `libraries[].id` |
| `path...` | はい | ライブラリルートからの相対ファイルパス |

### 7.2 リクエスト例

```sh
curl --user reader:change-me \
  --remote-header-name \
  --remote-name \
  http://localhost:8080/download/books/novels/example.epub
```

### 7.3 成功レスポンス

```http
HTTP/1.1 200 OK
Content-Type: application/epub+zip
Content-Disposition: attachment; filename="download.epub"; filename*=UTF-8''example.epub
Accept-Ranges: bytes
```

ファイル配信にはGo標準の `http.ServeFile` を使用するため、更新日時、Rangeリクエスト、条件付きリクエストは同関数の挙動に従います。

`Content-Disposition` には、従来形式しか解釈しないクライアント向けのASCIIファイル名 `filename="download.{拡張子}"` と、元のファイル名をUTF-8で表す `filename*` を併記します。

### 7.4 MIMEタイプ

| 拡張子 | Content-Type |
|---|---|
| `.pdf` | `application/pdf` |
| `.epub` | `application/epub+zip` |
| `.cbz` | `application/vnd.comicbook+zip` |
| `.zip` | `application/vnd.comicbook+zip` |

拡張子判定は大文字・小文字を区別しません。

`.zip` は画像書庫としてCBZと同じMIMEタイプで配信します。ダウンロード時の `Content-Disposition` では `filename` と `filename*` の拡張子を `.cbz` に置き換えますが、ライブラリ内の元ファイルは変更しません。書庫内容の検証や別形式からの変換は行いません。

### 7.5 エラー

| 条件 | ステータス | 本文 |
|---|---:|---|
| `library` が存在しない | `404 Not Found` | Go標準のNot Foundレスポンス |
| 相対パスが不正、ライブラリ外、または未対応形式 | `400 Bad Request` | `不正なファイルです` |
| 対象がない、または通常ファイルではない | `404 Not Found` | Go標準のNot Foundレスポンス |

## 8. OPDS要素のマッピング

| 対象 | Atom/OPDS要素 | 値 |
|---|---|---|
| ルートフィードID | `feed/id` | `urn:armarium:root` |
| ライブラリID | `entry/id` | `urn:armarium:library:{library}` |
| フォルダID | `entry/id` | `urn:armarium:dir:{library}:{relative-path}` |
| 書籍ID | `entry/id` | `urn:armarium:book:{library}:{relative-path}` |
| フォルダリンク | `entry/link@rel` | `subsection` |
| 書籍取得リンク | `entry/link@rel` | `http://opds-spec.org/acquisition` |
| 書籍取得形式 | `entry/link@type`、`entry/dc:format` | 配信時のMIMEタイプ |
| 書籍取得名 | `entry/link@title` | 配信時のファイル名。ZIPは `.cbz` |
| 書籍サイズ | `entry/link@length` | 10進数のバイト数 |
| 画像ページ数 | `entry/dcterms:extent` | 対応画像ファイル数。例: `42 pages` |
| 書籍タイトル | `entry/title` | EPUBタイトルまたはファイル名に配信拡張子を付加 |
| 書籍著者 | `entry/author/name` | EPUB creator。空の場合は要素自体を省略 |
| 書籍更新日時 | `entry/updated` | ファイル更新日時 |
| フォルダ更新日時 | `entry/updated` | ディレクトリ更新日時 |
| フィード更新日時 | `feed/updated` | リクエスト処理時刻 |

## 9. API上の現行制約

- OpenSearch、ページング、検索、ソート指定はありません。
- OPDS 2.0 JSONは提供しません。
- 表紙画像、サムネイル、カテゴリ、概要、言語、識別子は提供しません。
- フィードの自己参照リンク、startリンク、upリンクは提供しません。
- ETagやフィード単位の明示的なキャッシュ制御ヘッダーは提供しません。
- エラーレスポンスは構造化JSONまたはXMLではなくプレーンテキストです。
