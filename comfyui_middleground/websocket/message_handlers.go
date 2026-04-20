/*
 * @Author: chenyu02_dxm chenyu02@duxiaoman.com
 * @Date: 2025-12-27 17:30:00
 * @LastEditors: chenyu02_dxm chenyu02@duxiaoman.com
 * @LastEditTime: 2025-12-28 21:18:12
 * @FilePath: /comfyui_server/comfyui_middleground/websocket/message_handlers.go
 * @Description: WebSocket消息处理器注册 - 统一管理前台WebSocket的各种消息处理器
 */

package websocket

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"comfyui_middleground/config"
	"comfyui_middleground/logger"
	"comfyui_middleground/models"
	"comfyui_middleground/scheduler"
)

// RegisterMessageHandlers 注册所有WebSocket消息处理器
// 这个函数集中管理所有的WebSocket消息处理器，使main.go更加简洁
func RegisterMessageHandlers(wsClient *FrontWSClient, taskScheduler *scheduler.TaskScheduler) {
	if wsClient == nil {
		logger.Warnf("WebSocket客户端为空，跳过注册消息处理器")
		return
	}

	if taskScheduler == nil {
		logger.Warnf("任务调度器为空，跳过注册消息处理器")
		return
	}

	// 注册任务提交处理器
	registerTaskSubmitHandler(wsClient, taskScheduler)

	// 注册任务状态查询处理器
	registerTaskStatusQueryHandler(wsClient, taskScheduler)

	// 注册任务取消处理器
	registerTaskCancelHandler(wsClient, taskScheduler)

	// 注册心跳响应处理器
	registerHeartbeatHandler(wsClient)

	// 注册心跳响应处理器（接收前台的心跳响应）
	registerHeartbeatPongHandler(wsClient)

	// 注册图像查询处理器
	registerImageQueryHandler(wsClient)

	// 注册图像文件查看处理器（支持预览、目录列表等）
	registerImageFileQueryHandler(wsClient)

	// 注册队列查询处理器
	registerQueueQueryHandler(wsClient, taskScheduler)

	// 注册工作流列表查询处理器
	registerWorkflowListQueryHandler(wsClient)

	logger.Infof("[WebSocket] ✅ 所有消息处理器注册完成 | 已注册: task_submit, task_status_query, task_cancel, heartbeat_ping, heartbeat_pong, image_query, image_file_query, queue_query, workflow_list_query")
}

// registerTaskSubmitHandler 注册任务提交处理器
// 接收前台发送的任务提交请求，提交到任务调度器
func registerTaskSubmitHandler(wsClient *FrontWSClient, taskScheduler *scheduler.TaskScheduler) {
	wsClient.RegisterHandler("task_submit", func(data map[string]interface{}) {
		logger.Debugf("[WebSocket] 收到任务提交请求: %+v", data)

		// 解析任务ID
		requestID, ok := data["task_id"].(string)
		if !ok || requestID == "" {
			logger.Errorf("[WebSocket] 任务提交失败: 缺少task_id字段")
			return
		}

		// 解析用户ID
		userIDFloat, ok := data["user_id"].(float64)
		if !ok {
			logger.Errorf("[WebSocket] 任务提交失败: 缺少user_id字段, RequestID=%s", requestID)
			return
		}
		userID := int64(userIDFloat)

		// 解析设备ID
		deviceID, ok := data["device_id"].(string)
		if !ok || deviceID == "" {
			logger.Warnf("[WebSocket] 任务提交警告: 缺少device_id字段, RequestID=%s", requestID)
			deviceID = "unknown"
		}

		// 解析工作流
		workflow, ok := data["workflow"].(map[string]interface{})
		if !ok || workflow == nil {
			logger.Errorf("[WebSocket] 任务提交失败: 缺少workflow字段, RequestID=%s", requestID)
			return
		}

		// 创建任务请求
		req := &scheduler.TaskRequest{
			RequestID: requestID,
			UserID:    userID,
			DeviceID:  deviceID,
			Workflow:  workflow,
		}

		logger.Infof("[WebSocket] 📥 提交任务 | RequestID=%s | UserID=%d | DeviceID=%s",
			requestID, userID, deviceID)

		// 提交到任务调度器
		if err := taskScheduler.SubmitTask(req); err != nil {
			logger.Errorf("[WebSocket] ❌ 提交任务失败 | RequestID=%s | Error=%v", requestID, err)

			// 发送错误响应
			sendErrorResponse(wsClient, userID, requestID, "提交任务失败: "+err.Error())
		} else {
			logger.Infof("[WebSocket] ✅ 任务已提交 | RequestID=%s", requestID)

			// 发送成功响应
			sendSuccessResponse(wsClient, userID, requestID, "task_submitted", map[string]interface{}{
				"status": "pending",
			})
		}
	})

	logger.Debugf("[WebSocket] 已注册处理器: task_submit")
}

// registerTaskStatusQueryHandler 注册任务状态查询处理器
// 接收前台发送的任务状态查询请求，返回任务当前状态
func registerTaskStatusQueryHandler(wsClient *FrontWSClient, taskScheduler *scheduler.TaskScheduler) {
	wsClient.RegisterHandler("task_status_query", func(data map[string]interface{}) {
		logger.Debugf("[WebSocket] 收到任务状态查询请求: %+v", data)

		// 解析请求ID（用于响应匹配）
		requestID, ok := data["request_id"].(string)
		if !ok || requestID == "" {
			logger.Errorf("[WebSocket] 任务状态查询失败: 缺少request_id字段")
			return
		}

		// 解析任务ID（用于查询数据库）
		taskID, ok := data["task_id"].(string)
		if !ok || taskID == "" {
			logger.Errorf("[WebSocket] 任务状态查询失败: 缺少task_id字段, RequestID=%s", requestID)
			sendErrorResponse(wsClient, 0, requestID, "缺少task_id字段")
			return
		}

		// 解析用户ID
		userIDFloat, ok := data["user_id"].(float64)
		if !ok {
			logger.Errorf("[WebSocket] 任务状态查询失败: 缺少user_id字段, RequestID=%s", requestID)
			sendErrorResponse(wsClient, 0, requestID, "缺少user_id字段")
			return
		}
		userID := int64(userIDFloat)

		logger.Infof("[WebSocket] 🔍 查询任务状态 | RequestID=%s | TaskID=%s | UserID=%d", requestID, taskID, userID)

		// 从数据库查询任务状态（使用taskID查询）
		task, err := models.GetGenerationRequestByID(taskID)
		if err != nil {
			logger.Errorf("[WebSocket] 查询任务状态失败: TaskID=%s, Error=%v", taskID, err)
			sendErrorResponse(wsClient, userID, requestID, "任务不存在: "+err.Error())
			return
		}

		// 验证用户权限（可选，根据需求决定是否检查）
		if task.UserID != userID {
			logger.Warnf("[WebSocket] 用户权限验证失败: RequestID=%s, UserID=%d, TaskUserID=%d", requestID, userID, task.UserID)
			// 根据业务需求决定是否返回错误或允许查询
		}

		// 解析文件信息
		var files []models.FileInfo
		if task.FilesInfo != "" {
			if err := json.Unmarshal([]byte(task.FilesInfo), &files); err != nil {
				logger.Warnf("[WebSocket] 解析文件信息失败: RequestID=%s, Error=%v", requestID, err)
			}
		}

		// 构建响应数据
		responseData := map[string]interface{}{
			"task_id":         task.RequestID,
			"prompt_id":       task.PromptID,
			"status":          task.Status,
			"progress":        task.Progress,
			"prompt_text":     task.PromptText,
			"negative_prompt": task.NegativePrompt,
			"created_at":      task.CreatedAt,
			"started_at":      task.StartedAt,
			"completed_at":    task.CompletedAt,
			"files":           files,
			"file_count":      len(files),
		}

		if task.Error != "" {
			responseData["error"] = task.Error
		}

		// 发送成功响应（使用原始的requestID，确保能匹配到前台的响应通道）
		sendSuccessResponse(wsClient, userID, requestID, "task_status_response", responseData)
		logger.Debugf("[WebSocket] ✅ 任务状态查询成功: RequestID=%s | TaskID=%s | Status=%s | Progress=%d",
			requestID, task.RequestID, task.Status, task.Progress)
	})

	logger.Debugf("[WebSocket] 已注册处理器: task_status_query")
}

// registerTaskCancelHandler 注册任务取消处理器
// 接收前台发送的任务取消请求，尝试取消正在执行的任务
func registerTaskCancelHandler(wsClient *FrontWSClient, taskScheduler *scheduler.TaskScheduler) {
	wsClient.RegisterHandler("task_cancel", func(data map[string]interface{}) {
		logger.Debugf("[WebSocket] 收到任务取消请求: %+v", data)

		// 解析任务ID
		requestID, ok := data["task_id"].(string)
		if !ok || requestID == "" {
			logger.Errorf("[WebSocket] 任务取消失败: 缺少task_id字段")
			return
		}

		// 解析用户ID
		userIDFloat, ok := data["user_id"].(float64)
		if !ok {
			logger.Errorf("[WebSocket] 任务取消失败: 缺少user_id字段, RequestID=%s", requestID)
			return
		}
		userID := int64(userIDFloat)

		logger.Infof("[WebSocket] 🚫 取消任务 | RequestID=%s | UserID=%d", requestID, userID)

		// TODO: 实现任务取消逻辑
		// 1. 从taskScheduler中移除任务（如果还在队列中）
		// 2. 如果任务正在执行，可能需要向ComfyUI发送中断请求
		// 3. 更新数据库中的任务状态为cancelled
		// 目前先返回未实现的消息
		sendErrorResponse(wsClient, userID, requestID, "任务取消功能待实现")
	})

	logger.Debugf("[WebSocket] 已注册处理器: task_cancel")
}

// registerHeartbeatHandler 注册心跳响应处理器
// 处理前台发送的心跳检测请求
func registerHeartbeatHandler(wsClient *FrontWSClient) {
	wsClient.RegisterHandler("heartbeat_ping", func(data map[string]interface{}) {
		logger.Debugf("[WebSocket] 收到心跳检测请求")

		// 提取用户ID（可选）
		var userID int64 = 0
		if userIDFloat, ok := data["user_id"].(float64); ok {
			userID = int64(userIDFloat)
		}

		// 提取请求ID（可选）
		var requestID string = ""
		if reqID, ok := data["request_id"].(string); ok {
			requestID = reqID
		}

		// 响应心跳
		sendSuccessResponse(wsClient, userID, requestID, "heartbeat_pong", map[string]interface{}{
			"timestamp": data["timestamp"],
		})
	})

	logger.Debugf("[WebSocket] 已注册处理器: heartbeat_ping")
}

// registerHeartbeatPongHandler 注册心跳响应处理器（接收前台的心跳响应）
func registerHeartbeatPongHandler(wsClient *FrontWSClient) {
	wsClient.RegisterHandler("heartbeat_pong", func(data map[string]interface{}) {
		// 心跳响应，记录日志即可（DEBUG级别，减少日志噪音）
		logger.Debugf("[WebSocket] 收到心跳响应: heartbeat_pong")
	})

	logger.Debugf("[WebSocket] 已注册处理器: heartbeat_pong")
}

// registerImageQueryHandler 注册图像查询处理器
// 接收前台发送的图像查询请求，返回任务的图像列表
func registerImageQueryHandler(wsClient *FrontWSClient) {
	wsClient.RegisterHandler("image_query", func(data map[string]interface{}) {
		logger.Debugf("[WebSocket] 收到图像查询请求: %+v", data)

		// 解析任务ID
		requestID, ok := data["task_id"].(string)
		if !ok || requestID == "" {
			logger.Errorf("[WebSocket] 图像查询失败: 缺少task_id字段")
			return
		}

		// 解析用户ID
		userIDFloat, ok := data["user_id"].(float64)
		if !ok {
			logger.Errorf("[WebSocket] 图像查询失败: 缺少user_id字段, RequestID=%s", requestID)
			return
		}
		userID := int64(userIDFloat)

		logger.Infof("[WebSocket] 🖼️  查询图像列表 | RequestID=%s | UserID=%d", requestID, userID)

		// 从数据库查询任务
		task, err := models.GetGenerationRequestByID(requestID)
		if err != nil {
			logger.Errorf("[WebSocket] 查询任务失败: RequestID=%s, Error=%v", requestID, err)
			sendErrorResponse(wsClient, userID, requestID, "任务不存在: "+err.Error())
			return
		}

		// 验证用户权限
		if task.UserID != userID {
			logger.Warnf("[WebSocket] 用户权限验证失败: RequestID=%s, UserID=%d, TaskUserID=%d", requestID, userID, task.UserID)
			sendErrorResponse(wsClient, userID, requestID, "无权访问该任务的图像")
			return
		}

		// 解析文件信息
		var files []models.FileInfo
		if task.FilesInfo != "" {
			if err := json.Unmarshal([]byte(task.FilesInfo), &files); err != nil {
				logger.Warnf("[WebSocket] 解析文件信息失败: RequestID=%s, Error=%v", requestID, err)
				files = []models.FileInfo{}
			}
		}

		// 构建响应数据
		responseData := map[string]interface{}{
			"task_id":     requestID,
			"images":      files,
			"image_count": len(files),
			"status":      task.Status,
		}

		// 发送成功响应
		sendSuccessResponse(wsClient, userID, requestID, "image_query_response", responseData)
		logger.Debugf("[WebSocket] ✅ 图像查询成功: RequestID=%s, ImageCount=%d", requestID, len(files))
	})

	logger.Debugf("[WebSocket] 已注册处理器: image_query")
}

// registerImageFileQueryHandler 注册图像文件查看处理器
// 支持根据任务ID、目录、文件名查询图像文件，返回预览URL或文件列表
func registerImageFileQueryHandler(wsClient *FrontWSClient) {
	wsClient.RegisterHandler("image_file_query", func(data map[string]interface{}) {
		logger.Debugf("[WebSocket] 收到图像文件查询请求: %+v", data)

		// 解析请求ID（用于响应匹配）
		requestID, ok := data["request_id"].(string)
		if !ok || requestID == "" {
			logger.Errorf("[WebSocket] 图像文件查询失败: 缺少request_id字段")
			return
		}

		// 解析用户ID
		userIDFloat, ok := data["user_id"].(float64)
		if !ok {
			logger.Errorf("[WebSocket] 图像文件查询失败: 缺少user_id字段, RequestID=%s", requestID)
			sendErrorResponse(wsClient, 0, requestID, "缺少user_id字段")
			return
		}
		userID := int64(userIDFloat)

		// 解析任务ID（必需）
		taskID, ok := data["task_id"].(string)
		if !ok || taskID == "" {
			logger.Errorf("[WebSocket] 图像文件查询失败: 缺少task_id字段, RequestID=%s", requestID)
			sendErrorResponse(wsClient, userID, requestID, "缺少task_id字段")
			return
		}

		// 解析目录（可选，默认为任务目录）
		directory, _ := data["directory"].(string)

		// 解析文件名（可选，如果指定则返回单个文件，否则返回任务的所有图像）
		filename, _ := data["filename"].(string)

		logger.Infof("[WebSocket] 🖼️  查询图像文件 | RequestID=%s | TaskID=%s | UserID=%d | Directory=%s | Filename=%s",
			requestID, taskID, userID, directory, filename)

		// 获取配置
		cfg := config.GetConfig()

		// 如果指定了文件名，返回单个文件信息和内容
		if filename != "" {
			// 构建基础路径：{ImagePath}/{userID}/{taskID}
			basePath := filepath.Join(cfg.Storage.ImagePath, fmt.Sprintf("%d", userID), taskID)

			// 如果指定了目录，追加到路径
			if directory != "" {
				basePath = filepath.Join(basePath, directory)
			}

			// 验证路径安全性（防止路径遍历攻击）
			absBasePath, err := filepath.Abs(basePath)
			if err != nil {
				logger.Errorf("[WebSocket] 获取绝对路径失败: Path=%s, Error=%v", basePath, err)
				sendErrorResponse(wsClient, userID, requestID, "路径错误: "+err.Error())
				return
			}

			absImagePath, err := filepath.Abs(cfg.Storage.ImagePath)
			if err != nil {
				logger.Errorf("[WebSocket] 获取图像存储绝对路径失败: Path=%s, Error=%v", cfg.Storage.ImagePath, err)
				sendErrorResponse(wsClient, userID, requestID, "配置错误")
				return
			}

			// 确保路径在图像存储目录内
			if !filepath.HasPrefix(absBasePath, absImagePath) {
				logger.Errorf("[WebSocket] 路径安全检查失败: BasePath=%s, ImagePath=%s", absBasePath, absImagePath)
				sendErrorResponse(wsClient, userID, requestID, "无效的路径")
				return
			}
			filePath := filepath.Join(absBasePath, filename)

			// 检查文件是否存在
			fileInfo, err := os.Stat(filePath)
			if err != nil {
				if os.IsNotExist(err) {
					logger.Warnf("[WebSocket] 文件不存在: Path=%s", filePath)
					sendErrorResponse(wsClient, userID, requestID, "文件不存在")
					return
				}
				logger.Errorf("[WebSocket] 获取文件信息失败: Path=%s, Error=%v", filePath, err)
				sendErrorResponse(wsClient, userID, requestID, "获取文件信息失败: "+err.Error())
				return
			}

			// 检查文件大小，避免读取过大的文件（限制为50MB）
			const maxFileSize = 50 * 1024 * 1024 // 50MB
			if fileInfo.Size() > maxFileSize {
				logger.Warnf("[WebSocket] 文件过大，跳过内容读取: Path=%s | Size=%d", filePath, fileInfo.Size())
				sendErrorResponse(wsClient, userID, requestID, fmt.Sprintf("文件过大（%d字节），超过限制（%d字节）", fileInfo.Size(), maxFileSize))
				return
			}

			// 读取文件内容
			fileContent, err := os.ReadFile(filePath)
			if err != nil {
				logger.Errorf("[WebSocket] 读取文件内容失败: Path=%s, Error=%v", filePath, err)
				sendErrorResponse(wsClient, userID, requestID, "读取文件内容失败: "+err.Error())
				return
			}

			// 将文件内容编码为base64
			fileContentBase64 := base64.StdEncoding.EncodeToString(fileContent)
			logger.Debugf("[WebSocket] 文件内容已读取: Path=%s | Size=%d | Base64Size=%d",
				filePath, fileInfo.Size(), len(fileContentBase64))

			// 构建文件URL
			relativePath := filepath.Join(fmt.Sprintf("%d", userID), taskID)
			if directory != "" {
				relativePath = filepath.Join(relativePath, directory)
			}
			fileURL := fmt.Sprintf("/api/image-file/%d/%s/%s", userID, taskID, filepath.Join(directory, filename))
			if directory == "" {
				fileURL = fmt.Sprintf("/api/image-file/%d/%s/%s", userID, taskID, filename)
			}

			// 获取文件MIME类型
			ext := filepath.Ext(filename)
			fileType := getMimeType(ext)

			// 构建响应数据
			responseData := map[string]interface{}{
				"task_id":        taskID,
				"directory":      directory,
				"filename":       filename,
				"file_path":      filepath.Join(relativePath, filename),
				"file_size":      fileInfo.Size(),
				"file_url":       fileURL,
				"file_type":      fileType,
				"file_content":   fileContentBase64, // base64编码的文件内容
				"content_format": "base64",          // 内容格式标识
				"exists":         true,
			}

			// 如果是图像文件，可以添加预览URL和数据URL
			if isImageFile(ext) {
				responseData["preview_url"] = fileURL
				// 添加data URL，方便前端直接使用
				dataURL := fmt.Sprintf("data:%s;base64,%s", fileType, fileContentBase64)
				responseData["data_url"] = dataURL
			}

			sendSuccessResponse(wsClient, userID, requestID, "image_file_response", responseData)
			logger.Debugf("[WebSocket] ✅ 图像文件查询成功: RequestID=%s | File=%s | Size=%d | ContentSize=%d",
				requestID, filename, fileInfo.Size(), len(fileContentBase64))
			return
		}

		// 如果没有指定文件名，但有任务ID，从数据库获取任务的所有图像文件并返回base64编码内容
		// 从数据库查询任务信息
		task, err := models.GetGenerationRequestByID(taskID)
		if err != nil {
			logger.Errorf("[WebSocket] 查询任务失败: TaskID=%s, Error=%v", taskID, err)
			sendErrorResponse(wsClient, userID, requestID, "任务不存在: "+err.Error())
			return
		}

		// 验证用户权限
		if task.UserID != userID {
			logger.Warnf("[WebSocket] 用户权限验证失败: RequestID=%s, UserID=%d, TaskUserID=%d", requestID, userID, task.UserID)
			sendErrorResponse(wsClient, userID, requestID, "无权访问该任务")
			return
		}

		// 解析文件信息
		var files []models.FileInfo
		if task.FilesInfo != "" {
			if err := json.Unmarshal([]byte(task.FilesInfo), &files); err != nil {
				logger.Warnf("[WebSocket] 解析文件信息失败: RequestID=%s, Error=%v", requestID, err)
				// 即使解析失败，也返回空列表而不是错误
				files = []models.FileInfo{}
			}
		}

		// 构建基础路径：{ImagePath}/{userID}/{taskID}
		basePath := filepath.Join(cfg.Storage.ImagePath, fmt.Sprintf("%d", userID), taskID)

		// 如果指定了目录，追加到路径
		if directory != "" {
			basePath = filepath.Join(basePath, directory)
		}

		// 验证路径安全性（防止路径遍历攻击）
		absBasePath, err := filepath.Abs(basePath)
		if err != nil {
			logger.Errorf("[WebSocket] 获取绝对路径失败: Path=%s, Error=%v", basePath, err)
			sendErrorResponse(wsClient, userID, requestID, "路径错误: "+err.Error())
			return
		}

		absImagePath, err := filepath.Abs(cfg.Storage.ImagePath)
		if err != nil {
			logger.Errorf("[WebSocket] 获取图像存储绝对路径失败: Path=%s, Error=%v", cfg.Storage.ImagePath, err)
			sendErrorResponse(wsClient, userID, requestID, "配置错误")
			return
		}

		// 确保路径在图像存储目录内
		if !filepath.HasPrefix(absBasePath, absImagePath) {
			logger.Errorf("[WebSocket] 路径安全检查失败: BasePath=%s, ImagePath=%s", absBasePath, absImagePath)
			sendErrorResponse(wsClient, userID, requestID, "无效的路径")
			return
		}

		// 读取所有图像文件并编码为base64
		var imageFiles []map[string]interface{}
		const maxFileSize = 50 * 1024 * 1024 // 50MB

		// 如果数据库中有文件信息，优先使用数据库中的文件列表
		if len(files) > 0 {
			for _, fileInfo := range files {
				// 构建文件路径
				filePath := filepath.Join(absBasePath, fileInfo.Filename)

				// 检查文件是否存在
				fileStat, err := os.Stat(filePath)
				if err != nil {
					if os.IsNotExist(err) {
						logger.Warnf("[WebSocket] 文件不存在，跳过: Path=%s", filePath)
						continue
					}
					logger.Warnf("[WebSocket] 获取文件信息失败，跳过: Path=%s, Error=%v", filePath, err)
					continue
				}

				// 检查文件大小
				if fileStat.Size() > maxFileSize {
					logger.Warnf("[WebSocket] 文件过大，跳过: Path=%s | Size=%d", filePath, fileStat.Size())
					continue
				}

				// 只处理图像文件
				ext := filepath.Ext(fileInfo.Filename)
				if !isImageFile(ext) {
					continue
				}

				// 读取文件内容
				fileContent, err := os.ReadFile(filePath)
				if err != nil {
					logger.Warnf("[WebSocket] 读取文件内容失败，跳过: Path=%s, Error=%v", filePath, err)
					continue
				}

				// 编码为base64
				fileContentBase64 := base64.StdEncoding.EncodeToString(fileContent)
				fileType := getMimeType(ext)
				fileURL := fmt.Sprintf("/api/image-file/%d/%s/%s", userID, taskID, fileInfo.Filename)
				dataURL := fmt.Sprintf("data:%s;base64,%s", fileType, fileContentBase64)

				imageFile := map[string]interface{}{
					"filename":          fileInfo.Filename,
					"original_filename": fileInfo.OriginalFilename,
					"file_path":         fileInfo.FilePath,
					"file_size":         fileStat.Size(),
					"file_url":          fileURL,
					"file_type":         fileType,
					"width":             fileInfo.Width,
					"height":            fileInfo.Height,
					"file_content":      fileContentBase64, // base64编码的文件内容
					"content_format":    "base64",          // 内容格式标识
					"preview_url":       fileURL,
					"data_url":          dataURL, // data URL，方便前端直接使用
					"exists":            true,
				}

				imageFiles = append(imageFiles, imageFile)
			}
		} else {
			// 如果数据库中没有文件信息，从文件系统读取目录中的所有图像文件
			entries, err := os.ReadDir(absBasePath)
			if err != nil {
				if os.IsNotExist(err) {
					logger.Warnf("[WebSocket] 目录不存在: Path=%s", absBasePath)
					// 返回空列表而不是错误
					sendSuccessResponse(wsClient, userID, requestID, "image_file_response", map[string]interface{}{
						"task_id":   taskID,
						"directory": directory,
						"images":    []interface{}{},
						"count":     0,
					})
					return
				}
				logger.Errorf("[WebSocket] 读取目录失败: Path=%s, Error=%v", absBasePath, err)
				sendErrorResponse(wsClient, userID, requestID, "读取目录失败: "+err.Error())
				return
			}

			// 读取所有图像文件并构建FileInfo列表，用于更新数据库
			var newFiles []models.FileInfo

			// 读取所有图像文件
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}

				ext := filepath.Ext(entry.Name())
				if !isImageFile(ext) {
					continue
				}

				entryPath := filepath.Join(absBasePath, entry.Name())
				fileInfo, err := entry.Info()
				if err != nil {
					logger.Warnf("[WebSocket] 获取文件信息失败: Path=%s, Error=%v", entryPath, err)
					continue
				}

				// 检查文件大小
				if fileInfo.Size() > maxFileSize {
					logger.Warnf("[WebSocket] 文件过大，跳过: Path=%s | Size=%d", entryPath, fileInfo.Size())
					continue
				}

				// 读取文件内容
				fileContent, err := os.ReadFile(entryPath)
				if err != nil {
					logger.Warnf("[WebSocket] 读取文件内容失败，跳过: Path=%s, Error=%v", entryPath, err)
					continue
				}

				// 编码为base64
				fileContentBase64 := base64.StdEncoding.EncodeToString(fileContent)
				fileType := getMimeType(ext)
				fileURL := fmt.Sprintf("/api/image-file/%d/%s/%s", userID, taskID, entry.Name())
				dataURL := fmt.Sprintf("data:%s;base64,%s", fileType, fileContentBase64)

				imageFile := map[string]interface{}{
					"filename":       entry.Name(),
					"file_path":      filepath.Join(fmt.Sprintf("%d", userID), taskID, entry.Name()),
					"file_size":      fileInfo.Size(),
					"file_url":       fileURL,
					"file_type":      fileType,
					"file_content":   fileContentBase64, // base64编码的文件内容
					"content_format": "base64",          // 内容格式标识
					"preview_url":    fileURL,
					"data_url":       dataURL, // data URL，方便前端直接使用
					"exists":         true,
				}

				imageFiles = append(imageFiles, imageFile)

				// 构建FileInfo用于更新数据库
				newFiles = append(newFiles, models.FileInfo{
					Filename:         entry.Name(),
					OriginalFilename: entry.Name(),
					FilePath:         filepath.Join(fmt.Sprintf("%d", userID), taskID, entry.Name()),
					FileSize:         fileInfo.Size(),
					FileType:         fileType,
					URL:              fileURL,
				})
			}

			// 如果找到了文件，更新数据库中的文件信息
			if len(newFiles) > 0 {
				filesInfoJSON, err := json.Marshal(newFiles)
				if err == nil {
					if err := models.UpdateGenerationRequest(taskID, map[string]interface{}{
						"files_info": string(filesInfoJSON),
					}); err != nil {
						logger.Warnf("[WebSocket] 更新数据库文件信息失败: TaskID=%s, Error=%v", taskID, err)
					} else {
						logger.Debugf("[WebSocket] ✅ 已更新数据库文件信息: TaskID=%s, Files=%d", taskID, len(newFiles))
					}
				} else {
					logger.Warnf("[WebSocket] 序列化文件信息失败: TaskID=%s, Error=%v", taskID, err)
				}
			}
		}

		// 构建响应数据
		responseData := map[string]interface{}{
			"task_id":   taskID,
			"directory": directory,
			"images":    imageFiles,
			"count":     len(imageFiles),
		}

		sendSuccessResponse(wsClient, userID, requestID, "image_file_response", responseData)
		logger.Debugf("[WebSocket] ✅ 任务图像文件查询成功: RequestID=%s | TaskID=%s | Count=%d",
			requestID, taskID, len(imageFiles))
	})

	logger.Debugf("[WebSocket] 已注册处理器: image_file_query")
}

// 辅助函数：判断是否为图像文件
func isImageFile(ext string) bool {
	ext = strings.ToLower(ext)
	return ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".gif" || ext == ".webp" || ext == ".bmp"
}

// 辅助函数：根据扩展名获取文件类型
func getFileType(ext string) string {
	ext = strings.ToLower(ext)
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp":
		return "image"
	case ".txt", ".log":
		return "text"
	case ".json":
		return "json"
	default:
		return "unknown"
	}
}

// 辅助函数：根据扩展名获取MIME类型
func getMimeType(ext string) string {
	ext = strings.ToLower(ext)
	switch ext {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".bmp":
		return "image/bmp"
	case ".txt":
		return "text/plain"
	case ".json":
		return "application/json"
	case ".html":
		return "text/html"
	case ".css":
		return "text/css"
	case ".js":
		return "application/javascript"
	default:
		return "application/octet-stream"
	}
}

// registerQueueQueryHandler 注册队列查询处理器
// 接收前台发送的队列查询请求，返回当前任务队列状态
func registerQueueQueryHandler(wsClient *FrontWSClient, taskScheduler *scheduler.TaskScheduler) {
	wsClient.RegisterHandler("queue_query", func(data map[string]interface{}) {
		logger.Debugf("[WebSocket] 收到队列查询请求: %+v", data)

		// 解析用户ID（可选）
		var userID int64 = 0
		if userIDFloat, ok := data["user_id"].(float64); ok {
			userID = int64(userIDFloat)
		}

		// 提取请求ID（可选）
		var requestID string = ""
		if reqID, ok := data["request_id"].(string); ok {
			requestID = reqID
		}

		logger.Infof("[WebSocket] 📊 查询队列状态 | UserID=%d", userID)

		// 从数据库查询不同状态的任务数量
		var pendingCount, runningCount, completedCount, failedCount int64

		// 使用 models 包的查询方法获取总数
		// 注意：GetGenerationRequests 返回的第二个值是总数
		if userID > 0 {
			// 查询指定用户的任务
			_, pendingCount, _ = models.GetGenerationRequests(1, 1, userID, "", "pending", "")
			_, runningCount, _ = models.GetGenerationRequests(1, 1, userID, "", "running", "")
			_, completedCount, _ = models.GetGenerationRequests(1, 1, userID, "", "completed", "")
			_, failedCount, _ = models.GetGenerationRequests(1, 1, userID, "", "failed", "")
		} else {
			// 查询所有用户的任务
			_, pendingCount, _ = models.GetGenerationRequests(1, 1, 0, "", "pending", "")
			_, runningCount, _ = models.GetGenerationRequests(1, 1, 0, "", "running", "")
			_, completedCount, _ = models.GetGenerationRequests(1, 1, 0, "", "completed", "")
			_, failedCount, _ = models.GetGenerationRequests(1, 1, 0, "", "failed", "")
		}

		// 构建响应数据
		responseData := map[string]interface{}{
			"pending_count":   pendingCount,
			"running_count":   runningCount,
			"completed_count": completedCount,
			"failed_count":    failedCount,
			"total_count":     pendingCount + runningCount + completedCount + failedCount,
		}

		// 发送成功响应
		sendSuccessResponse(wsClient, userID, requestID, "queue_query_response", responseData)
		logger.Debugf("[WebSocket] ✅ 队列查询成功: Pending=%d, Running=%d, Completed=%d, Failed=%d",
			pendingCount, runningCount, completedCount, failedCount)
	})

	logger.Debugf("[WebSocket] 已注册处理器: queue_query")
}

// SendWSMessage 发送WebSocket消息（导出函数，供其他包调用）
// 这是一个通用的消息发送函数，用于统一管理WebSocket消息发送
func SendWSMessage(wsClient *FrontWSClient, userID int64, requestID string, msgType string, data map[string]interface{}) {
	msg := WSMessage{
		Type:      msgType,
		RequestID: requestID,
		UserID:    userID,
		Data:      data,
	}

	if err := wsClient.SendMessage(msg); err != nil {
		logger.Errorf("[WebSocket] ❌ 发送消息失败 | Type=%s | RequestID=%s | Error=%v",
			msgType, requestID, err)
	} else {
		logger.Debugf("[WebSocket] ✅ 发送消息成功 | Type=%s | RequestID=%s", msgType, requestID)
	}
}

// sendSuccessResponse 发送成功响应消息到前台
func sendSuccessResponse(wsClient *FrontWSClient, userID int64, requestID string, msgType string, data map[string]interface{}) {
	msg := WSMessage{
		Type:      msgType,
		RequestID: requestID,
		UserID:    userID,
		Data:      data,
	}

	if err := wsClient.SendMessage(msg); err != nil {
		logger.Errorf("[WebSocket] ❌ 发送消息失败 | Type=%s | RequestID=%s | Error=%v",
			msgType, requestID, err)
	} else {
		logger.Debugf("[WebSocket] ✅ 发送消息成功 | Type=%s | RequestID=%s", msgType, requestID)
	}
}

// sendErrorResponse 发送错误响应消息到前台
func sendErrorResponse(wsClient *FrontWSClient, userID int64, requestID string, errorMsg string) {
	msg := WSMessage{
		Type:      "error",
		RequestID: requestID,
		UserID:    userID,
		Data: map[string]interface{}{
			"error": errorMsg,
		},
	}

	if err := wsClient.SendMessage(msg); err != nil {
		logger.Errorf("[WebSocket] ❌ 发送错误消息失败 | RequestID=%s | Error=%v",
			requestID, err)
	}
}

// sendTaskProgress 发送任务进度更新（供TaskScheduler调用）
func SendTaskProgress(wsClient *FrontWSClient, userID int64, requestID string, progress int, status string) {
	data := map[string]interface{}{
		"task_id":  requestID,
		"progress": progress,
		"status":   status,
	}
	sendSuccessResponse(wsClient, userID, requestID, "task_progress", data)
}

// SendTaskComplete 发送任务完成通知（供TaskScheduler调用）
func SendTaskComplete(wsClient *FrontWSClient, userID int64, requestID string, promptID string, status string, images interface{}, errorMsg string) {
	data := map[string]interface{}{
		"task_id":   requestID,
		"prompt_id": promptID,
		"status":    status,
	}

	if images != nil {
		data["images"] = images
	}

	if errorMsg != "" {
		data["error"] = errorMsg
	}

	msgType := "task_complete"
	if status == "failed" {
		msgType = "task_failed"
	}

	sendSuccessResponse(wsClient, userID, requestID, msgType, data)
}

// SendImageReady 发送图像就绪通知（供TaskScheduler调用）
func SendImageReady(wsClient *FrontWSClient, userID int64, requestID string, fileInfo interface{}) {
	data := map[string]interface{}{
		"task_id": requestID,
		"file":    fileInfo,
	}
	sendSuccessResponse(wsClient, userID, requestID, "image_ready", data)
}

// LogWebSocketConnection 记录WebSocket连接状态
func LogWebSocketConnection(wsClient *FrontWSClient, connected bool) {
	if connected {
		logger.Infof("[WebSocket] ✅ WebSocket已连接 | 消息处理器已就绪")
	} else {
		logger.Warnf("[WebSocket] ❌ WebSocket未连接 | 消息将无法发送")
	}
}

// ValidateMessageData 验证消息数据的完整性
func ValidateMessageData(data map[string]interface{}, requiredFields ...string) error {
	for _, field := range requiredFields {
		if _, ok := data[field]; !ok {
			return fmt.Errorf("缺少必需字段: %s", field)
		}
	}
	return nil
}

// registerWorkflowListQueryHandler 注册工作流列表查询处理器
// 接收前台发送的工作流列表查询请求，返回工作流列表
func registerWorkflowListQueryHandler(wsClient *FrontWSClient) {
	wsClient.RegisterHandler("workflow_list_query", func(data map[string]interface{}) {
		logger.Infof("[WebSocket] 📥 收到工作流列表查询请求: %+v", data)

		// 解析用户ID（可选）
		var userID int64 = 0
		if userIDFloat, ok := data["user_id"].(float64); ok {
			userID = int64(userIDFloat)
		} else {
			logger.Warnf("[WebSocket] 工作流列表查询: user_id 字段缺失或类型错误")
		}

		// 提取请求ID（可选）
		var requestID string = ""
		if reqID, ok := data["request_id"].(string); ok {
			requestID = reqID
		} else {
			logger.Warnf("[WebSocket] 工作流列表查询: request_id 字段缺失或类型错误")
		}

		// 解析分页参数
		page := 1
		if pageFloat, ok := data["page"].(float64); ok {
			page = int(pageFloat)
		} else if pageStr, ok := data["page"].(string); ok {
			if p, err := strconv.Atoi(pageStr); err == nil {
				page = p
			}
		}
		if page < 1 {
			page = 1
		}

		pageSize := 20
		if pageSizeFloat, ok := data["page_size"].(float64); ok {
			pageSize = int(pageSizeFloat)
		} else if pageSizeStr, ok := data["page_size"].(string); ok {
			if ps, err := strconv.Atoi(pageSizeStr); err == nil {
				pageSize = ps
			}
		}
		if pageSize < 1 || pageSize > 100 {
			pageSize = 20
		}

		// 解析筛选参数
		category := ""
		if cat, ok := data["category"].(string); ok {
			category = cat
		}

		search := ""
		if s, ok := data["search"].(string); ok {
			search = s
		}

		logger.Infof("[WebSocket] 📋 查询工作流列表 | UserID=%d | Page=%d | PageSize=%d | Category=%s | Search=%s",
			userID, page, pageSize, category, search)

		// 从数据库查询工作流列表
		workflows, total, err := models.GetWorkflowList(page, pageSize, category, search)
		if err != nil {
			logger.Errorf("[WebSocket] 查询工作流列表失败: Error=%v", err)
			sendErrorResponse(wsClient, userID, requestID, "查询工作流列表失败: "+err.Error())
			return
		}

		// 构建响应数据
		responseData := map[string]interface{}{
			"workflows": workflows,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		}

		// 发送成功响应
		sendSuccessResponse(wsClient, userID, requestID, "workflow_list_response", responseData)
		logger.Debugf("[WebSocket] ✅ 工作流列表查询成功: Total=%d, Returned=%d", total, len(workflows))
	})

	logger.Debugf("[WebSocket] 已注册处理器: workflow_list_query")
}
