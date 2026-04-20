# ComfyUI Go 客户端 Demo

这是一个使用 Go 语言请求 ComfyUI 生成图像的示例程序。

## 功能特性

- 通过 WebSocket 连接到 ComfyUI 服务器
- 提交图像生成任务
- 实时监听生成进度
- 自动下载生成的图像

## 前置要求

1. 已安装并运行 ComfyUI 服务器（默认地址：127.0.0.1:8188）
2. Go 1.21 或更高版本

## 安装依赖

```bash
go mod download
```

## 使用方法

1. 确保 ComfyUI 服务器正在运行
2. 准备你的工作流 JSON 文件（默认使用 `常用.json`）
   - 工作流文件应该包含 ComfyUI 的节点配置
   - 可以从 ComfyUI 界面导出工作流 JSON
3. 修改 `main.go` 中的配置（如需要）：
   - `ComfyUIHost`: ComfyUI 服务器地址（默认：127.0.0.1:8188）
   - `workflowFile`: 工作流 JSON 文件路径（默认：常用.json）

4. 运行程序：

```bash
go run main.go
```

## 工作流说明

程序从 JSON 文件加载工作流配置，而不是在代码中创建。这样可以：
- 使用 ComfyUI 界面中设计好的复杂工作流
- 支持自定义节点和插件
- 方便修改和复用工作流配置

工作流文件格式应该符合 ComfyUI 的标准 JSON 格式，包含节点 ID 和对应的配置信息。

## 输出

生成的图像将保存在 `./output` 目录中。

## 注意事项

- 确保 ComfyUI 服务器已启动并可访问
- 根据你的实际模型文件名修改 `ckpt_name`
- 可以根据需要调整采样参数（steps, cfg, sampler_name 等）

