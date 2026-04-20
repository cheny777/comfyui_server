package models

import (
	"time"

	"comfyui_front_server/database"
)

type File struct {
	ID              int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	TaskID          string    `gorm:"index;size:50;not null" json:"task_id"`
	Task            Task      `gorm:"foreignKey:TaskID;constraint:OnDelete:CASCADE" json:"-"` // 关联任务
	UserID          int64     `gorm:"index;not null" json:"user_id"`
	User            User      `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"` // 关联用户
	Filename        string    `gorm:"size:255;not null" json:"filename"`
	OriginalFilename string   `gorm:"size:255" json:"original_filename"`
	FilePath        string    `gorm:"size:500;not null" json:"file_path"`
	FileSize        int64     `json:"file_size"`
	FileType        string    `gorm:"size:50" json:"file_type"`
	Width           int       `json:"width,omitempty"`
	Height          int       `json:"height,omitempty"`
	URL             string    `gorm:"size:500" json:"url"`
	CreatedAt       time.Time `gorm:"index" json:"created_at"`
}

func CreateFile(file *File) error {
	return database.DB.Create(file).Error
}

func GetFilesByTaskID(taskID string) ([]*File, error) {
	var files []*File
	err := database.DB.Where("task_id = ?", taskID).Order("created_at ASC").Find(&files).Error
	return files, err
}

func GetFilesByTaskIDAndUserID(taskID string, userID int64) ([]*File, error) {
	var files []*File
	err := database.DB.Where("task_id = ? AND user_id = ?", taskID, userID).
		Order("created_at ASC").Find(&files).Error
	return files, err
}

func DeleteFilesByTaskID(taskID string) error {
	return database.DB.Where("task_id = ?", taskID).Delete(&File{}).Error
}

