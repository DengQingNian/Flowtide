# Flowable Go 常用构建目标。
GO ?= go
BIN_DIR ?= bin

.PHONY: all build test cover lint run clean

all: build

## build 构建服务端二进制。
build:
	CGO_ENABLED=0 $(GO) build -trimpath -o $(BIN_DIR)/flowable-server ./cmd/flowable-server

## test 运行全部测试。
test:
	$(GO) test ./...

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
