# ComfyUI Middle Server 构建说明

本目录包含用于编译和打包 ComfyUI Middle Server 的构建脚本。

## 快速开始

### Linux/macOS

```bash
# 进入构建目录
cd build

# 编译当前平台
./build.sh

# 编译指定平台
./build.sh -o linux -a amd64
./build.sh -o darwin -a arm64
./build.sh -o windows -a amd64

# 使用 Makefile（推荐）
make build-linux
make build-darwin
make build-windows
```

### Windows

```cmd
REM 进入构建目录
cd build

REM 编译当前平台
build.bat

REM 编译指定平台
build.bat -o linux -a amd64
build.bat -o windows -a amd64
```

## 命令行参数

### build.sh / build.bat

- `-o, --os OS` - 指定操作系统 (linux, darwin, windows)
- `-a, --arch ARCH` - 指定架构 (amd64, arm64, 386)
- `-t, --type TYPE` - 构建类型 (debug, release) [默认: release]
- `-v, --version VERSION` - 版本号 (例如: 1.0.0)
- `-d, --output DIR` - 输出目录 [默认: dist]
- `-h, --help` - 显示帮助信息

## 支持的平台

| 操作系统 | 架构 | 说明 |
|---------|------|------|
| linux | amd64 | Linux x86_64 |
| linux | arm64 | Linux ARM64 |
| darwin | amd64 | macOS Intel |
| darwin | arm64 | macOS Apple Silicon |
| windows | amd64 | Windows x64 |
| windows | 386 | Windows x86 |

## 构建输出

构建完成后，会在 `dist/` 目录下生成：

1. **可执行文件**: `comfyui_middleground` (Linux/macOS) 或 `comfyui_middleground.exe` (Windows)
2. **压缩包**: `comfyui_middleground-{OS}-{ARCH}-{VERSION}.tar.gz` 或 `.zip`

压缩包包含：
- 可执行文件
- `config/config.yaml` - 配置文件
- `static/` - 静态资源文件
- `data/` - 数据目录（包含 images 子目录）
- `start.sh` / `start.bat` - 启动脚本
- `README.md` - 使用说明

## 使用示例

### 示例 1: 编译 Linux 版本

```bash
./build.sh -o linux -a amd64 -v 1.0.0
```

输出：
- `dist/comfyui_middleground` - 可执行文件
- `dist/comfyui_middleground-linux-amd64-1.0.0.tar.gz` - 压缩包

### 示例 2: 编译 macOS ARM64 版本

```bash
./build.sh -o darwin -a arm64 -v 1.0.0
```

### 示例 3: 编译 Windows 版本

```bash
./build.sh -o windows -a amd64 -v 1.0.0
```

### 示例 4: 使用 Makefile

```bash
# 编译 Linux 版本
make build-linux

# 编译 macOS 版本
make build-darwin

# 编译 Windows 版本
make build-windows

# 清理构建产物
make clean
```

## 构建类型

- **release** (默认): 优化构建，去除调试信息，减小文件体积
- **debug**: 包含调试信息，便于调试

## 版本号

如果不指定版本号 (`-v`)，脚本会尝试：
1. 使用 Git 标签（如果存在）
2. 使用 Git 提交哈希（如果存在）
3. 使用日期作为版本号

## 注意事项

1. 确保已安装 Go 1.21 或更高版本
2. 确保已安装必要的工具：
   - Linux/macOS: `tar`, `gzip`
   - Windows: PowerShell（用于压缩）
3. **CGO 要求**：
   - SQLite 驱动需要 CGO，构建时会自动启用 CGO
   - 需要安装 C 编译器（gcc/clang）
   - Linux 系统需要安装 SQLite 开发库：
     - Ubuntu/Debian: `sudo apt-get install libsqlite3-dev`
     - CentOS/RHEL: `sudo yum install sqlite-devel`
     - Alpine: `apk add sqlite-dev`
4. **运行时依赖**：
   - Linux: 需要 `libc` 和 `libsqlite3` 库（通常系统已安装）
   - macOS: 系统自带，无需额外安装
   - Windows: 需要 Visual C++ Redistributable（通常已安装）
5. 配置文件需要手动修改，构建脚本不会修改配置内容
6. 构建时使用 `-trimpath` 标志，去除构建路径信息，日志中不会显示开发路径
7. **重要**: 首次运行前需要配置：
   - `comfyui.host` - ComfyUI 服务地址
   - `front_server.ws_url` - 前台服务器 WebSocket 地址
   - `front_server.secret_key` - 与前台服务器一致的密钥

## 故障排除

### 问题: 找不到 go 命令

```bash
# 检查 Go 是否安装
go version

# 如果未安装，请访问 https://golang.org/dl/ 下载安装
```

### 问题: 权限 denied

```bash
# Linux/macOS: 添加执行权限
chmod +x build.sh

# Windows: 以管理员身份运行
```

### 问题: 编译失败

1. 检查 Go 版本是否符合要求
2. 检查依赖是否完整：`go mod download`
3. 查看错误信息，根据提示解决问题

## 开发建议

1. 开发时使用 `debug` 模式：`./build.sh -t debug`
2. 发布时使用 `release` 模式：`./build.sh -t release -v 1.0.0`
3. 建议在 CI/CD 中使用这些脚本进行自动化构建

