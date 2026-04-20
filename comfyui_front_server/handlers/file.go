package handlers

import (
	"comfyui_front_server/models"
	"comfyui_front_server/websocket"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// GetImage 获取图像文件（代理到中台）
// 实际文件存储在中台，这里只是记录文件信息
func GetImage(c *gin.Context) {
	userID := c.MustGet("user_id").(int64)
	paramUserID := c.Param("user_id")
	taskID := c.Param("task_id")
	filename := c.Param("filename")

	// 验证用户权限
	paramUserIDInt, err := strconv.ParseInt(paramUserID, 10, 64)
	if err != nil || paramUserIDInt != userID {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": "无权访问该文件"})
		return
	}

	// 验证任务属于该用户
	_, err = models.GetTaskByIDAndUserID(taskID, userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "任务不存在"})
		return
	}

	// 验证文件存在
	files, err := models.GetFilesByTaskIDAndUserID(taskID, userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "文件不存在"})
		return
	}

	// 查找文件
	var file *models.File
	for _, f := range files {
		if f.Filename == filename {
			file = f
			break
		}
	}

	if file == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "文件不存在"})
		return
	}

	// 重定向到中台的图像URL
	c.Redirect(http.StatusFound, file.URL)
}

// QueryTaskImageFiles 查询任务图像文件列表或单个文件
// GET /api/tasks/:id/images?directory=xxx&filename=xxx
func QueryTaskImageFiles(c *gin.Context) {
	userID := c.MustGet("user_id").(int64)
	taskID := c.Param("id")

	// 验证任务属于该用户
	_, err := models.GetTaskByIDAndUserID(taskID, userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "任务不存在"})
		return
	}

	// 获取查询参数
	directory := c.Query("directory")
	filename := c.Query("filename")

	// 如果指定了文件名，查询单个文件
	if filename != "" {
		// 先查询数据库，检查文件记录是否存在
		files, err := models.GetFilesByTaskIDAndUserID(taskID, userID)
		if err == nil {
			// 查找匹配的文件
			for _, file := range files {
				if file.Filename == filename {
					// 文件记录存在，但需要查询中台获取base64内容
					// 通过WebSocket查询中台获取图像文件内容
					result, err := websocket.QueryImageFile(userID, taskID, directory, filename)
					if err != nil {
						c.JSON(http.StatusInternalServerError, gin.H{
							"code":    500,
							"message": "查询图像文件内容失败: " + err.Error(),
						})
						return
					}
					// 合并数据库中的文件信息和查询到的文件内容
					result["filename"] = file.Filename
					result["original_filename"] = file.OriginalFilename
					result["file_path"] = file.FilePath
					result["file_size"] = file.FileSize
					result["file_type"] = file.FileType
					result["width"] = file.Width
					result["height"] = file.Height
					result["url"] = file.URL
					c.JSON(http.StatusOK, gin.H{
						"code":    0,
						"message": "success",
						"data":    result,
					})
					return
				}
			}
		}

		// 文件记录不存在，查询中台并保存到数据库
		result, err := websocket.QueryImageFile(userID, taskID, directory, filename)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "查询图像文件失败: " + err.Error(),
			})
			return
		}

		// 保存文件记录到数据库
		if err := AddFile(taskID, userID, result); err != nil {
			// 保存失败不影响返回结果，只记录日志
			// logger.Warnf("保存文件记录失败: %v", err)
		}

		c.JSON(http.StatusOK, gin.H{
			"code":    0,
			"message": "success",
			"data":    result,
		})
		return
	}

	// 查询任务的所有图像文件
	// 先查询数据库，检查文件记录是否存在
	dbFiles, err := models.GetFilesByTaskIDAndUserID(taskID, userID)
	if err == nil && len(dbFiles) > 0 {
		// 数据库中有文件记录，但需要查询中台获取base64内容
		result, err := websocket.QueryImageFile(userID, taskID, directory, filename)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "查询图像文件内容失败: " + err.Error(),
			})
			return
		}

		// 如果中台返回了images数组，合并数据库中的文件信息
		if images, ok := result["images"].([]interface{}); ok {
			// 创建文件名到文件记录的映射
			fileMap := make(map[string]*models.File)
			for _, file := range dbFiles {
				fileMap[file.Filename] = file
			}

			// 更新images数组中的文件信息
			for i, img := range images {
				if imgMap, ok := img.(map[string]interface{}); ok {
					if filename, ok := imgMap["filename"].(string); ok {
						if dbFile, exists := fileMap[filename]; exists {
							imgMap["original_filename"] = dbFile.OriginalFilename
							imgMap["file_path"] = dbFile.FilePath
							if imgMap["file_size"] == nil {
								imgMap["file_size"] = dbFile.FileSize
							}
							if imgMap["file_type"] == nil || imgMap["file_type"] == "" {
								imgMap["file_type"] = dbFile.FileType
							}
							if dbFile.Width > 0 {
								imgMap["width"] = dbFile.Width
							}
							if dbFile.Height > 0 {
								imgMap["height"] = dbFile.Height
							}
							if imgMap["url"] == nil || imgMap["url"] == "" {
								imgMap["url"] = dbFile.URL
							}
						}
					}
					images[i] = imgMap
				}
			}
			result["images"] = images
		}

		c.JSON(http.StatusOK, gin.H{
			"code":    0,
			"message": "success",
			"data":    result,
		})
		return
	}

	// 数据库中没有文件记录，查询中台并保存到数据库
	result, err := websocket.QueryImageFile(userID, taskID, directory, filename)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "查询图像文件失败: " + err.Error(),
		})
		return
	}

	// 保存文件记录到数据库
	if images, ok := result["images"].([]interface{}); ok {
		for _, img := range images {
			if imgMap, ok := img.(map[string]interface{}); ok {
				if err := AddFile(taskID, userID, imgMap); err != nil {
					// 保存失败不影响返回结果，只记录日志
					// logger.Warnf("保存文件记录失败: %v", err)
				}
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    result,
	})
}

// AddFile 添加文件记录（由WebSocket消息处理器调用）
func AddFile(taskID string, userID int64, fileInfo map[string]interface{}) error {
	file := &models.File{
		TaskID:           taskID,
		UserID:           userID,
		Filename:         getString(fileInfo, "filename"),
		OriginalFilename: getString(fileInfo, "original_filename"),
		FilePath:         getString(fileInfo, "file_path"),
		FileSize:         getInt64(fileInfo, "file_size"),
		FileType:         getString(fileInfo, "file_type"),
		Width:            getInt(fileInfo, "width"),
		Height:           getInt(fileInfo, "height"),
		URL:              getString(fileInfo, "url"),
	}

	return models.CreateFile(file)
}

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getInt64(m map[string]interface{}, key string) int64 {
	if v, ok := m[key].(float64); ok {
		return int64(v)
	}
	return 0
}

func getInt(m map[string]interface{}, key string) int {
	if v, ok := m[key].(float64); ok {
		return int(v)
	}
	return 0
}
