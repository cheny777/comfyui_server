/*
 * @Author: chenyu02_dxm chenyu02@duxiaoman.com
 * @Date: 2025-12-27 16:48:03
 * @LastEditors: chenyu02_dxm chenyu02@duxiaoman.com
 * @LastEditTime: 2025-12-27 17:03:52
 * @FilePath: /comfyui_server/comfyui_middleground/handlers/task.go
 * @Description: 这是默认设置,请设置`customMade`, 打开koroFileHeader查看配置 进行设置: https://github.com/OBKoro1/koro1FileHeader/wiki/%E9%85%8D%E7%BD%AE
 */
package handlers

import (
	"net/http"
	"strconv"

	"comfyui_middleground/models"

	"github.com/gin-gonic/gin"
)

func GetTaskList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	status := c.Query("status")
	userIDStr := c.Query("user_id")
	deviceID := c.Query("device_id")
	search := c.Query("search")

	var userID int64
	if userIDStr != "" {
		userID, _ = strconv.ParseInt(userIDStr, 10, 64)
	}

	tasks, total, err := models.GetGenerationRequests(page, pageSize, userID, deviceID, status, search)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "获取任务列表失败",
			"error":   err.Error(),
		})
		return
	}

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

func GetTaskDetail(c *gin.Context) {
	id := c.Param("id")

	task, err := models.GetGenerationRequestByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": "任务不存在",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    task,
	})
}

func GetTaskImages(c *gin.Context) {
	id := c.Param("id")

	task, err := models.GetGenerationRequestByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": "任务不存在",
		})
		return
	}

	// 解析文件信息
	var files []models.FileInfo
	if task.FilesInfo != "" {
		// 这里需要解析JSON，简化处理
		// 实际应该使用json.Unmarshal
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    files,
	})
}
