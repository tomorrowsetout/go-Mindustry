# mdt-server Makefile
# 支持 Windows/Linux/macOS

.PHONY: all build clean test run

# 默认目标
all: build

# 构建服务器
build:
	@echo "正在构建 mdt-server..."
	@go build -o bin/mdt-server.exe ./cmd/mdt-server
	@echo "构建完成：bin/mdt-server.exe"

# 清理
clean:
	@echo "正在清理..."
	@-rm -rf bin/*
	@-rm -f *.exe
	@-rm -f *.txt
	@-rm -f *.py
	@-rm -rf temp-*
	@echo "清理完成"

# 测试
test:
	@echo "运行测试..."
	@go test ./...

# 运行服务器
run: build
	@echo "启动服务器..."
	@cd bin && ./mdt-server.exe

# 开发模式（带调试信息）
dev: 
	@echo "开发模式构建..."
	@go build -gcflags="all=-N -l" -o bin/mdt-server-dev.exe ./cmd/mdt-server

# 交叉编译 Windows
build-windows:
	@echo "交叉编译 Windows 版本..."
	@GOOS=windows GOARCH=amd64 go build -o bin/mdt-server-win64.exe ./cmd/mdt-server

# 交叉编译 Linux
build-linux:
	@echo "交叉编译 Linux 版本..."
	@GOOS=linux GOARCH=amd64 go build -o bin/mdt-server-linux64 ./cmd/mdt-server

# 交叉编译 macOS
build-macos:
	@echo "交叉编译 macOS 版本..."
	@GOOS=darwin GOARCH=amd64 go build -o bin/mdt-server-macos64 ./cmd/mdt-server
