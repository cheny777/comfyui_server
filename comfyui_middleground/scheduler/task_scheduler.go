package scheduler

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"comfyui_middleground/comfyui"
	"comfyui_middleground/config"
	"comfyui_middleground/logger"
	"comfyui_middleground/models"
)

// FrontWSClientInterface 定义WebSocket客户端接口，避免循环导入
// SendMessage 接受 interface{}，实际使用时需要匹配 websocket.WSMessage 结构
type FrontWSClientInterface interface {
	SendMessage(msg interface{}) error
	IsConnected() bool
}

type TaskScheduler struct {
	comfyUIClient *comfyui.ComfyUIClient
	frontWSClient FrontWSClientInterface
	config        *config.Config
	taskQueue     chan *TaskRequest
	activeTasks   map[string]*TaskContext
	mu            sync.RWMutex
}

type TaskRequest struct {
	RequestID string                 `json:"request_id"`
	UserID    int64                  `json:"user_id"`
	DeviceID  string                 `json:"device_id"`
	Workflow  map[string]interface{} `json:"workflow"`
}

type TaskContext struct {
	RequestID string
	PromptID  string
	UserID    int64
	DeviceID  string
	Status    string
	Progress  int
	Files     []models.FileInfo
}

func NewTaskScheduler(cfg *config.Config, comfyUIClient *comfyui.ComfyUIClient, frontWSClient FrontWSClientInterface) *TaskScheduler {
	return &TaskScheduler{
		comfyUIClient: comfyUIClient,
		frontWSClient: frontWSClient,
		config:        cfg,
		taskQueue:     make(chan *TaskRequest, 100),
		activeTasks:   make(map[string]*TaskContext),
	}
}

func (s *TaskScheduler) Start() {
	// 启动工作协程
	for i := 0; i < s.config.Storage.MaxConcurrentTasks; i++ {
		go s.worker()
	}

	logger.Infof("任务调度器已启动，最大并发任务数: %d", s.config.Storage.MaxConcurrentTasks)
}

func (s *TaskScheduler) SubmitTask(req *TaskRequest) error {
	select {
	case s.taskQueue <- req:
		return nil
	default:
		return fmt.Errorf("任务队列已满")
	}
}

func (s *TaskScheduler) worker() {
	for req := range s.taskQueue {
		s.processTask(req)
	}
}

func (s *TaskScheduler) processTask(req *TaskRequest) {
	logger.Infof("[任务调度] 📋 开始处理任务 | RequestID=%s | UserID=%d | DeviceID=%s",
		req.RequestID, req.UserID, req.DeviceID)

	// 转换工作流格式
	workflow := make(comfyui.Workflow)
	for k, v := range req.Workflow {
		if nodeMap, ok := v.(map[string]interface{}); ok {
			node := comfyui.Node{
				ClassType: nodeMap["class_type"].(string),
				Inputs:    nodeMap["inputs"].(map[string]interface{}),
			}
			workflow[k] = node
		}
	}

	// 提取提示词
	promptText, negativePrompt := comfyui.ExtractPrompts(workflow)
	logger.Debugf("[任务调度] 提取提示词: RequestID=%s, PromptText长度=%d, NegativePrompt长度=%d",
		req.RequestID, len(promptText), len(negativePrompt))

	// 创建生成请求记录
	workflowJSON, _ := json.Marshal(req.Workflow)
	genReq := &models.GenerationRequest{
		RequestID:      req.RequestID,
		UserID:         req.UserID,
		DeviceID:       req.DeviceID,
		PromptText:     promptText,
		NegativePrompt: negativePrompt,
		Workflow:       string(workflowJSON),
		Status:         "pending",
		Progress:       0,
	}

	if err := models.CreateGenerationRequest(genReq); err != nil {
		logger.Errorf("[任务调度] ❌ 创建生成请求记录失败: RequestID=%s, Error=%v", req.RequestID, err)
		return
	}
	logger.Debugf("[任务调度] ✅ 创建生成请求记录成功: RequestID=%s", req.RequestID)

	// 创建任务上下文
	ctx := &TaskContext{
		RequestID: req.RequestID,
		UserID:    req.UserID,
		DeviceID:  req.DeviceID,
		Status:    "pending",
		Progress:  0,
		Files:     []models.FileInfo{},
	}

	s.mu.Lock()
	s.activeTasks[req.RequestID] = ctx
	s.mu.Unlock()

	// 关键修复：先注册监听器，再提交任务，避免错过消息
	// 由于ComfyUI消息是全局广播且不包含promptID，我们需要先注册监听器
	// 使用RequestID作为临时标识，提交后更新为实际promptID
	tempPromptID := fmt.Sprintf("pending_%s", req.RequestID)

	logger.Debugf("[任务调度] 🔔 先注册任务监听器: RequestID=%s, TempPromptID=%s", req.RequestID, tempPromptID)

	// 准备回调函数
	statusCallback := func(status *comfyui.TaskStatus) {
		// 使用RequestID查找任务上下文
		s.mu.RLock()
		taskCtx, exists := s.activeTasks[req.RequestID]
		s.mu.RUnlock()

		if !exists {
			logger.Warnf("[任务调度] 收到状态更新但任务上下文不存在: RequestID=%s", req.RequestID)
			return
		}

		// 更新promptID（如果已设置）
		if taskCtx.PromptID != "" {
			status.PromptID = taskCtx.PromptID
		}

		s.handleTaskStatus(req.RequestID, taskCtx.PromptID, status)
	}

	// 先注册监听器
	if err := s.comfyUIClient.ListenForTask(tempPromptID, statusCallback); err != nil {
		logger.Errorf("[任务调度] 注册监听器失败: RequestID=%s, Error=%v", req.RequestID, err)
		s.updateTaskStatus(req.RequestID, "failed", 0, fmt.Sprintf("注册监听器失败: %v", err))
		return
	}

	// 提交到ComfyUI（在监听器注册之后）
	promptID, err := s.comfyUIClient.QueuePrompt(workflow)
	if err != nil {
		logger.Errorf("[任务调度] ❌ 提交任务到ComfyUI失败: RequestID=%s, Error=%v", req.RequestID, err)
		// 清理临时监听器
		s.comfyUIClient.UnregisterTask(tempPromptID)
		s.updateTaskStatus(req.RequestID, "failed", 0, fmt.Sprintf("提交失败: %v", err))
		return
	}

	// 更新promptID并迁移监听器
	ctx.PromptID = promptID
	logger.Debugf("[任务调度] 🔄 迁移监听器: TempPromptID=%s -> PromptID=%s", tempPromptID, promptID)
	if err := s.comfyUIClient.MigrateTaskListener(tempPromptID, promptID); err != nil {
		logger.Warnf("[任务调度] ⚠️  迁移监听器失败，重新注册: RequestID=%s, Error=%v", req.RequestID, err)
		// 如果迁移失败，取消旧注册，重新注册
		s.comfyUIClient.UnregisterTask(tempPromptID)
		s.comfyUIClient.ListenForTask(promptID, statusCallback)
	}

	logger.Infof("[任务调度] ✅ 任务已提交 | RequestID=%s | PromptID=%s | 进度: %s | 0%%",
		req.RequestID, promptID, getProgressBar(0))

	now := time.Now()
	models.UpdateGenerationRequest(req.RequestID, map[string]interface{}{
		"prompt_id":  promptID,
		"status":     "running",
		"started_at": &now,
	})
}

func (s *TaskScheduler) handleTaskStatus(requestID, promptID string, status *comfyui.TaskStatus) {
	logger.Infof("[任务状态] 收到状态更新: RequestID=%s, PromptID=%s, Status=%s, Progress=%d, Images=%d",
		requestID, promptID, status.Status, status.Progress, len(status.Images))

	s.mu.Lock()
	ctx, exists := s.activeTasks[requestID]
	s.mu.Unlock()

	if !exists {
		logger.Infof("[任务状态] 警告: 任务上下文不存在: RequestID=%s", requestID)
		return
	}

	// 更新状态
	if status.Status != "" && status.Status != ctx.Status {
		ctx.Status = status.Status
		logger.Debugf("[任务状态] 更新状态: RequestID=%s, Status=%s", requestID, status.Status)
	}

	// 更新进度（每5%或关键节点打印一次，减少日志噪音）
	if status.Progress > ctx.Progress {
		ctx.Progress = status.Progress

		// 打印进度条（每5%或100%时打印）
		if status.Progress%5 == 0 || status.Progress == 100 {
			progressBar := getProgressBar(status.Progress)
			stage := getProgressStage(status.Progress)
			logger.Infof("[任务进度] RequestID=%s | %s | %d%% | %s", requestID, progressBar, status.Progress, stage)
		} else {
			logger.Debugf("[任务进度] RequestID=%s | %d%%", requestID, status.Progress)
		}

		if err := models.UpdateGenerationRequest(requestID, map[string]interface{}{
			"progress": status.Progress,
		}); err != nil {
			logger.Errorf("[任务状态] 更新进度失败: RequestID=%s, Error=%v", requestID, err)
		}

		// 通知前台
		s.notifyFront(requestID, "task_progress", map[string]interface{}{
			"task_id":   requestID,
			"prompt_id": promptID,
			"progress":  status.Progress,
			"status":    ctx.Status,
		})
	}

	// 处理图像
	if len(status.Images) > 0 {
		logger.Debugf("[任务状态] 发现图像: RequestID=%s, 图像数量=%d", requestID, len(status.Images))
		if logger.GetLevel() == logger.DEBUG {
			logger.Debugf("[任务状态] 图像列表: %v", status.Images)
		}

		// 检查哪些图像还没有下载
		s.mu.RLock()
		downloadedFiles := make(map[string]bool)
		for _, file := range ctx.Files {
			downloadedFiles[file.Filename] = true
		}
		s.mu.RUnlock()

		// 下载未下载的图像
		var newImages []string
		for _, imgFilename := range status.Images {
			if !downloadedFiles[imgFilename] {
				newImages = append(newImages, imgFilename)
			} else {
				logger.Debugf("[任务状态] 图像已下载，跳过: RequestID=%s, Filename=%s", requestID, imgFilename)
			}
		}

		if len(newImages) > 0 {
			logger.Infof("[任务状态] 📥 开始下载图像: RequestID=%s, 数量=%d", requestID, len(newImages))
			// 顺序下载图像（避免并发问题）
			for _, imgFilename := range newImages {
				s.downloadAndSaveImage(requestID, ctx, imgFilename)
			}
			logger.Debugf("[任务状态] ✅ 所有新图像下载完成: RequestID=%s, 图像数量=%d", requestID, len(newImages))
		}
	}

	// 处理完成或失败
	if status.Status == "completed" {
		// 确保所有图像都已下载
		if len(status.Images) > 0 {
			s.mu.RLock()
			downloadedFiles := make(map[string]bool)
			for _, file := range ctx.Files {
				downloadedFiles[file.Filename] = true
			}
			s.mu.RUnlock()

			// 检查是否有未下载的图像
			for _, imgFilename := range status.Images {
				if !downloadedFiles[imgFilename] {
					logger.Debugf("[任务状态] 任务完成时发现未下载图像，立即下载: RequestID=%s, Filename=%s", requestID, imgFilename)
					s.downloadAndSaveImage(requestID, ctx, imgFilename)
				}
			}
		}
		s.completeTask(requestID, promptID, ctx)
	} else if status.Status == "failed" {
		s.failTask(requestID, promptID, status.Error)
	}
}

func (s *TaskScheduler) downloadAndSaveImage(requestID string, ctx *TaskContext, filename string) {
	logger.Debugf("[图像下载] 开始下载图像: RequestID=%s, Filename=%s", requestID, filename)

	// 构建保存路径
	imageDir := filepath.Join(s.config.Storage.ImagePath, fmt.Sprintf("%d", ctx.UserID), requestID)
	if err := os.MkdirAll(imageDir, 0755); err != nil {
		logger.Errorf("[图像下载] ❌ 创建目录失败: RequestID=%s, Path=%s, Error=%v", requestID, imageDir, err)
		return
	}

	outputPath := filepath.Join(imageDir, filename)

	// 下载图像
	if err := s.comfyUIClient.DownloadImage(filename, "", "output", outputPath); err != nil {
		logger.Errorf("[图像下载] ❌ 下载图像失败: RequestID=%s, Filename=%s, Error=%v", requestID, filename, err)
		return
	}
	logger.Infof("[图像下载] ✅ 下载成功: RequestID=%s, Filename=%s", requestID, filename)

	// 获取文件信息
	fileInfo, err := os.Stat(outputPath)
	if err != nil {
		logger.Errorf("[图像下载] ❌ 获取文件信息失败: RequestID=%s, Path=%s, Error=%v", requestID, outputPath, err)
		return
	}
	logger.Debugf("[图像下载] 文件信息: RequestID=%s, Filename=%s, Size=%.2f KB", requestID, filename, float64(fileInfo.Size())/1024)

	// 创建文件信息
	fileInfoModel := models.FileInfo{
		Filename:         filename,
		OriginalFilename: filename,
		FilePath:         filepath.Join(fmt.Sprintf("%d", ctx.UserID), requestID, filename),
		FileSize:         fileInfo.Size(),
		FileType:         "image/png",
		URL:              fmt.Sprintf("/api/image-file/%d/%s/%s", ctx.UserID, requestID, filename),
	}

	// 添加到上下文
	s.mu.Lock()
	ctx.Files = append(ctx.Files, fileInfoModel)
	filesCount := len(ctx.Files)
	s.mu.Unlock()

	// 更新数据库中的文件信息
	filesInfoJSON, _ := json.Marshal(ctx.Files)
	if err := models.UpdateGenerationRequest(requestID, map[string]interface{}{
		"files_info": string(filesInfoJSON),
	}); err != nil {
		logger.Errorf("[图像下载] ❌ 更新数据库失败: RequestID=%s, Error=%v", requestID, err)
	}

	// 通知前台图像就绪
	s.notifyFront(requestID, "image_ready", map[string]interface{}{
		"task_id": requestID,
		"file":    fileInfoModel,
	})
	logger.Debugf("[图像下载] ✅ 图像处理完成: RequestID=%s, Filename=%s, 当前文件数=%d", requestID, filename, filesCount)
}

// 获取进度条字符串
func getProgressBar(progress int) string {
	const barLength = 30
	filled := int(float64(barLength) * float64(progress) / 100)
	bar := ""
	for i := 0; i < barLength; i++ {
		if i < filled {
			bar += "█"
		} else {
			bar += "░"
		}
	}
	return bar
}

// 获取进度阶段描述
func getProgressStage(progress int) string {
	if progress == 0 {
		return "任务已提交，等待执行"
	} else if progress < 20 {
		return "初始化工作流"
	} else if progress < 40 {
		return "加载模型和资源"
	} else if progress < 60 {
		return "处理提示词"
	} else if progress < 80 {
		return "生成图像中"
	} else if progress < 100 {
		return "图像后处理"
	} else {
		return "任务完成"
	}
}

func (s *TaskScheduler) completeTask(requestID, promptID string, ctx *TaskContext) {
	logger.Infof("[任务完成] ✅ 任务执行完成 | RequestID=%s | PromptID=%s | 图像数=%d",
		requestID, promptID, len(ctx.Files))
	if len(ctx.Files) > 0 && logger.GetLevel() == logger.DEBUG {
		for i, file := range ctx.Files {
			logger.Debugf("[任务完成]   [%d] %s (%.2f KB)", i+1, file.Filename, float64(file.FileSize)/1024)
		}
	}

	now := time.Now()
	if err := models.UpdateGenerationRequest(requestID, map[string]interface{}{
		"status":       "completed",
		"progress":     100,
		"completed_at": &now,
	}); err != nil {
		logger.Errorf("[任务完成] ❌ 更新数据库失败: RequestID=%s, Error=%v", requestID, err)
	}

	// 确保文件信息已更新
	if len(ctx.Files) > 0 {
		filesInfoJSON, _ := json.Marshal(ctx.Files)
		if err := models.UpdateGenerationRequest(requestID, map[string]interface{}{
			"files_info": string(filesInfoJSON),
		}); err != nil {
			logger.Errorf("[任务完成] 更新文件信息失败: RequestID=%s, Error=%v", requestID, err)
		} else {
			logger.Infof("[任务完成] 更新文件信息成功: RequestID=%s, Files=%d", requestID, len(ctx.Files))
		}
	}

	logger.Infof("[任务完成] 通知前台: RequestID=%s, Files=%d", requestID, len(ctx.Files))
	s.notifyFront(requestID, "task_complete", map[string]interface{}{
		"task_id":   requestID,
		"prompt_id": promptID,
		"status":    "completed",
		"images":    ctx.Files,
	})

	s.mu.Lock()
	delete(s.activeTasks, requestID)
	s.mu.Unlock()
	logger.Infof("[任务完成] 任务完成: RequestID=%s, PromptID=%s", requestID, promptID)
}

func (s *TaskScheduler) failTask(requestID, promptID, errorMsg string) {
	logger.Errorf("[任务失败] ❌ 任务执行失败 | RequestID=%s | PromptID=%s | Error=%s",
		requestID, promptID, errorMsg)

	if err := models.UpdateGenerationRequest(requestID, map[string]interface{}{
		"status": "failed",
		"error":  errorMsg,
	}); err != nil {
		logger.Errorf("[任务失败] ❌ 更新数据库失败: RequestID=%s, Error=%v", requestID, err)
	}

	s.notifyFront(requestID, "task_complete", map[string]interface{}{
		"task_id":   requestID,
		"prompt_id": promptID,
		"status":    "failed",
		"error":     errorMsg,
	})

	s.mu.Lock()
	delete(s.activeTasks, requestID)
	s.mu.Unlock()
}

func (s *TaskScheduler) updateTaskStatus(requestID, status string, progress int, errorMsg string) {
	updates := map[string]interface{}{
		"status":   status,
		"progress": progress,
	}
	if errorMsg != "" {
		updates["error"] = errorMsg
	}
	models.UpdateGenerationRequest(requestID, updates)
}

func (s *TaskScheduler) notifyFront(requestID, msgType string, data map[string]interface{}) {
	s.mu.RLock()
	ctx, exists := s.activeTasks[requestID]
	s.mu.RUnlock()

	if !exists {
		logger.Debugf("[通知前台] 任务上下文不存在，跳过通知: RequestID=%s, Type=%s", requestID, msgType)
		return
	}

	// 检查WebSocket连接状态
	if !s.frontWSClient.IsConnected() {
		logger.Debugf("[通知前台] WebSocket未连接，跳过通知: RequestID=%s, Type=%s", requestID, msgType)
		return
	}

	// 构建消息并发送（避免循环导入，使用匿名结构体匹配websocket.WSMessage）
	// 注意：结构体字段必须与 websocket.WSMessage 完全一致
	msg := struct {
		Type      string      `json:"type"`
		RequestID string      `json:"request_id,omitempty"`
		UserID    int64       `json:"user_id,omitempty"`
		Data      interface{} `json:"data"`
	}{
		Type:      msgType,
		RequestID: requestID,
		UserID:    ctx.UserID,
		Data:      data,
	}
	if err := s.frontWSClient.SendMessage(msg); err != nil {
		logger.Errorf("[通知前台] ❌ 发送消息失败: RequestID=%s, Type=%s, Error=%v", requestID, msgType, err)
	} else {
		logger.Debugf("[通知前台] ✅ 发送消息成功: RequestID=%s, Type=%s", requestID, msgType)
	}
}
