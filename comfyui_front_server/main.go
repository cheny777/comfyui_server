package main

import (
	"comfyui_front_server/config"
	"comfyui_front_server/database"
	"comfyui_front_server/handlers"
	"comfyui_front_server/logger"
	"comfyui_front_server/middleware"
	"comfyui_front_server/models"
	"comfyui_front_server/websocket"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/gin-gonic/gin"
)

// 构建时注入的版本信息
var (
	Version   = "dev"
	BuildTime = "unknown"
)

func main() {
	// 加载配置
	cfg, err := config.LoadConfig("config/config.yaml")
	if err != nil {
		// 在logger初始化前使用标准log
		fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
		os.Exit(1)
	}

	// 初始化日志
	logger.SetLevelString(cfg.Log.Level)

	// 输出版本信息
	logger.Infof("ComfyUI Front Server v%s (Build: %s)", Version, BuildTime)

	// 初始化数据库
	if err := database.InitDB(cfg.Database.Path); err != nil {
		logger.Fatalf("初始化数据库失败: %v", err)
	}

	// 自动迁移表结构
	if err := database.AutoMigrate(
		&models.User{},
		&models.Task{},
		&models.File{},
	); err != nil {
		logger.Fatalf("数据库迁移失败: %v", err)
	}

	// 设置Gin模式
	gin.SetMode(gin.ReleaseMode)

	// 设置WebSocket回调函数（打破循环导入）
	websocket.SetTaskUpdateCallback(handlers.UpdateTaskStatus)
	websocket.SetFileAddCallback(handlers.AddFile)
	handlers.SetSendTaskToMiddleFunc(websocket.SendTaskToMiddle)

	// 设置路由
	r := setupRouter(cfg)

	// 启动服务器
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	logger.Infof("前台服务启动在: %s", addr)

	// 优雅关闭
	go func() {
		if err := r.Run(addr); err != nil {
			logger.Fatalf("服务器启动失败: %v", err)
		}
	}()

	// 等待中断信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("正在关闭服务器...")

	// 关闭数据库连接
	if err := database.CloseDB(); err != nil {
		logger.Errorf("关闭数据库失败: %v", err)
	}

	logger.Info("服务器已关闭")
}

func setupRouter(cfg *config.Config) *gin.Engine {
	r := gin.Default()

	// CORS中间件
	r.Use(middleware.CORSMiddleware())

	// 静态文件服务
	r.Static("/static", "./static")
	r.LoadHTMLGlob("static/*.html")

	// 首页
	r.GET("/", func(c *gin.Context) {
		c.HTML(200, "index.html", nil)
	})

	// API路由组
	api := r.Group("/api")
	{
		// 用户相关（无需认证）
		user := api.Group("/user")
		{
			user.POST("/init", handlers.InitUser)
			user.GET("/info", middleware.AuthMiddleware(), handlers.GetUserInfo)
			user.PUT("/profile", middleware.AuthMiddleware(), handlers.UpdateUserProfile)
			user.GET("/history", middleware.AuthMiddleware(), handlers.GetUserHistory)
		}

		// 任务相关（需要认证）
		tasks := api.Group("/tasks")
		tasks.Use(middleware.AuthMiddleware())
		{
			tasks.POST("", handlers.CreateTask)
			tasks.GET("", handlers.GetTaskList)
			tasks.GET("/:id", handlers.GetTaskDetail)
			tasks.DELETE("/:id", handlers.DeleteTask)
			tasks.GET("/:id/images", handlers.QueryTaskImageFiles) // 查询任务图像文件列表或单个文件
		}

		// 工作流相关（需要认证）
		workflows := api.Group("/workflows")
		workflows.Use(middleware.AuthMiddleware())
		{
			workflows.GET("", handlers.GetWorkflowList)
		}

		// 文件相关（需要认证）
		files := api.Group("/files")
		files.Use(middleware.AuthMiddleware())
		{
			files.GET("/:user_id/:task_id/:filename", handlers.GetImage)
		}
	}

	// WebSocket路由
	r.GET(cfg.Server.WSPath, websocket.HandleMiddleConnection)                    // 中台连接
	r.GET("/ws/user", middleware.AuthMiddleware(), websocket.HandleUserWebSocket) // 用户连接（用于实时推送）

	return r
}
