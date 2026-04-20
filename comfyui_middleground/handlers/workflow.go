package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"comfyui_middleground/models"
	"comfyui_middleground/scheduler"
)

// SetTaskSchedulerForWorkflow 设置任务调度器（供workflow处理器使用）
func SetTaskSchedulerForWorkflow(ts *scheduler.TaskScheduler) {
	taskScheduler = ts
}

// 获取工作流列表
func GetWorkflowList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	category := c.Query("category")
	search := c.Query("search")

	workflows, total, err := models.GetWorkflowList(page, pageSize, category, search)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "获取工作流列表失败",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"workflows": workflows,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}

// 获取工作流详情
func GetWorkflowDetail(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "无效的工作流ID",
		})
		return
	}

	workflow, err := models.GetWorkflowByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": "工作流不存在",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    workflow,
	})
}

// 创建工作流
func CreateWorkflow(c *gin.Context) {
	var req struct {
		Name              string `json:"name" binding:"required"`
		Description       string `json:"description"`
		WorkflowJSON      string `json:"workflow_json" binding:"required"`
		Category          string `json:"category"`
		Tags              string `json:"tags"`
		PositiveNodeID    string `json:"positive_node_id"`
		PositiveFieldName string `json:"positive_field_name"`
		NegativeNodeID    string `json:"negative_node_id"`
		NegativeFieldName string `json:"negative_field_name"`
		IsDefault         bool   `json:"is_default"`
		IsPublic          bool   `json:"is_public"`
		CreatedBy         string `json:"created_by"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误",
			"error":   err.Error(),
		})
		return
	}

	// 验证工作流JSON格式
	var workflowMap map[string]interface{}
	if err := json.Unmarshal([]byte(req.WorkflowJSON), &workflowMap); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "工作流JSON格式错误",
			"error":   err.Error(),
		})
		return
	}

	workflow := &models.Workflow{
		Name:              req.Name,
		Description:       req.Description,
		WorkflowJSON:      req.WorkflowJSON,
		Category:          req.Category,
		Tags:              req.Tags,
		PositiveNodeID:    req.PositiveNodeID,
		PositiveFieldName: req.PositiveFieldName,
		NegativeNodeID:    req.NegativeNodeID,
		NegativeFieldName: req.NegativeFieldName,
		IsDefault:         req.IsDefault,
		IsPublic:          req.IsPublic,
		CreatedBy:         req.CreatedBy,
		UsageCount:        0,
	}

	if err := models.CreateWorkflow(workflow); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "创建工作流失败",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    workflow,
	})
}

// 更新工作流
func UpdateWorkflow(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "无效的工作流ID",
		})
		return
	}

	var req struct {
		Name              string `json:"name"`
		Description       string `json:"description"`
		WorkflowJSON      string `json:"workflow_json"`
		Category          string `json:"category"`
		Tags              string `json:"tags"`
		PositiveNodeID    string `json:"positive_node_id"`
		PositiveFieldName string `json:"positive_field_name"`
		NegativeNodeID    string `json:"negative_node_id"`
		NegativeFieldName string `json:"negative_field_name"`
		IsDefault         bool   `json:"is_default"`
		IsPublic          bool   `json:"is_public"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误",
			"error":   err.Error(),
		})
		return
	}

	// 获取现有工作流
	existing, err := models.GetWorkflowByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": "工作流不存在",
		})
		return
	}

	// 更新字段
	if req.Name != "" {
		existing.Name = req.Name
	}
	if req.Description != "" {
		existing.Description = req.Description
	}
	if req.WorkflowJSON != "" {
		// 验证JSON格式
		var workflowMap map[string]interface{}
		if err := json.Unmarshal([]byte(req.WorkflowJSON), &workflowMap); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": "工作流JSON格式错误",
				"error":   err.Error(),
			})
			return
		}
		existing.WorkflowJSON = req.WorkflowJSON
	}
	if req.Category != "" {
		existing.Category = req.Category
	}
	if req.Tags != "" {
		existing.Tags = req.Tags
	}
	if req.PositiveNodeID != "" {
		existing.PositiveNodeID = req.PositiveNodeID
	}
	if req.PositiveFieldName != "" {
		existing.PositiveFieldName = req.PositiveFieldName
	}
	if req.NegativeNodeID != "" {
		existing.NegativeNodeID = req.NegativeNodeID
	}
	if req.NegativeFieldName != "" {
		existing.NegativeFieldName = req.NegativeFieldName
	}
	existing.IsDefault = req.IsDefault
	existing.IsPublic = req.IsPublic

	if err := models.UpdateWorkflow(id, existing); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "更新工作流失败",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    existing,
	})
}

// 删除工作流
func DeleteWorkflow(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "无效的工作流ID",
		})
		return
	}

	if err := models.DeleteWorkflow(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "删除工作流失败",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
	})
}

// 执行工作流
func ExecuteWorkflow(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "无效的工作流ID",
		})
		return
	}

	// 获取工作流
	workflow, err := models.GetWorkflowByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": "工作流不存在",
		})
		return
	}

	// 解析工作流JSON
	var workflowMap map[string]interface{}
	if err := json.Unmarshal([]byte(workflow.WorkflowJSON), &workflowMap); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "工作流JSON格式错误",
			"error":   err.Error(),
		})
		return
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
	requestID := fmt.Sprintf("workflow_%d_%d", id, time.Now().Unix())

	// 创建任务请求
	taskReq := &scheduler.TaskRequest{
		RequestID: requestID,
		UserID:    0, // 中台执行，用户ID为0
		DeviceID:  "middle_server",
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

	// 增加使用次数
	models.IncrementWorkflowUsage(id)

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"workflow_id": id,
			"request_id": requestID,
			"status":      "pending",
		},
	})
}

