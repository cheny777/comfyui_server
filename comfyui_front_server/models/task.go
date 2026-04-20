package models

import (
	"encoding/json"
	"time"

	"comfyui_front_server/database"
	"gorm.io/gorm"
)

type Task struct {
	ID          string                 `gorm:"primaryKey;size:50" json:"id"`
	UserID      int64                  `gorm:"index;not null" json:"user_id"`
	User        User                   `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"` // 关联用户
	Workflow    string                  `gorm:"type:text;not null" json:"-"` // JSON格式，不直接返回
	WorkflowObj map[string]interface{} `gorm:"-" json:"workflow"`           // 用于JSON返回
	Status      string                  `gorm:"index;size:20;not null;default:'pending'" json:"status"`
	PromptID    string                  `gorm:"index;size:100" json:"prompt_id"`
	Progress    int                    `gorm:"default:0" json:"progress"`
	Error       string                  `gorm:"type:text" json:"error,omitempty"`
	Files       []File                  `gorm:"foreignKey:TaskID;constraint:OnDelete:CASCADE" json:"files,omitempty"` // 关联文件
	CreatedAt   time.Time              `gorm:"index" json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	CompletedAt *time.Time             `json:"completed_at,omitempty"`
}

func (t *Task) AfterFind(db *gorm.DB) error {
	// 从JSON字符串解析Workflow
	if t.Workflow != "" {
		if err := json.Unmarshal([]byte(t.Workflow), &t.WorkflowObj); err != nil {
			return err
		}
	}
	return nil
}

func (t *Task) BeforeSave(db *gorm.DB) error {
	// 将Workflow对象序列化为JSON字符串
	if t.WorkflowObj != nil {
		data, err := json.Marshal(t.WorkflowObj)
		if err != nil {
			return err
		}
		t.Workflow = string(data)
	}
	return nil
}

func CreateTask(userID int64, taskID string, workflow map[string]interface{}) (*Task, error) {
	workflowJSON, err := json.Marshal(workflow)
	if err != nil {
		return nil, err
	}

	task := &Task{
		ID:       taskID,
		UserID:   userID,
		Workflow: string(workflowJSON),
		Status:   "pending",
		Progress: 0,
	}

	if err := database.DB.Create(task).Error; err != nil {
		return nil, err
	}

	// 解析Workflow用于返回
	task.WorkflowObj = workflow
	return task, nil
}

func GetTaskByID(taskID string) (*Task, error) {
	var task Task
	err := database.DB.Where("id = ?", taskID).First(&task).Error
	if err != nil {
		return nil, err
	}
	return &task, nil
}

func GetTaskByIDAndUserID(taskID string, userID int64) (*Task, error) {
	var task Task
	err := database.DB.Where("id = ? AND user_id = ?", taskID, userID).
		Preload("Files").First(&task).Error
	if err != nil {
		return nil, err
	}
	return &task, nil
}

func GetTasksByUserID(userID int64, page, pageSize int, status string) ([]*Task, int64, error) {
	var tasks []*Task
	var total int64

	query := database.DB.Model(&Task{}).Where("user_id = ?", userID)

	if status != "" {
		query = query.Where("status = ?", status)
	}

	// 获取总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询，预加载文件关联
	offset := (page - 1) * pageSize
	if err := query.Preload("Files").
		Order("created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&tasks).Error; err != nil {
		return nil, 0, err
	}

	return tasks, total, nil
}

func UpdateTask(taskID string, updates map[string]interface{}) error {
	updates["updated_at"] = time.Now()
	return database.DB.Model(&Task{}).Where("id = ?", taskID).Updates(updates).Error
}

func DeleteTask(taskID string, userID int64) error {
	return database.DB.Where("id = ? AND user_id = ?", taskID, userID).Delete(&Task{}).Error
}

