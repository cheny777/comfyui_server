package models

import (
	"time"

	"comfyui_front_server/database"
	"gorm.io/gorm"
)

type User struct {
	ID           int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	DeviceID     string    `gorm:"uniqueIndex;size:100;not null" json:"device_id"`
	UserType     string    `gorm:"size:20;not null;default:'guest'" json:"user_type"`
	Nickname     string    `gorm:"size:50" json:"nickname"`
	Avatar       string    `gorm:"size:255" json:"avatar"`
	LastActiveAt time.Time `gorm:"index" json:"last_active_at"`
	Tasks        []Task    `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"` // 关联任务
	Files        []File    `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"` // 关联文件
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func CreateUser(deviceID, nickname string) (*User, error) {
	user := &User{
		DeviceID:     deviceID,
		UserType:     "guest",
		Nickname:     nickname,
		LastActiveAt: time.Now(),
	}

	if err := database.DB.Create(user).Error; err != nil {
		return nil, err
	}

	return user, nil
}

func GetUserByDeviceID(deviceID string) (*User, error) {
	var user User
	err := database.DB.Where("device_id = ?", deviceID).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func GetUserByID(id int64) (*User, error) {
	var user User
	err := database.DB.First(&user, id).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func UpdateUser(id int64, updates map[string]interface{}) error {
	updates["updated_at"] = time.Now()
	return database.DB.Model(&User{}).Where("id = ?", id).Updates(updates).Error
}

func UpdateUserLastActive(id int64) error {
	return database.DB.Model(&User{}).Where("id = ?", id).Update("last_active_at", time.Now()).Error
}

// GetDB 获取数据库实例（用于复杂查询）
func GetDB() *gorm.DB {
	return database.DB
}

