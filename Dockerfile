# syntax=docker/dockerfile:1

# ============================================================
# dev: 開発用ステージ
#
# VS Code の Dev Containers がこのステージに接続する。
# ローカルに Go / Node を入れない方針のため、開発に必要なものは全てここに入れる。
#
# ビルド用・実行用のステージは M1 の仕上げで追加する。
# 中身が無い段階で本番イメージを書いても検証できず、動かないまま腐るため。
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
