# ComfyUI Front Server Makefile
# 简化编译和打包流程

.PHONY: help build build-linux build-darwin build-windows clean package

# 默认目标
help:
	@echo "ComfyUI Front Server 构建工具"
	@echo ""
	@echo "可用目标:"
	@echo "  make build          - 编译当前平台"
	@echo "  make build-linux    - 编译 Linux AMD64"
	@echo "  make build-darwin   - 编译 macOS ARM64"
	@echo "  make build-windows  - 编译 Windows AMD64"
	@echo "  make package        - 编译并打包当前平台"
	@echo "  make clean          - 清理构建产物"
	@echo ""
	@echo "示例:"
	@echo "  make build-linux"
	@echo "  make package"

# 编译当前平台
build:
	@./build.sh

# 编译 Linux AMD64
build-linux:
	@./build.sh -o linux -a amd64

# 编译 macOS ARM64
build-darwin:
	@./build.sh -o darwin -a arm64

# 编译 Windows AMD64
build-windows:
	@./build.sh -o windows -a amd64

# 编译并打包当前平台
package: build
	@./build.sh

# 清理构建产物
clean:
	@echo "清理构建产物..."
	@rm -rf dist
	@echo "清理完成"

