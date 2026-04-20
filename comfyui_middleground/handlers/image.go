package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"comfyui_middleground/config"
	"comfyui_middleground/database"
	"comfyui_middleground/models"

	"github.com/gin-gonic/gin"
)

// ImageItem 图像列表项
type ImageItem struct {
	ID          int64      `json:"id"`
	RequestID   string     `json:"request_id"`
	UserID      int64      `json:"user_id"`
	PromptText  string     `json:"prompt_text"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	models.FileInfo
}

func GetImageList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "24"))
	userIDStr := c.Query("user_id")
	taskID := c.Query("task_id")
	dateFrom := c.Query("date_from")
	dateTo := c.Query("date_to")

	var userID int64
	if userIDStr != "" {
		userID, _ = strconv.ParseInt(userIDStr, 10, 64)
	}

	// 查询所有已完成的任务（包含图像）
	query := database.DB.Model(&models.GenerationRequest{}).
		Where("status = ? AND files_info != '' AND files_info IS NOT NULL", "completed")

	if userID > 0 {
		query = query.Where("user_id = ?", userID)
	}
	if taskID != "" {
		query = query.Where("request_id LIKE ?", "%"+taskID+"%")
	}
	if dateFrom != "" {
		if t, err := time.Parse("2006-01-02", dateFrom); err == nil {
			query = query.Where("created_at >= ?", t)
		}
	}
	if dateTo != "" {
		if t, err := time.Parse("2006-01-02", dateTo); err == nil {
			// 设置为当天的23:59:59
			t = t.Add(24*time.Hour - time.Second)
			query = query.Where("created_at <= ?", t)
		}
	}

	// 获取总数
	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "查询失败",
			"error":   err.Error(),
		})
		return
	}

	// 分页查询
	offset := (page - 1) * pageSize
	var tasks []models.GenerationRequest
	if err := query.Order("created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&tasks).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "查询失败",
			"error":   err.Error(),
		})
		return
	}

	// 提取所有图像
	var images []ImageItem
	imageID := 0
	for _, task := range tasks {
		if task.FilesInfo != "" {
			var files []models.FileInfo
			if err := json.Unmarshal([]byte(task.FilesInfo), &files); err == nil {
				for _, file := range files {
					imageID++
					images = append(images, ImageItem{
						ID:          int64(imageID),
						RequestID:   task.RequestID,
						UserID:      task.UserID,
						PromptText:  task.PromptText,
						Status:      task.Status,
						CreatedAt:   task.CreatedAt,
						CompletedAt: task.CompletedAt,
						FileInfo:    file,
					})
				}
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"images":      images,
			"total":       total,
			"page":        page,
			"page_size":   pageSize,
			"image_count": len(images),
		},
	})
}

func GetImageDetail(c *gin.Context) {
	id := c.Param("id")

	// 从请求ID查找任务
	task, err := models.GetGenerationRequestByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": "图像不存在",
		})
		return
	}

	// 解析文件信息
	var files []models.FileInfo
	if task.FilesInfo != "" {
		if err := json.Unmarshal([]byte(task.FilesInfo), &files); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "解析文件信息失败",
			})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"task":  task,
			"files": files,
		},
	})
}

func GetImageFile(c *gin.Context) {
	userID := c.Param("user_id")
	taskID := c.Param("task_id")
	filename := c.Param("filename")

	cfg := config.GetConfig()
	imagePath := filepath.Join(cfg.Storage.ImagePath, userID, taskID, filename)

	// 检查文件是否存在
	if _, err := os.Stat(imagePath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": "图像不存在",
		})
		return
	}

	c.File(imagePath)
}
