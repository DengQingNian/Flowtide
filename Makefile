# Flowable Go 常用构建与验收目标。
GO ?= go
BIN_DIR ?= bin

.PHONY: all build test test-mysql test-postgres test-sdk race cover lint fmt-check doccheck contract contract-update check run clean

all: build

## build 构建服务端与数据库迁移工具。
build:
	CGO_ENABLED=0 $(GO) build -trimpath -o $(BIN_DIR)/flowable-server ./cmd/flowable-server
	CGO_ENABLED=0 $(GO) build -trimpath -o $(BIN_DIR)/flowable-migrate ./cmd/flowable-migrate

## test 运行全部普通测试；真实数据库由独立任务显式启用。
test:
	$(GO) test -p 2 -count=1 ./...

## test-mysql 在隔离临时库运行真实 MySQL 测试。
test-mysql:
	FLOWABLE_MYSQL_INTEGRATION=1 $(GO) test -p 2 -count=1 -v ./internal/persistence/mysql ./internal/migration

## test-postgres 在隔离临时库运行 PostgreSQL 测试，连接由环境注入。
test-postgres:
	FLOWABLE_POSTGRES_INTEGRATION=1 $(GO) test -p 2 -count=1 -v ./internal/persistence/postgres ./internal/migration

## test-sdk 从独立 Go 模块验证公共构造入口。
test-sdk:
	cd testdata/sdk && $(GO) test -mod=mod -count=1 ./...

## race 在独立启用 CGO 的环境执行并发检查。
race:
	CGO_ENABLED=1 $(GO) test -p 2 -race -count=1 ./...

## cover 对总量及每个核心库包执行百分之九十语句覆盖率门禁。
cover:
	$(GO) test -p 2 -count=1 -coverprofile=coverage.out ./...
	$(GO) run ./tools/quality -coverage coverage.out -min 0.90

## fmt-check 拒绝未格式化的已跟踪 Go 文件。
fmt-check:
	@test -z "$$(gofmt -l $$(git ls-files '*.go'))"

## doccheck 检查命名函数的中文说明、参数和返回值。
doccheck:
	$(GO) run ./tools/quality -doccheck

## lint 执行格式、中文函数文档与静态检查。
lint: fmt-check doccheck
	$(GO) vet ./...

## contract 校验固定资产摘要与端点清单，行为兼容由后续 Java 回放验收。
contract:
	$(GO) run ./tools/contract

## contract-update 更新方法级端点对照表，变更必须人工审核。
contract-update:
	$(GO) run ./tools/contract -write

## check 执行无需真实数据库的最小交付门禁。
check: lint cover contract test-sdk build

## run 启动本地服务。
run:
	$(GO) run ./cmd/flowable-server -addr :8080

## clean 清理产物。
clean:
	rm -rf $(BIN_DIR) coverage.out
