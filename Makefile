# 開発は全てコンテナ内で行う。このファイルは docker compose の薄いラッパでしかない。
# Windows では make.ps1 が同じタスク名を提供する。__タスクを増やすときは両方に追加する。__
#
# ここに無いタスク（build / test-web / create-admin）は、対象の機能を実装した時に足す。
# 動かないターゲットを先に置くと、壊れているのか未実装なのか区別できなくなる。

COMPOSE := docker compose -f compose.dev.yaml

# コンテナ内では sh -lc を使わない。ログインシェルが PATH を上書きして go が消える。
EXEC := $(COMPOSE) exec dev bash -c

.DEFAULT_GOAL := help
.PHONY: help up down sh logs fmt test

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
