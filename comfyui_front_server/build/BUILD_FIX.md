# 构建脚本修复说明

## 问题描述

在 Linux 服务器上运行时出现错误：
```
Binary was compiled with 'CGO_ENABLED=0', go-sqlite3 requires cgo to work.
```

## 原因分析

虽然构建脚本使用了 `sqlite_omit_load_extension` 标签来启用纯 Go SQLite 驱动（modernc.org/sqlite），但 `CGO_ENABLED=0` 的设置可能没有正确应用到 `go build` 命令中。

## 修复方案

已修复构建脚本，确保在编译命令中显式设置 `CGO_ENABLED=0`：

```bash
CGO_ENABLED=0 go build -trimpath -tags "sqlite_omit_load_extension" -ldflags "$ldflags" -o "$OUTPUT_DIR/$binary_name" .
```

这样可以确保：
1. 使用纯 Go SQLite 驱动（modernc.org/sqlite）
2. 不需要 CGO 支持
3. 支持跨平台编译（包括交叉编译）

## 重新编译步骤

### 在 Linux 服务器上编译

```bash
cd comfyui_front_server
bash build/build.sh -o linux -a amd64 -v <version>
```

### 在 macOS 上交叉编译 Linux 版本

```bash
cd comfyui_front_server
bash build/build.sh -o linux -a amd64 -v <version>
```

### 验证编译结果

编译完成后，检查二进制文件：

```bash
file dist/comfyui_front_server
# 应该显示: ELF 64-bit LSB executable, x86-64, version 1 (SYSV), statically linked
```

## 技术说明

### SQLite 驱动选择

`gorm.io/driver/sqlite` 支持两种驱动：

1. **github.com/mattn/go-sqlite3**（需要 CGO）
   - 性能更好
   - 需要 CGO 支持
   - 不支持交叉编译

2. **modernc.org/sqlite**（纯 Go，不需要 CGO）
   - 纯 Go 实现
   - 不需要 CGO
   - 支持交叉编译
   - 性能略低于 CGO 版本

### 构建标签

使用 `sqlite_omit_load_extension` 构建标签时，GORM 会自动选择 `modernc.org/sqlite` 驱动。

### CGO 设置

- `CGO_ENABLED=0`：禁用 CGO，使用纯 Go 驱动
- `CGO_ENABLED=1`：启用 CGO，使用 CGO 驱动（需要 C 编译器）

## 注意事项

1. **重新编译**：如果之前编译的二进制文件有问题，需要重新编译
2. **构建标签**：必须使用 `sqlite_omit_load_extension` 标签
3. **CGO 设置**：必须在编译命令中显式设置 `CGO_ENABLED=0`
4. **静态链接**：编译出的二进制文件是静态链接的，不依赖系统库

## 验证方法

编译完成后，在 Linux 服务器上运行：

```bash
./comfyui_front_server
```

如果看到版本信息而没有 CGO 错误，说明编译成功。

