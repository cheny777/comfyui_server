# 中台服务实现说明

## 已完成的功能

### 1. 项目基础结构 ✅
- ✅ go.mod 配置
- ✅ 配置文件管理（config/config.go, config/config.yaml）
- ✅ 主入口文件（main.go）

### 2. 数据库模块 ✅
- ✅ SQLite数据库初始化（database/db.go）
- ✅ 生成请求记录表（generation_requests）
- ✅ 数据模型（models/generation_request.go）
- ✅ CRUD操作实现

### 3. ComfyUI客户端模块 ✅
- ✅ ComfyUI连接（HTTP + WebSocket）
- ✅ 任务提交（QueuePrompt）
- ✅ 任务状态监听（ListenForTask）
- ✅ 图像下载（DownloadImage）
- ✅ 提示词提取（ExtractPrompts）

### 4. WebSocket客户端模块 ✅
- ✅ 前台WebSocket连接（websocket/front_client.go）
- ✅ 认证机制
- ✅ 心跳保持
- ✅ 自动重连
- ✅ 消息处理器注册

### 5. 任务调度模块 ✅
- ✅ 任务队列管理（scheduler/task_scheduler.go）
- ✅ 并发控制
- ✅ 任务状态跟踪
- ✅ 图像下载和存储
- ✅ 数据库记录更新
- ✅ 前台通知

### 6. Gin路由和处理器 ✅
- ✅ 任务列表API（handlers/task.go）
- ✅ 图像列表API（handlers/image.go）
- ✅ 测试请求API（handlers/test.go）
- ✅ 静态文件服务
- ✅ HTML页面路由

### 7. 前端页面 ✅
- ✅ 任务列表页面（static/index.html）
- ✅ 图像列表页面（static/images.html）
- ✅ 测试页面（static/test.html）
- ✅ CSS样式（static/css/style.css）
- ✅ JavaScript功能（static/js/*.js）

## 使用说明

### 1. 安装依赖
```bash
cd comfyui_middleground
go mod download
```

### 2. 配置
编辑 `config/config.yaml`，设置：
- 前台WebSocket地址
- ComfyUI地址
- 图像存储路径
- 数据库路径

### 3. 运行
```bash
go run main.go
```

或编译后运行：
```bash
go build -o middle_server
./middle_server
```

### 4. 访问
- 任务列表：http://localhost:8081/
- 图像列表：http://localhost:8081/images
- 测试页面：http://localhost:8081/test

## 注意事项

1. **ComfyUI连接**：确保ComfyUI服务已启动并运行在配置的地址
2. **前台连接**：前台WebSocket服务需要先启动，中台会主动连接
3. **数据库**：首次运行会自动创建数据库和表结构
4. **图像存储**：确保有足够的磁盘空间存储生成的图像

## 待优化项

1. **ComfyUI消息监听优化**：当前每个任务都创建goroutine监听，应该改为统一的消息分发机制
2. **图像元数据解析**：添加图像尺寸、格式等信息的自动解析
3. **错误处理增强**：完善错误处理和重试机制
4. **性能优化**：数据库查询优化、并发控制优化

## API接口

### 任务相关
- `GET /api/tasks` - 获取任务列表（支持分页、筛选、搜索）
- `GET /api/tasks/:id` - 获取任务详情
- `GET /api/tasks/:id/images` - 获取任务图像列表

### 图像相关
- `GET /api/images` - 获取图像列表
- `GET /api/images/:user_id/:task_id/:filename` - 获取图像文件

### 测试相关
- `POST /api/test/request` - 提交测试请求
- `GET /api/test/templates` - 获取工作流模板

