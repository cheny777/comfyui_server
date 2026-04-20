package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"comfyui_middleground/comfyui"
	"comfyui_middleground/config"
	"comfyui_middleground/database"
	"comfyui_middleground/handlers"
	"comfyui_middleground/logger"
	"comfyui_middleground/models"
	"comfyui_middleground/scheduler"
	"comfyui_middleground/websocket"

	"github.com/gin-gonic/gin"
)

// 构建时注入的版本信息
var (
	Version   = "dev"
	BuildTime = "unknown"
)

func main() {
	// 加载配置
	cfgPath := "config/config.yaml"
	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}

	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		logger.Warnf("加载配置失败，使用默认配置: %v", err)
		cfg = config.GetConfig()
	}

	// 初始化日志级别
	logLevel := cfg.Log.Level
	if logLevel == "" {
		logLevel = "INFO"
	}
	logger.SetLevelString(logLevel)
	logger.Infof("日志级别设置为: %s", logLevel)

	// 输出版本信息
	logger.Infof("ComfyUI Middle Server v%s (Build: %s)", Version, BuildTime)

	// 初始化数据库
	if err := database.InitDB(cfg.Database.Path); err != nil {
		logger.Fatalf("初始化数据库失败: %v", err)
	}
	defer database.CloseDB()

	// 自动迁移表结构
	if err := database.AutoMigrate(
		&models.GenerationRequest{},
		&models.Workflow{},
		&models.MiddleConnection{},
	); err != nil {
		logger.Fatalf("自动迁移表结构失败: %v", err)
	}

	// 创建图像存储目录
	if err := os.MkdirAll(cfg.Storage.ImagePath, 0755); err != nil {
		logger.Fatalf("创建图像存储目录失败: %v", err)
	}

	// 初始化全局ComfyUI客户端（单例模式，保持连接一致性）
	comfyUIClient, err := comfyui.InitGlobalClient(cfg.ComfyUI.Host, cfg.ComfyUI.HTTPTimeout)
	if err != nil {
		logger.Fatalf("初始化ComfyUI客户端失败: %v", err)
	}
	defer func() {
		if comfyUIClient != nil {
			comfyUIClient.Close()
		}
	}()

	// 初始化前台WebSocket客户端
	frontWSClient := websocket.NewFrontWSClient(&cfg.FrontServer)

	// 初始化任务调度器
	taskScheduler := scheduler.NewTaskScheduler(cfg, comfyUIClient, frontWSClient)
	taskScheduler.Start()
	handlers.SetTaskScheduler(taskScheduler)
	handlers.SetTaskSchedulerForWorkflow(taskScheduler)

	// 注册WebSocket消息处理器（统一管理所有消息处理器）
	logger.Infof("注册WebSocket消息处理器...")
	websocket.RegisterMessageHandlers(frontWSClient, taskScheduler)

	// 在后台启动WebSocket连接（连接失败不影响服务启动）
	logger.Infof("正在连接前台WebSocket: %s", cfg.FrontServer.WSURL)
	frontWSClient.Start()

	// 设置Gin路由
	r := setupRouter(cfg)

	// 启动HTTP服务
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	logger.Infof("中台服务启动在: %s", addr)

	// 优雅关闭
	go func() {
		if err := r.Run(addr); err != nil {
			logger.Fatalf("启动服务失败: %v", err)
		}
	}()

	// 等待中断信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("正在关闭服务...")
	frontWSClient.Close()
	comfyUIClient.Close()
	database.CloseDB()
	logger.Info("服务已关闭")
}

func setupRouter(cfg *config.Config) *gin.Engine {
	r := gin.Default()

	// 静态文件服务
	r.Static("/static", "./static")

	// 页面路由 - 直接返回HTML文件（不使用模板引擎，因为现在是Vue应用）
	r.GET("/", func(c *gin.Context) {
		c.File("./static/index.html")
	})
	r.GET("/images", func(c *gin.Context) {
		c.File("./static/images.html")
	})
	r.GET("/workflows", func(c *gin.Context) {
		c.File("./static/workflows.html")
	})
	r.GET("/test", func(c *gin.Context) {
		c.File("./static/test.html")
	})

	// API路由组
	api := r.Group("/api")
	{
		// 任务相关
		tasks := api.Group("/tasks")
		{
			tasks.GET("", handlers.GetTaskList)
			tasks.GET("/:id", handlers.GetTaskDetail)
			tasks.GET("/:id/images", handlers.GetTaskImages)
		}

		// 图像相关API
		images := api.Group("/images")
		{
			images.GET("", handlers.GetImageList)              // 图像列表路由
			images.GET("/detail/:id", handlers.GetImageDetail) // 图像详情路由
		}

		// 图像文件访问（使用独立路径避免路由冲突）
		api.GET("/image-file/:user_id/:task_id/:filename", handlers.GetImageFile)

		// 工作流相关
		workflows := api.Group("/workflows")
		{
			workflows.GET("", handlers.GetWorkflowList)              // 获取工作流列表
			workflows.GET("/:id", handlers.GetWorkflowDetail)        // 获取工作流详情
			workflows.POST("", handlers.CreateWorkflow)              // 创建工作流
			workflows.PUT("/:id", handlers.UpdateWorkflow)           // 更新工作流
			workflows.DELETE("/:id", handlers.DeleteWorkflow)        // 删除工作流
			workflows.POST("/:id/execute", handlers.ExecuteWorkflow) // 执行工作流
		}

		// 测试相关
		test := api.Group("/test")
		{
			test.POST("/request", handlers.SubmitTestRequest)
			test.GET("/templates", handlers.GetWorkflowTemplates)
		}
	}

	// 图像文件服务（使用/api/images/file路径，避免与页面路由冲突）
	// 注意：如果需要直接访问，可以使用 r.Static("/image-files", cfg.Storage.ImagePath)
	// 但为了统一管理和权限控制，建议通过API路由访问

	return r
}
