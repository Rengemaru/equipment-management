# syntax=docker/dockerfile:1

# ============================================================
# dev: 開発用ステージ
#
# VS Code の Dev Containers がこのステージに接続する。
# ローカルに Go / Node を入れない方針のため、開発に必要なものは全てここに入れる。
# ============================================================
FROM golang:1.26-bookworm AS dev

# Node は web/ のビルドとテストに使う。golang イメージには入っていない。
# NodeSource のリポジトリを追加するより、公式 node イメージからコピーする方が
# 手順が短く、ネットワーク上の前提も減る。どちらも bookworm ベースなので glibc が一致する。
COPY --from=node:22-bookworm /usr/local/bin/node /usr/local/bin/node
COPY --from=node:22-bookworm /usr/local/lib/node_modules /usr/local/lib/node_modules
RUN ln -s ../lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm \
 && ln -s ../lib/node_modules/npm/bin/npx-cli.js /usr/local/bin/npx

# sqlite3 は開発時に DB の中身を確認するためだけに入れる。
# 本番イメージには入れない（バックアップは -backup サブコマンドで行う）。
RUN apt-get update \
 && apt-get install -y --no-install-recommends sqlite3 \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /workspace

# 8080 = API / 5173 = Vite
EXPOSE 8080 5173


# ============================================================
# build-web: フロントをビルドする
#
# 成果物は次のステージで Go バイナリに embed する。
# ============================================================
FROM node:22-bookworm AS build-web

WORKDIR /web

# 依存だけを先に入れる。package.json を変えていない限り、この層は
# 使い回される。ソースを1行直すたびに全依存を取り直すのを避ける。
COPY web/package.json web/package-lock.json ./
# npm install ではなく npm ci。lockfile どおりに入れ、
# イメージを作り直すたびに依存が変わることを防ぐ。
RUN npm ci

COPY web/ ./
RUN npm run build


# ============================================================
# build: Go バイナリを作る
# ============================================================
FROM golang:1.26-bookworm AS build

WORKDIR /src

# 依存だけを先に入れる（build-web と同じ理由）。
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# フロントの成果物を置き換える。.dockerignore で web/dist を除いてあるため、
# ホストに古いものがあってもイメージには入らない。
COPY --from=build-web /web/dist ./web/dist

# CGO_ENABLED=0 で静的リンクにする。SQLite は modernc.org/sqlite（純Go）を
# 使っているため cgo は要らない。__これで scratch の上で動く。__
#
# -trimpath はビルド環境のパスをバイナリから消す。
# -s -w はデバッグ情報を落とす。数MB小さくなる。
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server


# ============================================================
# prep: 実行イメージに置くディレクトリを用意する
#
# scratch には mkdir も chown も無いため、ここで作って持ち込む。
# ============================================================
FROM golang:1.26-bookworm AS prep

# 名前付きボリュームは、空の時にイメージ側の中身と所有者を引き継ぐ。
# ここで nobody 所有にしておくと、非rootのまま書き込める。
# __これをやらないと root 所有のボリュームができ、起動しても書き込めない。__
RUN mkdir -p /out/data /out/uploads \
 && chown -R 65534:65534 /out/data /out/uploads

# /tmp を用意する。net/http の multipart は、本文が大きいと
# os.TempDir() に書き出す。今の上限（写真10MB・CSV5MB）では届かないが、
# 無いディレクトリに書こうとした時のエラーは原因が読み取りにくい。
RUN mkdir -m 1777 /out/tmp


# ============================================================
# runtime: 本番用の実行イメージ
#
# scratch。シェルも sqlite3 も入れない。入れる理由が無いものは
# 攻撃面にしかならない。バックアップは -backup サブコマンドで行う。
#
# 中身は「バイナリ1つ」。デプロイの単位を小さく保つ（CLAUDE.md）。
# ============================================================
FROM scratch AS runtime

COPY --from=build /out/server /server
COPY --from=prep --chown=65534:65534 /out/data /data
COPY --from=prep --chown=65534:65534 /out/uploads /uploads
COPY --from=prep /out/tmp /tmp

# nobody。root で動かす理由が無い。
USER 65534:65534

# 既定の置き場所。compose 側で上書きできる。
ENV DB_PATH=/data/app.db \
    UPLOAD_DIR=/uploads

EXPOSE 8080

# シェルを介さない（そもそも無い）。サブコマンドは docker compose exec で
# 引数を足して呼ぶ:
#   docker compose exec app /server -create-admin -login-id yamada -name 山田
#   docker compose exec app /server -backup /data/backup-2026-08-17.db
ENTRYPOINT ["/server"]
