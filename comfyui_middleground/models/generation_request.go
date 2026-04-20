package models

import (
	"time"

	"comfyui_middleground/database"
)

type GenerationRequest struct {
	ID             int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	RequestID      string     `gorm:"uniqueIndex;size:50;not null" json:"request_id"`
	UserID         int64      `gorm:"index;not null" json:"user_id"`
	DeviceID       string     `gorm:"index;size:100" json:"device_id"`
	PromptID       string     `gorm:"index;size:100" json:"prompt_id"`
	PromptText     string     `gorm:"type:text" json:"prompt_text"`
	NegativePrompt string     `gorm:"type:text" json:"negative_prompt"`
	Workflow       string     `gorm:"type:text;not null" json:"workflow"`
	Status         string     `gorm:"index;size:20;default:'pending';not null" json:"status"`
	Progress       int        `gorm:"default:0" json:"progress"`
	Error          string     `gorm:"type:text" json:"error,omitempty"`
	FilesInfo      string     `gorm:"type:text" json:"files_info"`
	CreatedAt      time.Time  `gorm:"index" json:"created_at"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	UpdatedAt      time.Time  `gorm:"index" json:"updated_at"`
}

type FileInfo struct {
	Filename         string `json:"filename"`
	OriginalFilename string `json:"original_filename"`
	FilePath         string `json:"file_path"`
	FileSize         int64  `json:"file_size"`
	FileType         string `json:"file_type"`
	Width            int    `json:"width,omitempty"`
	Height           int    `json:"height,omitempty"`
	URL              string `json:"url"`
}

func CreateGenerationRequest(req *GenerationRequest) error {
	return database.DB.Create(req).Error
}

func UpdateGenerationRequest(requestID string, updates map[string]interface{}) error {
	updates["updated_at"] = time.Now()
	return database.DB.Model(&GenerationRequest{}).
		Where("request_id = ?", requestID).
		Updates(updates).Error
}

func GetGenerationRequests(page, pageSize int, userID int64, deviceID, status, search string) ([]*GenerationRequest, int64, error) {
	var requests []*GenerationRequest
	var total int64

	query := database.DB.Model(&GenerationRequest{})

	// 构建查询条件
	if userID > 0 {
		query = query.Where("user_id = ?", userID)
	}
	if deviceID != "" {
		query = query.Where("device_id = ?", deviceID)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if search != "" {
		searchPattern := "%" + search + "%"
		query = query.Where("prompt_text LIKE ? OR request_id LIKE ?", searchPattern, searchPattern)
	}

	// 获取总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&requests).Error; err != nil {
		return nil, 0, err
	}

	return requests, total, nil
}

func GetGenerationRequestByID(requestID string) (*GenerationRequest, error) {
	var req GenerationRequest
	err := database.DB.Where("request_id = ?", requestID).First(&req).Error
	if err != nil {
		return nil, err
	}
	return &req, nil
}

func GetGenerationRequestByPromptID(promptID string) (*GenerationRequest, error) {
	var req GenerationRequest
	err := database.DB.Where("prompt_id = ?", promptID).First(&req).Error
	if err != nil {
		return nil, err
	}
	return &req, nil
}
