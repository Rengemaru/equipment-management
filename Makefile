# 開発は全てコンテナ内で行う。このファイルは docker compose の薄いラッパでしかない。
# Windows では make.ps1 が同じタスク名を提供する。__タスクを増やすときは両方に追加する。__
#
# ここに無いタスク（build / create-admin）は、対象の機能を実装した時に足す。
# 動かないターゲットを先に置くと、壊れているのか未実装なのか区別できなくなる。

COMPOSE := docker compose -f compose.dev.yaml

# コンテナ内では sh -lc を使わない。ログインシェルが PATH を上書きして go が消える。
EXEC := $(COMPOSE) exec dev bash -c

.DEFAULT_GOAL := help
.PHONY: help up down sh logs fmt test test-web

help: ## このヘルプを表示する
	@grep -E '^[a-z-]+:.*?## ' $(MAKEFILE_LIST) | awk -F':.*?## ' '{printf "  %-14s %s\n", $$1, $$2}'

up: ## 開発コンテナを起動する
	$(COMPOSE) up -d

down: ## 開発コンテナを停止する
	$(COMPOSE) down

sh: ## コンテナ内のシェルに入る
	$(COMPOSE) exec dev bash

logs: ## コンテナのログを追う
	$(COMPOSE) logs -f

fmt: ## gofmt と go vet をかける
	$(EXEC) 'gofmt -w . && go vet ./...'

test: ## Go のテストを実行する
	$(EXEC) 'go test ./...'

# npm ci を先に流すのは、node_modules が名前付きボリュームにあり、
# package-lock.json を変えても自動では反映されないため。手元と CI で同じ依存を検査する。
#
# テストだけでなく build も通すのは、型検査が `npm run build`（tsc --noEmit）にしか
# 無いため。vitest は esbuild で型を落として実行するので、型エラーを見逃す。
test-web: ## フロントの型検査・ビルド・テストを実行する
	$(EXEC) 'cd web && npm ci && npm run build && npm test'

# ---- 本番 ----
# 対象が compose.yaml（本番）であることを prod- で明示する。
# dev と同じ名前にすると、__止めるつもりで本番を止める__ 事故が起きる。

PROD := docker compose -f compose.yaml

.PHONY: prod-build prod-up prod-down prod-logs prod-ps

prod-build: ## 本番イメージを作り直す
	$(PROD) build

prod-up: ## 本番を起動する（イメージが無ければ作る）
	$(PROD) up -d

prod-down: ## 本番を停止する（データのボリュームは消さない）
	$(PROD) down

prod-logs: ## 本番のログを追う
	$(PROD) logs -f

prod-ps: ## 本番の状態とヘルスチェックの結果を見る
	$(PROD) ps
