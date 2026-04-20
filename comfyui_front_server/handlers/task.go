package handlers

import (
	"comfyui_front_server/logger"
	"comfyui_front_server/models"
	"comfyui_front_server/websocket"
	"encoding/json"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// SendTaskToMiddleFunc 发送任务到中台的函数类型
type SendTaskToMiddleFunc func(taskID string, userID int64, deviceID string, workflow map[string]interface{}) error

var sendTaskToMiddleFunc SendTaskToMiddleFunc

// SetSendTaskToMiddleFunc 设置发送任务到中台的函数
func SetSendTaskToMiddleFunc(fn SendTaskToMiddleFunc) {
	sendTaskToMiddleFunc = fn
}

// CreateTask 创建任务
func CreateTask(c *gin.Context) {
	logger.Infof("[API] 📥 收到创建任务请求")

	userID := c.MustGet("user_id").(int64)
	deviceID, _ := c.Get("device_id")

	logger.Debugf("[API] 请求参数: UserID=%d | DeviceID=%v", userID, deviceID)

	var req struct {
		WorkflowID     *int64                 `json:"workflow_id"`     // 可选：工作流ID
		PromptText     string                 `json:"prompt_text"`     // 可选：正向提示词
		NegativePrompt string                 `json:"negative_prompt"` // 可选：负向提示词
		Workflow       map[string]interface{} `json:"workflow"`        // 工作流JSON（如果提供了workflow_id则从数据库加载）
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Errorf("[API] ❌ 参数绑定失败: UserID=%d | Error=%v", userID, err)
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误", "error": err.Error()})
		return
	}

	var workflowMap map[string]interface{}
	var workflowConfig map[string]interface{} // 工作流配置信息（节点ID、字段名等）

	// 如果提供了workflow_id，从工作流列表查询工作流
	if req.WorkflowID != nil && *req.WorkflowID > 0 {
		logger.Debugf("[API] 查询工作流: WorkflowID=%d", *req.WorkflowID)

		// 通过WebSocket查询工作流列表，查找指定ID的工作流
		result, err := websocket.QueryWorkflowList(userID, 1, 100, "", "")
		if err != nil {
			logger.Errorf("[API] ❌ 查询工作流列表失败: WorkflowID=%d | Error=%v", *req.WorkflowID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "查询工作流失败", "error": err.Error()})
			return
		}

		// 从结果中查找指定ID的工作流
		var foundWorkflow map[string]interface{}
		if workflows, ok := result["workflows"].([]interface{}); ok {
			for _, wf := range workflows {
				if wfMap, ok := wf.(map[string]interface{}); ok {
					if id, ok := wfMap["id"].(float64); ok && int64(id) == *req.WorkflowID {
						foundWorkflow = wfMap
						workflowConfig = wfMap // 保存工作流配置信息
						break
					}
				}
			}
		}

		if foundWorkflow == nil {
			logger.Errorf("[API] ❌ 工作流不存在: WorkflowID=%d", *req.WorkflowID)
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "工作流不存在"})
			return
		}

		// 解析工作流JSON
		var workflowJSONStr string
		if wfJSON, ok := foundWorkflow["workflow_json"].(string); ok {
			workflowJSONStr = wfJSON
		} else if wfJSON, ok := foundWorkflow["workflow_json"].(map[string]interface{}); ok {
			// 如果已经是map，直接使用
			workflowMap = wfJSON
		} else {
			logger.Errorf("[API] ❌ 工作流JSON格式错误: WorkflowID=%d", *req.WorkflowID)
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "工作流JSON格式错误"})
			return
		}

		if workflowMap == nil && workflowJSONStr != "" {
			if err := json.Unmarshal([]byte(workflowJSONStr), &workflowMap); err != nil {
				logger.Errorf("[API] ❌ 解析工作流JSON失败: WorkflowID=%d | Error=%v", *req.WorkflowID, err)
				c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "工作流JSON格式错误", "error": err.Error()})
				return
			}
		}

		// 如果输入了提示词，根据配置的节点和字段替换工作流中的提示词
		if req.PromptText != "" || req.NegativePrompt != "" {
			// 如果配置了节点和字段，使用配置
			positiveNodeID, _ := workflowConfig["positive_node_id"].(string)
			positiveFieldName, _ := workflowConfig["positive_field_name"].(string)
			negativeNodeID, _ := workflowConfig["negative_node_id"].(string)
			negativeFieldName, _ := workflowConfig["negative_field_name"].(string)

			if positiveNodeID != "" || negativeNodeID != "" {
				workflowMap = replacePromptsInWorkflowWithConfig(
					workflowMap,
					req.PromptText,
					req.NegativePrompt,
					positiveNodeID,
					positiveFieldName,
					negativeNodeID,
					negativeFieldName,
				)
			} else {
				// 如果没有配置，自动查找并替换
				workflowMap = replacePromptsInWorkflowAuto(workflowMap, req.PromptText, req.NegativePrompt)
			}
		}
	} else if req.Workflow != nil {
		// 直接使用提供的工作流
		workflowMap = req.Workflow

		// 如果提供了提示词，自动替换
		if req.PromptText != "" || req.NegativePrompt != "" {
			workflowMap = replacePromptsInWorkflowAuto(workflowMap, req.PromptText, req.NegativePrompt)
		}
	} else {
		logger.Errorf("[API] ❌ 必须提供workflow_id或workflow: UserID=%d", userID)
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "必须提供workflow_id或workflow"})
		return
	}

	// 检查工作流是否为空
	if len(workflowMap) == 0 {
		logger.Errorf("[API] ❌ 工作流为空: UserID=%d", userID)
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "工作流不能为空"})
		return
	}

	logger.Debugf("[API] 工作流节点数: %d", len(workflowMap))

	// 更新工作流中的 seed，生成随机数
	workflowMap = updateSeedInWorkflow(workflowMap)
	logger.Debugf("[API] ✅ 已更新工作流中的 seed 为随机数")

	// 生成任务ID
	taskID := "task_" + uuid.New().String()
	logger.Infof("[API] 📝 生成任务ID: %s | UserID=%d", taskID, userID)

	// 创建任务记录
	task, err := models.CreateTask(userID, taskID, workflowMap)
	if err != nil {
		logger.Errorf("[API] ❌ 创建任务记录失败: TaskID=%s | UserID=%d | Error=%v", taskID, userID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "创建任务失败", "error": err.Error()})
		return
	}
	logger.Infof("[API] ✅ 任务记录已创建: TaskID=%s | Status=%s", taskID, task.Status)

	// 通过WebSocket发送任务到中台
	deviceIDStr := ""
	if deviceID != nil {
		deviceIDStr = deviceID.(string)
		logger.Debugf("[API] DeviceID: %s", deviceIDStr)
	} else {
		logger.Warnf("[API] ⚠️  DeviceID为空: UserID=%d", userID)
	}

	if sendTaskToMiddleFunc == nil {
		logger.Errorf("[API] ❌ WebSocket服务未初始化: TaskID=%s | UserID=%d", taskID, userID)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "WebSocket服务未初始化"})
		return
	}

	// 记录发送到中台的结构信息
	workflowJSON, _ := json.Marshal(workflowMap)
	logger.Infof("[API] 📤 发送任务到中台 | TaskID=%s | UserID=%d | DeviceID=%s", taskID, userID, deviceIDStr)
	logger.Debugf("[API] 📤 发送数据结构: TaskID=%s | UserID=%d | DeviceID=%s | PromptText长度=%d | NegativePrompt长度=%d | Workflow节点数=%d",
		taskID, userID, deviceIDStr, len(req.PromptText), len(req.NegativePrompt), len(workflowMap))
	logger.Debugf("[API] 📤 工作流JSON长度: TaskID=%s | JSON长度=%d字节", taskID, len(workflowJSON))

	// 构建发送到中台的完整数据结构（用于日志）
	taskData := map[string]interface{}{
		"task_id":         taskID,
		"user_id":         userID,
		"device_id":       deviceIDStr,
		"workflow":        workflowMap,
		"prompt_text":     req.PromptText,
		"negative_prompt": req.NegativePrompt,
	}
	taskDataJSON, _ := json.Marshal(taskData)
	logger.Debugf("[API] 📤 完整任务数据结构: TaskID=%s | 数据JSON长度=%d字节", taskID, len(taskDataJSON))

	err = sendTaskToMiddleFunc(taskID, userID, deviceIDStr, workflowMap)
	if err != nil {
		logger.Errorf("[API] ❌ 发送任务到中台失败: TaskID=%s | UserID=%d | Error=%v", taskID, userID, err)
		// WebSocket发送失败，更新任务状态为失败
		updateErr := models.UpdateTask(taskID, map[string]interface{}{
			"status": "failed",
			"error":  "发送任务到中台失败: " + err.Error(),
		})
		if updateErr != nil {
			logger.Errorf("[API] ❌ 更新任务状态失败: TaskID=%s | Error=%v", taskID, updateErr)
		} else {
			logger.Infof("[API] ✅ 任务状态已更新为失败: TaskID=%s", taskID)
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "发送任务失败", "error": err.Error()})
		return
	}

	logger.Infof("[API] ✅ 任务创建成功: TaskID=%s | UserID=%d | Status=%s", taskID, userID, task.Status)
	logger.Debugf("[API] ✅ 任务创建完成: TaskID=%s | UserID=%d | Status=%s | PromptText长度=%d | NegativePrompt长度=%d",
		taskID, userID, task.Status, len(req.PromptText), len(req.NegativePrompt))

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"task_id": task.ID,
			"status":  task.Status,
		},
	})
}

// 替换工作流中的提示词（使用配置的节点和字段）
func replacePromptsInWorkflowWithConfig(
	workflow map[string]interface{},
	positivePrompt, negativePrompt string,
	positiveNodeID, positiveFieldName, negativeNodeID, negativeFieldName string,
) map[string]interface{} {
	// 深拷贝工作流
	workflowCopy := make(map[string]interface{})
	workflowJSON, _ := json.Marshal(workflow)
	json.Unmarshal(workflowJSON, &workflowCopy)

	// 替换正向提示词
	if positivePrompt != "" && positiveNodeID != "" {
		if node, ok := workflowCopy[positiveNodeID].(map[string]interface{}); ok {
			if inputs, ok := node["inputs"].(map[string]interface{}); ok {
				fieldName := positiveFieldName
				if fieldName == "" {
					fieldName = "text" // 默认字段名
				}
				inputs[fieldName] = positivePrompt
				logger.Debugf("[API] 替换正向提示词: 节点ID=%s, 字段名=%s", positiveNodeID, fieldName)
			} else {
				logger.Debugf("[API] 警告: 节点 %s 没有 inputs 字段", positiveNodeID)
			}
		} else {
			logger.Debugf("[API] 警告: 未找到节点 %s", positiveNodeID)
		}
	}

	// 替换负向提示词
	if negativePrompt != "" && negativeNodeID != "" {
		if node, ok := workflowCopy[negativeNodeID].(map[string]interface{}); ok {
			if inputs, ok := node["inputs"].(map[string]interface{}); ok {
				fieldName := negativeFieldName
				if fieldName == "" {
					fieldName = "text" // 默认字段名
				}
				inputs[fieldName] = negativePrompt
				logger.Debugf("[API] 替换负向提示词: 节点ID=%s, 字段名=%s", negativeNodeID, fieldName)
			} else {
				logger.Debugf("[API] 警告: 节点 %s 没有 inputs 字段", negativeNodeID)
			}
		} else {
			logger.Debugf("[API] 警告: 未找到节点 %s", negativeNodeID)
		}
	}

	return workflowCopy
}

// 自动查找并替换工作流中的提示词（递归查找 positive/negative 字段）
func replacePromptsInWorkflowAuto(workflow map[string]interface{}, positivePrompt, negativePrompt string) map[string]interface{} {
	// 深拷贝工作流
	workflowCopy := make(map[string]interface{})
	workflowJSON, _ := json.Marshal(workflow)
	json.Unmarshal(workflowJSON, &workflowCopy)

	// 递归查找并替换 positive 和 negative 字段
	var replaceField func(obj interface{}, fieldName string, value string) bool
	replaceField = func(obj interface{}, fieldName string, value string) bool {
		switch v := obj.(type) {
		case map[string]interface{}:
			// 如果找到目标字段，直接替换
			if val, ok := v[fieldName].(string); ok && val != "" {
				v[fieldName] = value
				logger.Debugf("[API] 自动替换提示词: 字段名=%s", fieldName)
				return true
			}
			// 递归查找所有子对象
			for _, val := range v {
				if replaceField(val, fieldName, value) {
					return true
				}
			}
		case []interface{}:
			// 如果是数组，递归查找每个元素
			for _, item := range v {
				if replaceField(item, fieldName, value) {
					return true
				}
			}
		}
		return false
	}

	// 替换正向提示词
	if positivePrompt != "" {
		// 优先查找 positive 字段
		if !replaceField(workflowCopy, "positive", positivePrompt) {
			// 如果没找到 positive，查找 CLIPTextEncode 节点的 text 字段
			logger.Debugf("[API] 未找到 positive 字段，尝试查找 CLIPTextEncode 节点")
			for _, v := range workflowCopy {
				if nodeMap, ok := v.(map[string]interface{}); ok {
					if classType, ok := nodeMap["class_type"].(string); ok && classType == "CLIPTextEncode" {
						if inputs, ok := nodeMap["inputs"].(map[string]interface{}); ok {
							if text, ok := inputs["text"].(string); ok && text != "" {
								inputs["text"] = positivePrompt
								logger.Debugf("[API] 自动替换正向提示词: CLIPTextEncode节点的text字段")
								break
							}
						}
					}
				}
			}
		}
	}

	// 替换负向提示词
	if negativePrompt != "" {
		// 优先查找 negative 字段
		if !replaceField(workflowCopy, "negative", negativePrompt) {
			// 如果没找到 negative，查找第二个 CLIPTextEncode 节点
			logger.Debugf("[API] 未找到 negative 字段，尝试查找第二个 CLIPTextEncode 节点")
			foundFirst := false
			for _, v := range workflowCopy {
				if nodeMap, ok := v.(map[string]interface{}); ok {
					if classType, ok := nodeMap["class_type"].(string); ok && classType == "CLIPTextEncode" {
						if inputs, ok := nodeMap["inputs"].(map[string]interface{}); ok {
							if text, ok := inputs["text"].(string); ok && text != "" {
								if foundFirst {
									inputs["text"] = negativePrompt
									logger.Debugf("[API] 自动替换负向提示词: CLIPTextEncode节点的text字段")
									break
								}
								foundFirst = true
							}
						}
					}
				}
			}
		}
	}

	return workflowCopy
}

// 辅助函数：从map中获取字符串值（支持多个键名）
func getStringFromMap(m map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if v, ok := m[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// 辅助函数：从map中获取int64值（支持多个键名）
func getInt64FromMap(m map[string]interface{}, keys ...string) int64 {
	for _, key := range keys {
		if v, ok := m[key].(float64); ok {
			return int64(v)
		} else if v, ok := m[key].(int64); ok {
			return v
		} else if v, ok := m[key].(int); ok {
			return int64(v)
		}
	}
	return 0
}

// addFileToTask 添加文件记录到任务（内部函数，避免循环导入）
func addFileToTask(taskID string, userID int64, fileInfo map[string]interface{}) error {
	file := &models.File{
		TaskID:           taskID,
		UserID:           userID,
		Filename:         getStringFromMap(fileInfo, "filename"),
		OriginalFilename: getStringFromMap(fileInfo, "original_filename"),
		FilePath:         getStringFromMap(fileInfo, "file_path"),
		FileSize:         getInt64FromMap(fileInfo, "file_size"),
		FileType:         getStringFromMap(fileInfo, "file_type"),
		URL:              getStringFromMap(fileInfo, "url"),
	}

	return models.CreateFile(file)
}

// GetTaskList 获取任务列表
func GetTaskList(c *gin.Context) {
	userID := c.MustGet("user_id").(int64)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	status := c.Query("status")

	tasks, total, err := models.GetTasksByUserID(userID, page, pageSize, status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "获取任务列表失败", "error": err.Error()})
		return
	}

	// 文件列表已通过Preload预加载，无需额外查询

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"tasks":     tasks,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}

// GetTaskDetail 获取任务详情
// GetTaskDetail 获取任务详情
func GetTaskDetail(c *gin.Context) {
	userID := c.MustGet("user_id").(int64)
	taskID := c.Param("id")

	// 先从本地数据库查询
	task, err := models.GetTaskByIDAndUserID(taskID, userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "任务不存在"})
		return
	}

	// 如果任务正在运行中，尝试从中台查询最新状态
	// 已完成的任务如果本地有文件列表，就不需要查询中台了
	needQueryMiddle := false
	if task.Status == "pending" || task.Status == "running" {
		needQueryMiddle = true
	} else if task.Status == "completed" {
		// 已完成的任务：如果本地没有文件列表，才查询中台
		if len(task.Files) == 0 {
			needQueryMiddle = true
			logger.Debugf("[API] 任务已完成但无文件列表，查询中台: TaskID=%s", taskID)
		}
	}

	if needQueryMiddle {
		logger.Debugf("[API] 📤 查询中台任务状态: TaskID=%s | Status=%s", taskID, task.Status)
		// 通过WebSocket查询中台获取最新状态
		result, err := websocket.QueryTaskStatus(userID, taskID)
		if err != nil {
			logger.Warnf("[API] ⚠️ 查询中台任务状态失败: TaskID=%s | Error=%v | 使用本地数据", taskID, err)
			// 查询失败不影响返回，使用本地数据
		} else if result != nil {
			// 更新本地数据库
			if status, ok := result["status"].(string); ok {
				progress := 0
				if p, ok := result["progress"].(int); ok {
					progress = p
				} else if p, ok := result["progress"].(float64); ok {
					progress = int(p)
				}

				updates := map[string]interface{}{
					"status":   status,
					"progress": progress,
				}

				if promptID, ok := result["prompt_id"].(string); ok && promptID != "" {
					updates["prompt_id"] = promptID
				}

				if errMsg, ok := result["error"].(string); ok && errMsg != "" {
					updates["error"] = errMsg
				}

				if status == "completed" || status == "failed" {
					now := time.Now()
					updates["completed_at"] = &now
				}

				models.UpdateTask(taskID, updates)

				// 处理文件列表：同步中台的文件列表到前台数据库
				if files, ok := result["files"].([]interface{}); ok {
					logger.Debugf("[API] 收到文件列表: TaskID=%s, 文件数=%d", taskID, len(files))

					// 获取当前已存在的文件列表
					existingFiles, _ := models.GetFilesByTaskIDAndUserID(taskID, userID)
					existingFileMap := make(map[string]bool)
					for _, f := range existingFiles {
						existingFileMap[f.Filename] = true
					}

					// 同步文件列表
					for _, fileData := range files {
						if fileMap, ok := fileData.(map[string]interface{}); ok {
							filename := ""
							if fn, ok := fileMap["filename"].(string); ok {
								filename = fn
							} else if fn, ok := fileMap["Filename"].(string); ok {
								filename = fn
							}

							// 如果文件不存在，创建文件记录
							if filename != "" && !existingFileMap[filename] {
								logger.Debugf("[API] 创建文件记录: TaskID=%s, Filename=%s", taskID, filename)

								// 提取文件信息
								fileInfo := map[string]interface{}{
									"filename":          filename,
									"original_filename": getStringFromMap(fileMap, "original_filename", "OriginalFilename"),
									"file_path":         getStringFromMap(fileMap, "file_path", "FilePath"),
									"file_size":         getInt64FromMap(fileMap, "file_size", "FileSize"),
									"file_type":         getStringFromMap(fileMap, "file_type", "FileType"),
									"url":               getStringFromMap(fileMap, "url", "URL"),
								}

								// 创建文件记录
								if err := addFileToTask(taskID, userID, fileInfo); err != nil {
									logger.Errorf("[API] ❌ 创建文件记录失败: TaskID=%s, Filename=%s, Error=%v", taskID, filename, err)
								} else {
									logger.Debugf("[API] ✅ 文件记录已创建: TaskID=%s, Filename=%s", taskID, filename)
								}
							}
						}
					}
				}

				// 重新查询任务以获取最新数据（包括文件列表）
				task, _ = models.GetTaskByIDAndUserID(taskID, userID)
				logger.Debugf("[API] ✅ 已从中台同步任务状态: TaskID=%s | Status=%s | Files=%d",
					taskID, task.Status, len(task.Files))
			}
		}
	} else {
		logger.Debugf("[API] ✅ 使用本地任务数据: TaskID=%s | Status=%s | Files=%d",
			taskID, task.Status, len(task.Files))
	}

	// 文件列表已通过Preload预加载到task.Files中

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"id":           task.ID,
			"status":       task.Status,
			"progress":     task.Progress,
			"prompt_id":    task.PromptID,
			"workflow":     task.WorkflowObj,
			"error":        task.Error,
			"created_at":   task.CreatedAt,
			"updated_at":   task.UpdatedAt,
			"completed_at": task.CompletedAt,
			"files":        task.Files,
		},
	})
}

// DeleteTask 删除任务
func DeleteTask(c *gin.Context) {
	userID := c.MustGet("user_id").(int64)
	taskID := c.Param("id")

	// 删除关联的文件记录
	models.DeleteFilesByTaskID(taskID)

	// 删除任务
	if err := models.DeleteTask(taskID, userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "删除任务失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
	})
}

// UpdateTaskStatus 更新任务状态（由WebSocket消息处理器调用）
func UpdateTaskStatus(taskID string, status string, progress int, promptID string, errorMsg string) error {
	updates := map[string]interface{}{
		"status":   status,
		"progress": progress,
	}

	if promptID != "" {
		updates["prompt_id"] = promptID
	}

	if errorMsg != "" {
		updates["error"] = errorMsg
	}

	if status == "completed" || status == "failed" {
		now := time.Now()
		updates["completed_at"] = &now
	}

	return models.UpdateTask(taskID, updates)
}

// GetWorkflowList 获取工作流列表（通过WebSocket查询中台）
func GetWorkflowList(c *gin.Context) {
	userID := c.MustGet("user_id").(int64)

	// 解析查询参数
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	category := c.Query("category")
	search := c.Query("search")

	// 参数验证
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 通过WebSocket查询中台
	result, err := websocket.QueryWorkflowList(userID, page, pageSize, category, search)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "查询工作流列表失败",
			"error":   err.Error(),
		})
		return
	}

	// 返回结果
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    result,
	})
}

// updateSeedInWorkflow 更新工作流中的 seed 值，生成随机数
// 查找所有包含 seed 字段的节点（如 KSampler, KSamplerAdvanced 等），并更新为随机数
func updateSeedInWorkflow(workflow map[string]interface{}) map[string]interface{} {
	// 深拷贝工作流
	workflowCopy := make(map[string]interface{})
	workflowJSON, _ := json.Marshal(workflow)
	json.Unmarshal(workflowJSON, &workflowCopy)

	// 生成随机 seed（使用当前时间戳 + 随机数，确保唯一性）
	rand.Seed(time.Now().UnixNano())
	newSeed := rand.Int63n(4294967295) // 0 到 2^32-1 之间的随机数

	// 递归查找并更新 seed 字段
	var updateSeed func(obj interface{}) bool
	updateSeed = func(obj interface{}) bool {
		switch v := obj.(type) {
		case map[string]interface{}:
			// 检查是否是节点对象（有 inputs 字段）
			if inputs, ok := v["inputs"].(map[string]interface{}); ok {
				// 查找 seed 字段
				if _, exists := inputs["seed"]; exists {
					inputs["seed"] = newSeed
					logger.Debugf("[API] 更新 seed: 节点类型=%v, 新 seed=%d", v["class_type"], newSeed)
					return true
				}
			}
			// 递归查找所有子对象
			for _, val := range v {
				if updateSeed(val) {
					// 找到并更新了 seed，继续查找其他节点（可能有多个节点都有 seed）
				}
			}
		case []interface{}:
			// 如果是数组，递归查找每个元素
			for _, item := range v {
				updateSeed(item)
			}
		}
		return false
	}

	// 更新所有节点的 seed
	updateSeed(workflowCopy)

	logger.Debugf("[API] 工作流 seed 更新完成: 新 seed=%d", newSeed)
	return workflowCopy
}
