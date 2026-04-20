# ComfyUI 中台服务

中台服务负责连接ComfyUI，管理任务调度，存储生成的图像，并提供管理界面。

## 功能特性

- 连接ComfyUI服务（HTTP + WebSocket）
- 连接前台WebSocket服务（主动连接）
- 任务调度和管理
- 图像下载和存储
- 生成请求记录（SQLite数据库）
- Web管理界面（任务列表、图像列表、测试页面）

## 项目结构

```
comfyui_middleground/
├── main.go                 # 入口文件
├── config/
│   ├── config.go          # 配置管理
│   └── config.yaml        # 配置文件
├── models/
│   └── generation_request.go  # 数据模型
├── database/
│   └── db.go              # 数据库初始化
├── comfyui/
│   └── client.go          # ComfyUI客户端
├── websocket/
│   └── front_client.go    # 前台WebSocket客户端
├── scheduler/
│   └── task_scheduler.go  # 任务调度器
├── handlers/
│   ├── task.go            # 任务处理器
│   ├── image.go           # 图像处理器
│   └── test.go            # 测试处理器
├── static/
│   ├── css/
│   │   └── style.css     # 样式文件
│   ├── js/
│   │   ├── task.js       # 任务列表JS
│   │   ├── image.js      # 图像列表JS
│   │   └── test.js       # 测试页面JS
│   ├── index.html        # 任务列表页面
│   ├── images.html       # 图像列表页面
│   └── test.html         # 测试页面
└── go.mod
```

## 配置说明

编辑 `config/config.yaml`：

```yaml
front_server:
  ws_url: "wss://front.example.com/ws/middle"  # 前台WebSocket地址
  server_id: "middle_server_001"                # 中台服务器ID
  secret_key: "your_middle_secret_key"          # 认证密钥
  reconnect_interval: 10s                       # 重连间隔

comfyui:
  host: "127.0.0.1:8188"                        # ComfyUI地址
  http_timeout: 30s                             # HTTP超时

storage:
  image_path: "./data/images"                   # 图像存储路径
  max_concurrent_tasks: 5                       # 最大并发任务数

database:
  type: "sqlite"
  path: "./data/middle.db"                      # 数据库路径

server:
  host: "0.0.0.0"
  port: 8081                                    # HTTP服务端口
```

## 运行

1. 安装依赖：
```bash
go mod download
```

2. 创建数据目录：
```bash
mkdir -p data/images
```

3. 运行服务：
```bash
go run main.go
```

或编译后运行：
```bash
go build -o middle_server
./middle_server
```

## 访问

- 任务列表：http://localhost:8081/
- 图像列表：http://localhost:8081/images
- 测试页面：http://localhost:8081/test

## API接口

- `GET /api/tasks` - 获取任务列表
- `GET /api/tasks/:id` - 获取任务详情
- `GET /api/tasks/:id/images` - 获取任务图像列表
- `GET /api/images` - 获取图像列表
- `GET /api/images/:user_id/:task_id/:filename` - 获取图像文件
- `POST /api/test/request` - 提交测试请求
- `GET /api/test/templates` - 获取工作流模板

