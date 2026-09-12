# サークル備品管理システム

大学サークルの備品（本・撮影機材・工具など）を管理するWebアプリ。
**「今どこに何があり、誰が持っているか」を継続的に把握できる状態を作る**ことが目的。

利用者は数十名の学生。主にスマートフォンから使う。オンプレミスで運用する。

> **現在は M1（基盤 + 備品マスタ + QR発行）まで。**
> できること: ログイン・ユーザー管理・備品マスタ・CSV一括登録・**CSV書き出し**・QRラベル印刷。
> **貸出と返却はまだ無い**（M2）。何がどこまで実装済みかは [CLAUDE.md](CLAUDE.md) の「タスク一覧」が正。

デプロイは **バイナリ1つ + SQLiteファイル1つ** で完結する。
フロントエンドの成果物も Go バイナリに埋め込んであり、配る物は Docker イメージだけ。

---

## 目次

- [開発する](#開発する) — コードを書く人
- [運用する](#運用する) — 部室のマシンで動かす人
  - [起動する](#起動する)
  - [環境変数](#環境変数)
  - [最初の admin を作る](#最初の-admin-を作る)
  - [ユーザーを追加する](#ユーザーを追加する)
  - [バックアップ](#バックアップ)
  - [復元](#復元)
  - [更新する](#更新する)
  - [様子を見る](#様子を見る)
- [引き継ぐ人へ](#引き継ぐ人へ)

---

## 開発する

### 必要なもの

| | 用途 |
|---|---|
| [Docker Desktop](https://www.docker.com/products/docker-desktop/) | 開発環境そのもの |
| [VS Code](https://code.visualstudio.com/) + [Dev Containers 拡張](https://marketplace.visualstudio.com/items?itemName=ms-vscode-remote.remote-containers) | コンテナ内で編集・補完するため |

**Go と Node をローカルにインストールする必要はない。** 全てコンテナ内に入っている。

この方針の理由は、開発を macOS と Windows の両方で行うため。ローカル環境に依存させると手順が2本に分かれ、片方が必ず古くなる。

### 始め方

```bash
git clone https://github.com/Rengemaru/equipment-management.git
cd equipment-management
cp .env.example .env            # Windows: Copy-Item .env.example .env
```

VS Code でこのフォルダを開き、右下に出る **「Reopen in Container」** を押す。
出ない場合はコマンドパレット（`F1`）から `Dev Containers: Reopen in Container`。初回はイメージのビルドで数分かかる。

**コンテナ内のターミナル**で:

```bash
go run ./cmd/server           # API サーバ
cd web && npm run dev -- --host 0.0.0.0   # 別ターミナルで画面（Vite）
```

`http://localhost:8080/healthz` が `ok` を返せば動いている。

### よく使うコマンド

ホスト側（VS Code の外）から叩く場合:

| やること | macOS | Windows |
|---|---|---|
| タスク一覧 | `make` | `.\make.ps1` |
| 開発コンテナを起動 | `make up` | `.\make.ps1 up` |
| 停止 | `make down` | `.\make.ps1 down` |
| コンテナ内シェル | `make sh` | `.\make.ps1 sh` |
| ログ | `make logs` | `.\make.ps1 logs` |
| フォーマット（gofmt + vet） | `make fmt` | `.\make.ps1 fmt` |
| テスト（Go） | `make test` | `.\make.ps1 test` |
| テスト（フロント） | `make test-web` | `.\make.ps1 test-web` |

コンテナ内では `go` も `npm` もそのまま使える。DBを覗くなら `sqlite3 /data/app.db`。

**コミット前に `fmt` と `test` を通すこと。** 同じ検査が push のたびに CI でも走るが、
CI は最後の網であって、手元で確認しない口実にしない。

---

## 運用する

**部室のマシンに必要なのは Docker だけ。** Go も Node も VS Code も要らない。

開発用（`compose.dev.yaml`）と本番用（`compose.yaml`）は別物。
Make のターゲットも本番側は `prod-` を頭に付けてある。**混同しないこと。**

### 起動する

```bash
cp .env.example .env      # 必ず中身を書き換える（次節）
docker compose up -d
```

Make 経由でも同じ:

| やること | macOS | Windows |
|---|---|---|
| イメージを作り直す | `make prod-build` | `.\make.ps1 prod-build` |
| 起動 | `make prod-up` | `.\make.ps1 prod-up` |
| 停止 | `make prod-down` | `.\make.ps1 prod-down` |
| ログ | `make prod-logs` | `.\make.ps1 prod-logs` |
| 状態・ヘルスチェック | `make prod-ps` | `.\make.ps1 prod-ps` |

`restart: unless-stopped` を付けてあるので、**停電から復帰すると勝手に立ち上がる。**
手で止めた時だけは止まったままになる。

データ（SQLiteと写真）は Docker の名前付きボリュームに入る。
`docker compose down` では消えない。**`down -v` は消える。**

### 環境変数

`.env.example` をコピーして `.env` を作る。**`.env` はコミットしない。**

| 変数 | 必須 | 説明 |
|---|:---:|---|
| `PORT` | | 待ち受けポート。既定 `8080`。変えるとホスト側の公開ポートも追随する |
| `HOST_URL` | ✓ | **QRに焼き込まれるURLの土台。** 末尾にスラッシュを付けない |
| `DB_PATH` | ✓ | SQLiteの置き場所。`/data/app.db` のまま使う |
| `UPLOAD_DIR` | ✓ | 備品写真の保存先。`/uploads` のまま使う |
| `SESSION_SECRET` | ✓ | セッションCookieの署名鍵。32文字以上 |
| `COOKIE_SECURE` | | Cookie に `Secure` を付けるか。既定 `true` |

設定に不備があると**起動せずに理由を全部出して落ちる。** 1つずつ直させない。

#### `HOST_URL` は後から変えられないと思うこと

QRの中身は `{HOST_URL}/i/{code}` になる。**印刷して貼ったラベルは貼り替えられない。**
ホスト名を変えると、それまでに刷った全てのQRが読めなくなる。

**短い名前にすること。** 長いとQRのセル数が増え、小さいラベルに載らなくなる。

#### `SESSION_SECRET` を変えると全員がログアウトする

生成例:

```bash
openssl rand -base64 48
```

**`.env.example` の値のまま起動すると警告が出る。** 本番では必ず変える。

#### `COOKIE_SECURE` は HTTPS の有無で決める

HTTPS を用意せず HTTP で公開する場合、**`false` にしないとログインが無限ループする。**
`Secure` 付きの Cookie はブラウザが保存しないため。

HTTPS があるなら `true` のままにする。既定が `true` なのは、設定を忘れた時に
安全でない側へ倒れないようにするため。

### 最初の admin を作る

**これをやらないと誰もログインできない。** Webの画面からは作れない。

```bash
docker compose exec app /server -create-admin -login-id yamada -name 山田
```

```powershell
.\make.ps1 create-admin -LoginId yamada -Name 山田      # Windows
```
```bash
make create-admin LOGIN_ID=yamada NAME=山田             # macOS
```

**初期パスワードは生成され、この出力に一度だけ表示される。** 控えてから閉じること。
DBにはハッシュしか残らず、再表示はできない。忘れたら作り直すことになる。

パスワードを引数で渡す形にしていないのは、シェルの履歴と `ps` の出力に平文が残るため。

初回ログイン時にパスワードの変更を求められる。

### ユーザーを追加する

2人目以降は画面から追加する。`/admin/users`（admin のみ）。

- 初期パスワードは**作成直後の画面に一度だけ表示される。** 控えるまで画面から消えない
- 卒業した人は**削除せず「無効化」する。** 削除すると貸出履歴の参照先が壊れる
- **最後の admin は無効化できない。** 全員が無効になるとWebから復旧できなくなる
- パスワードを忘れた人には admin が再発行する（自己リセットは無い）

### バックアップ

**定期的に取り、取ったファイルを別の場所に保管すること。**
同じマシンに置いたままでは、ディスクごと失われた時に一緒に消える。

```powershell
.\make.ps1 backup       # Windows
```
```bash
make backup             # macOS
```

リポジトリ直下に `backup-YYYY-MM-DD.db` ができる。手で叩くなら:

```bash
docker compose exec app /server -backup /data/backup-2026-08-27.db
docker compose cp app:/data/backup-2026-08-27.db ./backup-2026-08-27.db
```

- 中身は SQLite の `VACUUM INTO`。**サーバを止めずに一貫したコピーが取れる**
- **同じ名前のファイルには上書きしない。** 世代はファイル名の日付で残す
- 作った後に開いて `integrity_check` と件数まで確認し、その結果を表示する。
  「作れた」ことを成功と報告しない

#### CSVでの控えも取れる

`/admin/items` の **「全備品をCSVで書き出す」**（admin のみ）から、廃棄済みも含めた全件を書き出せる。
BOM付きUTF-8なので Excel でそのまま開ける。

**これはバックアップの代わりにはならない。** 取り込み直しても備品コードは戻らず、
利用者・セッション・写真も入っていない。**システムが死んでも「何があったか」だけは
人が読める形で残す**ための保険で、復元は `-restore` の役目。

両方取ること。用途が違う。

#### なぜ `sqlite3` コマンドを使わないのか

本番イメージは `scratch`（中身はバイナリ1つだけ）で、**シェルも `sqlite3` も入っていない。**
DBも名前付きボリュームにあってホストから直接は見えない。
そのため運用に必要な操作は全てバイナリのサブコマンドとして持たせてある。

単純なファイルコピーも使えない。WALモードで動いており、`app.db` だけを写しても
直近の書き込みは `-wal` 側に残っている。

### 復元

**サーバを止めてから実行する。** 動いているサーバの足元でファイルを差し替えると壊れる。

```powershell
.\make.ps1 restore -File .\backup-2026-08-27.db     # Windows
```
```bash
make restore FILE=./backup-2026-08-27.db            # macOS
```

中でやっていること:

```bash
docker compose stop
docker compose cp ./backup-2026-08-27.db app:/data/restore-src.db
docker compose run --rm app -restore /data/restore-src.db   # 使い捨てのコンテナ
docker compose start
```

`-restore` は、

1. **上書きより先に**戻す元を開いて `integrity_check` と件数を確認する
   （壊れたファイルで上書きしてから気付くと、戻す元も戻す先も失う）
2. `app.db` だけでなく `-wal` / `-shm` ごと置き換える
3. 戻した後にマイグレーションを流す（古いバックアップはスキーマが後ろにいることがある）
4. 件数が一致することまで確認する

**復元まで一度は自分で試しておくこと。** ボリュームごと消してから戻す手順で検証済み。

```bash
docker compose down -v      # 失う想定
docker compose up -d
make restore FILE=./backup-2026-08-27.db
```

戻ったDBには、**バックアップを取った時点のパスワード**が入っている。

### 更新する

本番はソースをマウントしていない。手元のコードを直しても反映されない。

```bash
git pull
docker compose up -d --build
```

マイグレーションは起動のたびに自動で適用される。人手で流す手順は無い。
**更新の前にバックアップを取ること。**

### 様子を見る

```bash
docker compose ps       # STATUS に (healthy) が出る
docker compose logs -f
```

ヘルスチェックは30秒ごとに `/healthz` を叩き、**DBに触れることまで確認する。**
プロセスの生存だけを見ると、ボリュームが外れた状態を正常と報告してしまう。

ログは1ファイル10MB × 5世代で回る。放っておいてもディスクを埋めない。

---

## 引き継ぐ人へ

数年で担当者が全入れ替えする前提で作ってある。**読めることをコードの短さより優先している。**

### まず読むもの

| ファイル | 内容 |
|---|---|
| [CLAUDE.md](CLAUDE.md) | 開発ガイド・**設計思想**・タスク一覧・落とし穴。**最初に読む** |
| [docs/equipment-management-requirements.md](docs/equipment-management-requirements.md) | 要件定義・データモデル |
| [docs/url-design.md](docs/url-design.md) | URL設計・QRの仕様・画面一覧 |
| [docs/schema.sql](docs/schema.sql) | 現行スキーマ（参照用スナップショット） |
| `docs/m1〜m4-implementation-spec.md` | 各マイルストーンの詳細仕様と受け入れ条件 |

**コミット履歴が唯一の引き継ぎ資料**という前提で書いてある。
「なぜこのコードがあるのか」はコミット本文に残っている。`git log` を読むこと。

### 壊すと戻せないもの

- **`HOST_URL`** — QRに焼き付く。変えると印刷済みのラベルが全て読めなくなる
- **備品コード** — `0001` から自動採番し、**再利用しない。** 空き番号も埋めない
- **`users` の行** — 削除しない。卒業者は `is_active = 0`
- **`items` の行** — 物理削除しない。`condition = '廃棄'` で表現する
- **`internal/db/migrations/` の既存ファイル** — 書き換えない。連番で足す

### 設計上の約束

- **記録する手間 < 記録しない手間。** 確認ダイアログ・必須入力・ログイン要求を安易に増やさない
- **承認フローを作らない。** メンバーの報告は即時反映し、運営は事後に追認する
- **権限チェックはAPI側で行う。** UIでボタンを隠すだけにしない
- **`items` に「貸出中フラグ」を持たせない。** 状態は `loans` から導出する

詳しくは [CLAUDE.md](CLAUDE.md) の「設計思想」。

### 未確定のまま残っていること

- **学外からアクセス可能にするか** — 不可なら M2 の事後登録・M3 の通知リンクが機能しない。
  **認証方式では解決できないネットワーク側の課題**として残っている
- **学内SMTPが使えるか** — M1 はメールに依存しないが、M2 の代理登録通知までに確定させる
- **ホスト名の決定** — 短いものにする。QRのセル数に直結する
- **ラベルシールの実物** — 1枚買って、QRのサイズを実測で確認する

### 次にやること

[CLAUDE.md](CLAUDE.md) の「タスク一覧」で、**上から順に最初の未チェック項目。**
M1 → M2 → M3 → M4 の順を崩さないこと。理由も CLAUDE.md に書いてある。
