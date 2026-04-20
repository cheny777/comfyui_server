package handlers

import (
	"comfyui_front_server/config"
	"comfyui_front_server/models"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// InitUser 初始化/获取用户（自动创建游客用户）
func InitUser(c *gin.Context) {
	var req struct {
		DeviceID string `json:"device_id"`
		Nickname  string `json:"nickname"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误", "error": err.Error()})
		return
	}

	// 如果没有提供device_id，自动生成
	deviceID := req.DeviceID
	if deviceID == "" {
		deviceID = generateDeviceID()
	}

	// 查找或创建用户
	user, err := models.GetUserByDeviceID(deviceID)
	if err != nil {
		// 用户不存在，创建新用户
		user, err = models.CreateUser(deviceID, req.Nickname)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "创建用户失败", "error": err.Error()})
			return
		}
	} else {
		// 用户存在，更新最后活跃时间
		models.UpdateUserLastActive(user.ID)
		if req.Nickname != "" && req.Nickname != user.Nickname {
			models.UpdateUser(user.ID, map[string]interface{}{"nickname": req.Nickname})
			user.Nickname = req.Nickname
		}
	}

	// 生成JWT Token
	token, err := generateJWTToken(user.ID, user.DeviceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "生成Token失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"message": "success",
		"data": gin.H{
			"token": token,
			"user":  user,
		},
	})
}

// GetUserInfo 获取当前用户信息
func GetUserInfo(c *gin.Context) {
	userID := c.MustGet("user_id").(int64)

	user, err := models.GetUserByID(userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "用户不存在"})
		return
	}

	// 更新最后活跃时间
	models.UpdateUserLastActive(userID)

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"message": "success",
		"data": user,
	})
}

// UpdateUserProfile 更新用户信息
func UpdateUserProfile(c *gin.Context) {
	userID := c.MustGet("user_id").(int64)

	var req struct {
		Nickname string `json:"nickname"`
		Avatar   string `json:"avatar"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误", "error": err.Error()})
		return
	}

	updates := make(map[string]interface{})
	if req.Nickname != "" {
		updates["nickname"] = req.Nickname
	}
	if req.Avatar != "" {
		updates["avatar"] = req.Avatar
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "没有要更新的字段"})
		return
	}

	if err := models.UpdateUser(userID, updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "更新用户信息失败", "error": err.Error()})
		return
	}

	user, _ := models.GetUserByID(userID)

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"message": "success",
		"data": user,
	})
}

// GetUserHistory 获取用户历史统计
func GetUserHistory(c *gin.Context) {
	userID := c.MustGet("user_id").(int64)

	// 使用GORM聚合函数统计任务
	var totalTasks, completedTasks, failedTasks, pendingTasks int64
	
	// 总任务数
	models.GetDB().Model(&models.Task{}).Where("user_id = ?", userID).Count(&totalTasks)
	
	// 已完成任务数
	models.GetDB().Model(&models.Task{}).Where("user_id = ? AND status = ?", userID, "completed").Count(&completedTasks)
	
	// 失败任务数
	models.GetDB().Model(&models.Task{}).Where("user_id = ? AND status = ?", userID, "failed").Count(&failedTasks)
	
	// 待处理任务数（pending + running）
	models.GetDB().Model(&models.Task{}).Where("user_id = ? AND status IN ?", userID, []string{"pending", "running"}).Count(&pendingTasks)

	// 使用GORM聚合函数统计文件总数（图像数量）
	var totalImages int64
	models.GetDB().Model(&models.File{}).Where("user_id = ?", userID).Count(&totalImages)

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"message": "success",
		"data": gin.H{
			"total_tasks":     totalTasks,
			"completed_tasks": completedTasks,
			"failed_tasks":    failedTasks,
			"pending_tasks":   pendingTasks,
			"total_images":    totalImages,
		},
	})
}

// generateDeviceID 生成设备ID（UUID v4格式）
func generateDeviceID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // Version 4
	b[8] = (b[8] & 0x3f) | 0x80 // Variant is 10
	return hex.EncodeToString(b[0:4]) + "-" + hex.EncodeToString(b[4:6]) + "-" +
		hex.EncodeToString(b[6:8]) + "-" + hex.EncodeToString(b[8:10]) + "-" +
		hex.EncodeToString(b[10:])
}

// generateJWTToken 生成JWT Token
func generateJWTToken(userID int64, deviceID string) (string, error) {
	claims := jwt.MapClaims{
		"user_id":  userID,
		"device_id": deviceID,
		"exp":      time.Now().Add(config.GetJWTExpireDuration()).Unix(),
		"iat":      time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(config.GetJWTSecret()))
}

