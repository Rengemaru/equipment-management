# サークル備品管理システム

大学サークルの備品（本・撮影機材・工具など）を管理するWebアプリ。
**「今どこに何があり、誰が持っているか」を継続的に把握できる状態を作る**ことが目的。

利用者は数十名の学生。主にスマートフォンから使う。オンプレミスで運用する。

> **開発中。** 現在は M0（着手準備）。動くのは `/healthz` のみ。
> 何がどこまで実装済みかは [CLAUDE.md](CLAUDE.md) の「タスク一覧」を見ること。

---

## 必要なもの

| | 用途 |
|---|---|
| [Docker Desktop](https://www.docker.com/products/docker-desktop/) | 開発環境そのもの |
| [VS Code](https://code.visualstudio.com/) + [Dev Containers 拡張](https://marketplace.visualstudio.com/items?itemName=ms-vscode-remote.remote-containers) | コンテナ内で編集・補完するため |

**Go と Node をローカルにインストールする必要はない。** 全てコンテナ内に入っている。

この方針の理由は、開発を macOS と Windows の両方で行うため。ローカル環境に依存させると手順が2本に分かれ、片方が必ず古くなる。

---

## 始め方

### 1. クローンして環境変数を用意する

```bash
git clone https://github.com/Rengemaru/equipment-management.git
cd equipment-management
cp .env.example .env            # Windows: Copy-Item .env.example .env
```

`.env` は無くても起動するが、作っておくと設定を変えやすい。**`.env` はコミットしない。**

### 2. VS Code でコンテナを開く

VS Code でこのフォルダを開き、右下に出る **「Reopen in Container」** を押す。
出ない場合はコマンドパレット（`F1`）から `Dev Containers: Reopen in Container`。

初回はイメージのビルドで数分かかる。

### 3. サーバを起動する

**コンテナ内のターミナル**で:

```bash
go run ./cmd/server
```

別のターミナルから確認:

```bash
curl http://localhost:8080/healthz
# ok
```

ブラウザから `http://localhost:8080/healthz` でも同じ。

---

## よく使うコマンド

ホスト側（VS Code の外）から叩く場合:

| やること | macOS | Windows |
|---|---|---|
| タスク一覧 | `make` | `.\make.ps1` |
| 開発コンテナを起動 | `make up` | `.\make.ps1 up` |
| 停止 | `make down` | `.\make.ps1 down` |
| コンテナ内シェル | `make sh` | `.\make.ps1 sh` |
| ログ | `make logs` | `.\make.ps1 logs` |
| フォーマット | `make fmt` | `.\make.ps1 fmt` |
| テスト | `make test` | `.\make.ps1 test` |

コンテナ内では `go` も `npm` もそのまま使える。

### DBの中身を見る

```bash
sqlite3 /data/app.db          # コンテナ内
```

---

## 構成

- **単一バイナリ + SQLiteファイル1つ**で動く。フロントのビルド成果物は Go バイナリに `embed` する
- バックエンド: Go / 標準 `net/http`（フレームワークなし）
- フロントエンド: React + TypeScript + Vite + Tailwind
- DB: SQLite（`modernc.org/sqlite`。cgo不要）

数年で担当者が全入れ替えするため、**引き継いだ人が読めることをコードの短さより優先している。**

---

## ドキュメント

| ファイル | 内容 |
|---|---|
| [CLAUDE.md](CLAUDE.md) | 開発ガイド・設計思想・**タスク一覧**・落とし穴 |
| [docs/equipment-management-requirements.md](docs/equipment-management-requirements.md) | 要件定義・データモデル |
| [docs/url-design.md](docs/url-design.md) | URL設計・QRの仕様 |
| [docs/schema.sql](docs/schema.sql) | スキーマ（参照用） |
| `docs/m1〜m4-implementation-spec.md` | 各マイルストーンの詳細仕様と受け入れ条件 |

**作業を始める前に [CLAUDE.md](CLAUDE.md) を読むこと。**

---

## まだ無いもの

以下は実装時にこの README へ追記する。**書いてあるのに動かない、という状態を作らない。**

- バックアップ・リストア手順（M1の仕上げ）
- 初期管理者の作成手順（M1）
- 本番デプロイ手順（M1の仕上げ）

進捗は [CLAUDE.md](CLAUDE.md) の「タスク一覧」が正。
