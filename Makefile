# Flowable Go 常用构建目标。
GO ?= go
BIN_DIR ?= bin

.PHONY: all build test test-mysql cover lint run clean

all: build

## build 构建服务端与数据库迁移工具。
build:
	CGO_ENABLED=0 $(GO) build -trimpath -o $(BIN_DIR)/flowable-server ./cmd/flowable-server
	CGO_ENABLED=0 $(GO) build -trimpath -o $(BIN_DIR)/flowable-migrate ./cmd/flowable-migrate

## test 运行全部测试。
test:
	$(GO) test ./...

## test-mysql 在隔离临时库运行真实 MySQL 测试，需要 .env 中的开发凭据。
test-mysql:
	FLOWABLE_MYSQL_INTEGRATION=1 $(GO) test -count=1 -v ./internal/persistence/mysql

## cover 输出覆盖率报告。
cover:
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out | tail -20

## lint 运行 vet。
lint:
	$(GO) vet ./...

## run 启动本地服务。
run:
	$(GO) run ./cmd/flowable-server -addr :8080

## clean 清理产物。
clean:
	rm -rf $(BIN_DIR) coverage.out
