@echo off
REM ComfyUI Middle Server 编译脚本 (Windows)
REM 支持指定编译环境和打包

setlocal enabledelayedexpansion

REM 默认值
set GOOS=
set GOARCH=
set BUILD_TYPE=release
set VERSION=
set OUTPUT_DIR=dist
set PROJECT_NAME=comfyui_middleground

REM 获取项目根目录
for %%I in ("%~dp0..") do set PROJECT_ROOT=%%~fI

REM 解析命令行参数
:parse_args
if "%~1"=="" goto validate_args
if /i "%~1"=="-o" (
    set GOOS=%~2
    shift
    shift
    goto parse_args
)
if /i "%~1"=="--os" (
    set GOOS=%~2
    shift
    shift
    goto parse_args
)
if /i "%~1"=="-a" (
    set GOARCH=%~2
    shift
    shift
    goto parse_args
)
if /i "%~1"=="--arch" (
    set GOARCH=%~2
    shift
    shift
    goto parse_args
)
if /i "%~1"=="-t" (
    set BUILD_TYPE=%~2
    shift
    shift
    goto parse_args
)
if /i "%~1"=="--type" (
    set BUILD_TYPE=%~2
    shift
    shift
    goto parse_args
)
if /i "%~1"=="-v" (
    set VERSION=%~2
    shift
    shift
    goto parse_args
)
if /i "%~1"=="--version" (
    set VERSION=%~2
    shift
    shift
    goto parse_args
)
if /i "%~1"=="-d" (
    set OUTPUT_DIR=%~2
    shift
    shift
    goto parse_args
)
if /i "%~1"=="--output" (
    set OUTPUT_DIR=%~2
    shift
    shift
    goto parse_args
)
if /i "%~1"=="-h" goto show_help
if /i "%~1"=="--help" goto show_help
echo 未知参数: %~1
goto show_help

:show_help
echo ComfyUI Middle Server 编译脚本
echo.
echo 用法: %~nx0 [选项]
echo.
echo 选项:
echo   -o, --os OS           指定操作系统 (linux, darwin, windows)
echo   -a, --arch ARCH       指定架构 (amd64, arm64, 386)
echo   -t, --type TYPE       构建类型 (debug, release) [默认: release]
echo   -v, --version VERSION 版本号 (例如: 1.0.0)
echo   -d, --output DIR      输出目录 [默认: dist]
echo   -h, --help            显示帮助信息
echo.
echo 示例:
echo   %~nx0 -o linux -a amd64
echo   %~nx0 -o windows -a amd64
echo.
exit /b 0

:validate_args
REM 验证参数
if "%GOOS%"=="" (
    echo 未指定操作系统，使用当前系统
    for /f "tokens=*" %%i in ('go env GOOS') do set GOOS=%%i
)

if "%GOARCH%"=="" (
    echo 未指定架构，使用当前架构
    for /f "tokens=*" %%i in ('go env GOARCH') do set GOARCH=%%i
)

REM 创建输出目录
if not exist "%OUTPUT_DIR%" mkdir "%OUTPUT_DIR%"

REM 编译
echo 开始编译...
echo   操作系统: %GOOS%
echo   架构: %GOARCH%
echo   构建类型: %BUILD_TYPE%

cd /d "%PROJECT_ROOT%"

REM 设置编译环境变量
set "GOOS=%GOOS%"
set "GOARCH=%GOARCH%"
REM 使用纯 Go SQLite 驱动（github.com/glebarez/sqlite），禁用 CGO
set "CGO_ENABLED=0"

REM 获取版本号
set BUILD_VERSION=%VERSION%
if "%BUILD_VERSION%"=="" set BUILD_VERSION=dev-%date:~0,4%%date:~5,2%%date:~8,2%

REM 构建 LDFLAGS
set LDFLAGS=-X main.Version=%BUILD_VERSION% -X main.BuildTime=%date% %time%
if /i "%BUILD_TYPE%"=="release" (
    set LDFLAGS=%LDFLAGS% -s -w
)

REM 编译
REM -trimpath 去除构建路径信息，避免日志中显示开发路径
REM github.com/glebarez/sqlite 是纯 Go 驱动，不需要任何 build tags
go build -trimpath -ldflags "%LDFLAGS%" -o "%OUTPUT_DIR%\%PROJECT_NAME%.exe" .

if errorlevel 1 (
    echo 编译失败
    exit /b 1
)

echo 编译成功: %OUTPUT_DIR%\%PROJECT_NAME%.exe

REM 打包
call :package

echo.
echo 构建完成！
exit /b 0

:package
echo 开始打包...

set PACKAGE_NAME=%PROJECT_NAME%-%GOOS%-%GOARCH%-%BUILD_VERSION%
set PACKAGE_DIR=%OUTPUT_DIR%\%PACKAGE_NAME%

REM 创建打包目录
if exist "%PACKAGE_DIR%" rmdir /s /q "%PACKAGE_DIR%"
mkdir "%PACKAGE_DIR%"

REM 复制可执行文件
copy "%OUTPUT_DIR%\%PROJECT_NAME%.exe" "%PACKAGE_DIR%\" >nul

REM 复制配置文件
mkdir "%PACKAGE_DIR%\config"
copy "%PROJECT_ROOT%\config\config.yaml" "%PACKAGE_DIR%\config\" >nul

REM 复制静态文件
if exist "%PROJECT_ROOT%\static" (
    xcopy /E /I /Y "%PROJECT_ROOT%\static" "%PACKAGE_DIR%\static" >nul
)

REM 创建数据目录
mkdir "%PACKAGE_DIR%\data"
mkdir "%PACKAGE_DIR%\data\images"

REM 创建启动脚本
call :create_start_script

REM 创建 README
call :create_readme

REM 打包成 ZIP
cd /d "%OUTPUT_DIR%"
if exist "%PACKAGE_NAME%.zip" del "%PACKAGE_NAME%.zip"
powershell -Command "Compress-Archive -Path '%PACKAGE_NAME%' -DestinationPath '%PACKAGE_NAME%.zip' -Force" >nul

if exist "%PACKAGE_NAME%.zip" (
    echo 打包成功: %OUTPUT_DIR%\%PACKAGE_NAME%.zip
) else (
    echo 打包失败
    exit /b 1
)

exit /b 0

:create_start_script
(
echo @echo off
echo chcp 65001 ^>nul
echo echo 启动 ComfyUI Middle Server...
echo echo.
echo.
echo REM 检查配置文件是否存在
echo if not exist "config\config.yaml" ^(
echo     echo 错误: 配置文件不存在: config\config.yaml
echo     pause
echo     exit /b 1
echo ^)
echo.
echo REM 启动服务
echo start "ComfyUI Middle Server" %%~dp0%PROJECT_NAME%.exe
echo.
echo echo 服务已启动，请查看日志...
echo timeout /t 3 ^>nul
) > "%PACKAGE_DIR%\start.bat"
exit /b 0

:create_readme
(
echo # ComfyUI Middle Server
echo.
echo 版本: %BUILD_VERSION%
echo 平台: %GOOS%/%GOARCH%
echo.
echo ## 文件说明
echo.
echo - `%PROJECT_NAME%.exe` - 主程序可执行文件
echo - `config/config.yaml` - 配置文件
echo - `static/` - 静态资源文件（HTML、CSS、JS）
echo - `data/` - 数据目录（数据库文件和图像文件将存储在此）
echo   - `data/middle.db` - SQLite 数据库文件
echo   - `data/images/` - 图像存储目录
echo.
echo ## 快速开始
echo.
echo ```cmd
echo REM 1. 修改配置文件（可选）
echo notepad config\config.yaml
echo.
echo REM 2. 启动服务
echo start.bat
echo ```
echo.
echo ## 配置说明
echo.
echo 主要配置项位于 `config/config.yaml`：
echo.
echo - `server.host` - 服务监听地址（默认: 127.0.0.1）
echo - `server.port` - 服务端口（默认: 8081）
echo - `database.path` - 数据库文件路径
echo - `storage.image_path` - 图像存储路径
echo - `comfyui.host` - ComfyUI 服务地址
echo - `front_server.ws_url` - 前台服务器 WebSocket 地址
echo - `log.level` - 日志级别（DEBUG, INFO, WARN, ERROR）
echo.
echo ## 注意事项
echo.
echo 1. 首次运行前，请修改 `config/config.yaml` 中的配置：
echo    - `comfyui.host` - 设置为实际的 ComfyUI 服务地址
echo    - `front_server.ws_url` - 设置为前台服务器的 WebSocket 地址
echo    - `front_server.secret_key` - 设置与前台服务器一致的密钥
echo 2. 确保有写入 `data/` 目录的权限
echo 3. 确保 ComfyUI 服务已启动并可访问
echo 4. 生产环境建议使用反向代理（如 Nginx）
) > "%PACKAGE_DIR%\README.md"
exit /b 0

