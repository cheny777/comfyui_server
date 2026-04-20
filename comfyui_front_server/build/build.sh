#!/bin/bash

# ComfyUI Front Server 编译脚本
# 支持指定编译环境和打包

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 默认值
GOOS=""
GOARCH=""
BUILD_TYPE="release"
VERSION=""
OUTPUT_DIR="dist"
PROJECT_NAME="comfyui_front_server"
PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# 显示帮助信息
show_help() {
    echo -e "${BLUE}ComfyUI Front Server 编译脚本${NC}"
    echo ""
    echo "用法: $0 [选项]"
    echo ""
    echo "选项:"
    echo "  -o, --os OS           指定操作系统 (linux, darwin, windows)"
    echo "  -a, --arch ARCH       指定架构 (amd64, arm64, 386)"
    echo "  -t, --type TYPE       构建类型 (debug, release) [默认: release]"
    echo "  -v, --version VERSION 版本号 (例如: 1.0.0)"
    echo "  -d, --output DIR      输出目录 [默认: dist]"
    echo "  -h, --help            显示帮助信息"
    echo ""
    echo "示例:"
    echo "  $0 -o linux -a amd64                    # Linux AMD64"
    echo "  $0 -o darwin -a arm64                   # macOS ARM64"
    echo "  $0 -o windows -a amd64                  # Windows AMD64"
    echo "  $0 -o linux -a amd64 -v 1.0.0           # 指定版本号"
    echo ""
    echo "支持的平台组合:"
    echo "  linux/amd64, linux/arm64"
    echo "  darwin/amd64, darwin/arm64"
    echo "  windows/amd64, windows/386"
}

# 解析命令行参数
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            -o|--os)
                GOOS="$2"
                shift 2
                ;;
            -a|--arch)
                GOARCH="$2"
                shift 2
                ;;
            -t|--type)
                BUILD_TYPE="$2"
                shift 2
                ;;
            -v|--version)
                VERSION="$2"
                shift 2
                ;;
            -d|--output)
                OUTPUT_DIR="$2"
                shift 2
                ;;
            -h|--help)
                show_help
                exit 0
                ;;
            *)
                echo -e "${RED}未知参数: $1${NC}"
                show_help
                exit 1
                ;;
        esac
    done
}

# 验证参数
validate_args() {
    if [[ -z "$GOOS" ]]; then
        echo -e "${YELLOW}未指定操作系统，使用当前系统: $(go env GOOS)${NC}"
        GOOS=$(go env GOOS)
    fi
    
    if [[ -z "$GOARCH" ]]; then
        echo -e "${YELLOW}未指定架构，使用当前架构: $(go env GOARCH)${NC}"
        GOARCH=$(go env GOARCH)
    fi
    
    # 验证操作系统
    case $GOOS in
        linux|darwin|windows)
            ;;
        *)
            echo -e "${RED}不支持的操作系统: $GOOS${NC}"
            echo "支持的操作系统: linux, darwin, windows"
            exit 1
            ;;
    esac
    
    # 验证架构
    case $GOARCH in
        amd64|arm64|386)
            ;;
        *)
            echo -e "${RED}不支持的架构: $GOARCH${NC}"
            echo "支持的架构: amd64, arm64, 386"
            exit 1
            ;;
    esac
    
    # 验证构建类型
    case $BUILD_TYPE in
        debug|release)
            ;;
        *)
            echo -e "${RED}不支持的构建类型: $BUILD_TYPE${NC}"
            echo "支持的构建类型: debug, release"
            exit 1
            ;;
    esac
}

# 获取版本号
get_version() {
    if [[ -n "$VERSION" ]]; then
        echo "$VERSION"
    elif command -v git &> /dev/null && git rev-parse --git-dir &> /dev/null; then
        # 尝试从 git 获取版本
        local git_tag=$(git describe --tags --exact-match 2>/dev/null || echo "")
        if [[ -n "$git_tag" ]]; then
            echo "$git_tag"
        else
            local git_commit=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
            echo "dev-${git_commit}"
        fi
    else
        echo "dev-$(date +%Y%m%d)"
    fi
}

# 编译
build() {
    local version=$(get_version)
    local binary_name="$PROJECT_NAME"
    
    # Windows 平台需要 .exe 后缀
    if [[ "$GOOS" == "windows" ]]; then
        binary_name="${PROJECT_NAME}.exe"
    fi
    
    local build_time=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    local ldflags="-X main.Version=${version} -X main.BuildTime=${build_time}"
    
    if [[ "$BUILD_TYPE" == "release" ]]; then
        ldflags="${ldflags} -s -w"  # 去除调试信息，减小体积
    fi
    
    echo -e "${BLUE}开始编译...${NC}"
    echo -e "  操作系统: ${GREEN}${GOOS}${NC}"
    echo -e "  架构: ${GREEN}${GOARCH}${NC}"
    echo -e "  构建类型: ${GREEN}${BUILD_TYPE}${NC}"
    echo -e "  版本: ${GREEN}${version}${NC}"
    
    cd "$PROJECT_ROOT"
    
    # 设置编译环境变量
    export GOOS=$GOOS
    export GOARCH=$GOARCH
    
    # 检查是否为交叉编译
    current_os=$(go env GOOS)
    current_arch=$(go env GOARCH)
    is_cross_compile=false
    if [[ "$GOOS" != "$current_os" ]] || [[ "$GOARCH" != "$current_arch" ]]; then
        is_cross_compile=true
    fi
    
    # 使用纯 Go SQLite 驱动（github.com/glebarez/sqlite），不需要 CGO
    # 该驱动基于 modernc.org/sqlite，完全用 Go 实现，支持跨平台编译
    export CGO_ENABLED=0
    echo -e "${YELLOW}  使用纯 Go SQLite 驱动（github.com/glebarez/sqlite），无需 CGO${NC}"
    if [[ "$is_cross_compile" == true ]]; then
        echo -e "${YELLOW}  交叉编译模式: ${GOOS}/${GOARCH}${NC}"
    fi
    
    # 编译
    # -trimpath 去除构建路径信息，避免日志中显示开发路径
    # github.com/glebarez/sqlite 是纯 Go 驱动，不需要任何 build tags
    echo -e "${BLUE}  编译命令: CGO_ENABLED=0 go build -trimpath -ldflags \"${ldflags}\"${NC}"
    
    # 确保 CGO_ENABLED=0 被设置（在命令中显式设置，确保生效）
    CGO_ENABLED=0 go build -trimpath -ldflags "$ldflags" -o "$OUTPUT_DIR/$binary_name" .
    
    if [[ $? -eq 0 ]]; then
        echo -e "${GREEN}编译成功: ${OUTPUT_DIR}/${binary_name}${NC}"
    else
        echo -e "${RED}编译失败${NC}"
        exit 1
    fi
}

# 打包
package() {
    local version=$(get_version)
    local package_name="${PROJECT_NAME}-${GOOS}-${GOARCH}-${version}"
    local package_dir="$OUTPUT_DIR/$package_name"
    local binary_name="$PROJECT_NAME"
    
    if [[ "$GOOS" == "windows" ]]; then
        binary_name="${PROJECT_NAME}.exe"
    fi
    
    echo -e "${BLUE}开始打包...${NC}"
    
    # 创建打包目录
    mkdir -p "$package_dir"
    
    # 复制可执行文件
    cp "$OUTPUT_DIR/$binary_name" "$package_dir/"
    
    # 复制配置文件
    mkdir -p "$package_dir/config"
    cp "$PROJECT_ROOT/config/config.yaml" "$package_dir/config/"
    
    # 复制静态文件
    if [[ -d "$PROJECT_ROOT/static" ]]; then
        cp -r "$PROJECT_ROOT/static" "$package_dir/"
    fi
    
    # 创建数据目录
    mkdir -p "$package_dir/data"
    
    # 创建启动脚本
    create_start_script "$package_dir" "$binary_name"
    
    # 创建 README
    create_readme "$package_dir" "$version"
    
    # 打包成压缩包
    cd "$OUTPUT_DIR"
    local archive_name="${package_name}.tar.gz"
    if [[ "$GOOS" == "windows" ]]; then
        archive_name="${package_name}.zip"
        if command -v zip &> /dev/null; then
            zip -r "$archive_name" "$package_name" > /dev/null
        else
            echo -e "${YELLOW}未找到 zip 命令，跳过压缩包创建${NC}"
        fi
    else
        # 禁用 macOS 扩展属性，确保跨平台兼容性
        # COPYFILE_DISABLE=1 告诉 macOS tar 不要包含扩展属性
        # --format=ustar 使用 POSIX ustar 格式，兼容性更好
        # 如果 --format 不支持，fallback 到普通 tar 命令
        if COPYFILE_DISABLE=1 tar --format=ustar -czf "$archive_name" "$package_name" 2>/dev/null; then
            : # 成功
        elif COPYFILE_DISABLE=1 tar -czf "$archive_name" "$package_name" 2>/dev/null; then
            : # 使用普通格式
        else
            # 最后的 fallback，不使用 COPYFILE_DISABLE（非 macOS 系统）
            tar -czf "$archive_name" "$package_name"
        fi
    fi
    
    if [[ $? -eq 0 ]]; then
        echo -e "${GREEN}打包成功: ${OUTPUT_DIR}/${archive_name}${NC}"
        echo -e "${BLUE}打包内容:${NC}"
        echo -e "  - ${binary_name} (可执行文件)"
        echo -e "  - config/ (配置文件)"
        echo -e "  - static/ (静态文件)"
        echo -e "  - data/ (数据目录)"
        echo -e "  - start.sh/start.bat (启动脚本)"
        echo -e "  - README.md (说明文档)"
    else
        echo -e "${RED}打包失败${NC}"
        exit 1
    fi
}

# 创建启动脚本
create_start_script() {
    local dir=$1
    local binary=$2
    
    if [[ "$GOOS" == "windows" ]]; then
        cat > "$dir/start.bat" << 'EOF'
@echo off
chcp 65001 >nul
echo 启动 ComfyUI Front Server...
echo.

REM 检查配置文件是否存在
if not exist "config\config.yaml" (
    echo 错误: 配置文件不存在: config\config.yaml
    pause
    exit /b 1
)

REM 启动服务
start "ComfyUI Front Server" %~dp0comfyui_front_server.exe

echo 服务已启动，请查看日志...
timeout /t 3 >nul
EOF
    else
        cat > "$dir/start.sh" << 'EOF'
#!/bin/bash

# 获取脚本所在目录
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

# 检查配置文件
if [ ! -f "config/config.yaml" ]; then
    echo "错误: 配置文件不存在: config/config.yaml"
    exit 1
fi

# 启动服务
echo "启动 ComfyUI Front Server..."
./comfyui_front_server
EOF
        chmod +x "$dir/start.sh"
    fi
}

# 创建 README
create_readme() {
    local dir=$1
    local version=$2
    
    # 确定可执行文件名
    local binary_name="${PROJECT_NAME}"
    if [[ "$GOOS" == "windows" ]]; then
        binary_name="${PROJECT_NAME}.exe"
    fi
    
    cat > "$dir/README.md" << EOF
# ComfyUI Front Server

版本: ${version}
平台: ${GOOS}/${GOARCH}

## 文件说明

- \`${binary_name}\` - 主程序可执行文件
- \`config/config.yaml\` - 配置文件
- \`static/\` - 静态资源文件（HTML、CSS、JS）
- \`data/\` - 数据目录（数据库文件将存储在此）

## 快速开始

### Linux/macOS

\`\`\`bash
# 1. 修改配置文件（可选）
vim config/config.yaml

# 2. 启动服务
./start.sh
\`\`\`

### Windows

\`\`\`cmd
REM 1. 修改配置文件（可选）
notepad config\\config.yaml

REM 2. 启动服务
start.bat
\`\`\`

## 配置说明

主要配置项位于 \`config/config.yaml\`：

- \`server.port\` - 服务端口（默认: 8080）
- \`database.path\` - 数据库文件路径
- \`jwt.secret\` - JWT 密钥（生产环境请修改）
- \`log.level\` - 日志级别（DEBUG, INFO, WARN, ERROR）

## 注意事项

1. 首次运行前，请修改 \`config/config.yaml\` 中的敏感配置（如 JWT secret）
2. 确保有写入 \`data/\` 目录的权限
3. 生产环境建议使用反向代理（如 Nginx）

## 技术支持

如有问题，请查看日志或联系技术支持。
EOF
}

# 主函数
main() {
    parse_args "$@"
    validate_args
    
    # 创建输出目录
    mkdir -p "$OUTPUT_DIR"
    
    # 编译
    build
    
    # 打包
    package
    
    echo ""
    echo -e "${GREEN}✓ 构建完成！${NC}"
}

# 执行主函数
main "$@"

