package handlers

import (
	"encoding/json"
	"fmt"
	"comfyui_middleground/logger"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"comfyui_middleground/comfyui"
	"comfyui_middleground/models"
	"comfyui_middleground/scheduler"
)

var taskScheduler *scheduler.TaskScheduler

func SetTaskScheduler(ts *scheduler.TaskScheduler) {
	taskScheduler = ts
}

func SubmitTestRequest(c *gin.Context) {
	var req struct {
		WorkflowID     *int64                 `json:"workflow_id"`     // 可选：工作流ID
		PromptText     string                 `json:"prompt_text"`     // 可选：正向提示词
		NegativePrompt string                 `json:"negative_prompt"` // 可选：负向提示词
		Workflow       map[string]interface{} `json:"workflow"`        // 工作流JSON（如果提供了workflow_id则从数据库加载）
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误",
			"error":   err.Error(),
		})
		return
	}

	var workflowMap map[string]interface{}
	var workflowModel *models.Workflow

	// 如果提供了workflow_id，从数据库加载工作流
	if req.WorkflowID != nil && *req.WorkflowID > 0 {
		var err error
		workflowModel, err = models.GetWorkflowByID(*req.WorkflowID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"code":    404,
				"message": "工作流不存在",
			})
			return
		}

		// 解析工作流JSON
		if err := json.Unmarshal([]byte(workflowModel.WorkflowJSON), &workflowMap); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": "工作流JSON格式错误",
				"error":   err.Error(),
			})
			return
		}

		// 如果输入了提示词，根据配置的节点和字段替换工作流中的提示词
		// 如果没有配置，自动查找 positive/negative 字段
		if req.PromptText != "" || req.NegativePrompt != "" {
			// 如果配置了节点和字段，使用配置
			if workflowModel.PositiveNodeID != "" || workflowModel.NegativeNodeID != "" {
				workflowMap = replacePromptsInWorkflowWithConfig(
					workflowMap,
					req.PromptText,
					req.NegativePrompt,
					workflowModel.PositiveNodeID,
					workflowModel.PositiveFieldName,
					workflowModel.NegativeNodeID,
					workflowModel.NegativeFieldName,
				)
			} else {
				// 如果没有配置，自动查找并替换
				workflowMap = replacePromptsInWorkflowAuto(workflowMap, req.PromptText, req.NegativePrompt)
			}
		}

		// 增加使用次数
		models.IncrementWorkflowUsage(*req.WorkflowID)
	} else if req.Workflow != nil {
		// 直接使用提供的工作流
		workflowMap = req.Workflow
	} else {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "必须提供workflow_id或workflow",
		})
		return
	}

	// 提取提示词（用于记录）
	promptText := req.PromptText
	negativePrompt := req.NegativePrompt
	if promptText == "" || negativePrompt == "" {
		// 从工作流中提取提示词
		extracted := extractPromptsFromWorkflow(workflowMap)
		if promptText == "" {
			promptText = extracted.Positive
		}
		if negativePrompt == "" {
			negativePrompt = extracted.Negative
		}
	}

	// 转换工作流格式
	workflow := make(comfyui.Workflow)
	for k, v := range workflowMap {
		if nodeMap, ok := v.(map[string]interface{}); ok {
			node := comfyui.Node{
				ClassType: nodeMap["class_type"].(string),
				Inputs:    nodeMap["inputs"].(map[string]interface{}),
			}
			workflow[k] = node
		}
	}

	// 通过任务调度器提交任务
	if taskScheduler == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "任务调度器未初始化",
		})
		return
	}

	// 生成请求ID
	requestID := fmt.Sprintf("test_%d", time.Now().Unix())

	// 创建任务请求
	taskReq := &scheduler.TaskRequest{
		RequestID: requestID,
		UserID:    0, // 测试用户
		DeviceID:  "test_device",
		Workflow:  workflowMap,
	}

	// 提交任务
	if err := taskScheduler.SubmitTask(taskReq); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "提交任务失败",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"request_id": requestID,
			"status":     "pending",
		},
	})
}

// 从工作流中提取提示词
func extractPromptsFromWorkflow(workflow map[string]interface{}) struct {
	Positive string
	Negative string
} {
	var positivePrompt, negativePrompt string
	var positiveFound bool

	// 查找CLIPTextEncode节点
	for _, v := range workflow {
		if nodeMap, ok := v.(map[string]interface{}); ok {
			if classType, ok := nodeMap["class_type"].(string); ok && classType == "CLIPTextEncode" {
				if inputs, ok := nodeMap["inputs"].(map[string]interface{}); ok {
					if text, ok := inputs["text"].(string); ok {
						if !positiveFound {
							positivePrompt = text
							positiveFound = true
						} else {
							negativePrompt = text
							break
						}
					}
				}
			}
		}
	}

	return struct {
		Positive string
		Negative string
	}{Positive: positivePrompt, Negative: negativePrompt}
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
				logger.Debugf("替换正向提示词: 节点ID=%s, 字段名=%s", positiveNodeID, fieldName)
			} else {
				logger.Debugf("警告: 节点 %s 没有 inputs 字段", positiveNodeID)
			}
		} else {
			logger.Debugf("警告: 未找到节点 %s", positiveNodeID)
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
				logger.Debugf("替换负向提示词: 节点ID=%s, 字段名=%s", negativeNodeID, fieldName)
			} else {
				logger.Debugf("警告: 节点 %s 没有 inputs 字段", negativeNodeID)
			}
		} else {
			logger.Debugf("警告: 未找到节点 %s", negativeNodeID)
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
				logger.Debugf("自动替换提示词: 字段名=%s", fieldName)
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
			// 如果没找到 positive，查找第一个包含较长字符串的字段（可能是提示词）
			logger.Debugf("未找到 positive 字段，尝试查找其他提示词字段")
			// 查找 CLIPTextEncode 节点的 text 字段
			for _, v := range workflowCopy {
				if nodeMap, ok := v.(map[string]interface{}); ok {
					if classType, ok := nodeMap["class_type"].(string); ok && classType == "CLIPTextEncode" {
						if inputs, ok := nodeMap["inputs"].(map[string]interface{}); ok {
							if text, ok := inputs["text"].(string); ok && text != "" {
								inputs["text"] = positivePrompt
								logger.Debugf("自动替换正向提示词: CLIPTextEncode节点的text字段")
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
			logger.Debugf("未找到 negative 字段，尝试查找其他提示词字段")
			foundFirst := false
			for _, v := range workflowCopy {
				if nodeMap, ok := v.(map[string]interface{}); ok {
					if classType, ok := nodeMap["class_type"].(string); ok && classType == "CLIPTextEncode" {
						if inputs, ok := nodeMap["inputs"].(map[string]interface{}); ok {
							if text, ok := inputs["text"].(string); ok && text != "" {
								if foundFirst {
									inputs["text"] = negativePrompt
									logger.Debugf("自动替换负向提示词: CLIPTextEncode节点的text字段")
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

func GetWorkflowTemplates(c *gin.Context) {
	// 返回工作流模板列表
	templates := []gin.H{
		{
			"id":   "default",
			"name": "默认模板",
		},
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    templates,
	})
}

