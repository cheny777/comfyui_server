package middleware

import (
	"comfyui_front_server/config"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "未授权"})
			c.Abort()
			return
		}

		// 提取Token
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "Token格式错误"})
			c.Abort()
			return
		}

		tokenString := parts[1]

		// 验证Token
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			return []byte(config.GetJWTSecret()), nil
		})

		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "Token无效"})
			c.Abort()
			return
		}

		// 提取用户信息
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "Token解析失败"})
			c.Abort()
			return
		}

		// 将用户ID存储到上下文
		userIDFloat, ok := claims["user_id"].(float64)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "Token中缺少user_id"})
			c.Abort()
			return
		}
		userID := int64(userIDFloat)

		deviceID, _ := claims["device_id"].(string)

		c.Set("user_id", userID)
		c.Set("device_id", deviceID)

		c.Next()
	}
}

