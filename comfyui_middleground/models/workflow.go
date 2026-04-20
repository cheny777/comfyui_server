package models

import (
	"time"

	"gorm.io/gorm"
	"comfyui_middleground/database"
)

type Workflow struct {
	ID                int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Name              string    `gorm:"index;size:100;not null" json:"name"`                    // 工作流名称
	Description       string    `gorm:"type:text" json:"description"`                            // 工作流描述
	WorkflowJSON      string    `gorm:"type:text;not null" json:"workflow_json"`                // 工作流JSON配置
	Category          string    `gorm:"index;size:50" json:"category"`                         // 分类
	Tags              string    `gorm:"size:255" json:"tags"`                                   // 标签（逗号分隔）
	PositiveNodeID    string    `gorm:"size:50" json:"positive_node_id"`                        // 正向提示词节点ID
	PositiveFieldName string    `gorm:"size:50" json:"positive_field_name"`                   // 正向提示词字段名（如：text）
	NegativeNodeID    string    `gorm:"size:50" json:"negative_node_id"`                        // 负向提示词节点ID
	NegativeFieldName string    `gorm:"size:50" json:"negative_field_name"`                     // 负向提示词字段名（如：text）
	IsDefault         bool      `gorm:"index;default:false" json:"is_default"`                  // 是否默认工作流
	IsPublic          bool      `gorm:"default:true" json:"is_public"`                           // 是否公开
	CreatedBy         string    `gorm:"size:100" json:"created_by"`                            // 创建者
	UsageCount        int       `gorm:"default:0" json:"usage_count"`                            // 使用次数
	CreatedAt         time.Time `gorm:"index" json:"created_at"`
	UpdatedAt         time.Time `gorm:"index" json:"updated_at"`
}

type MiddleConnection struct {
	ID           int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	ServerID     string     `gorm:"uniqueIndex;size:50;not null" json:"server_id"`
	Status       string     `gorm:"index;size:20;default:'offline';not null" json:"status"`
	LastHeartbeat *time.Time `json:"last_heartbeat,omitempty"`
	CreatedAt    time.Time  `gorm:"index" json:"created_at"`
	UpdatedAt    time.Time  `gorm:"index" json:"updated_at"`
}

func CreateWorkflow(workflow *Workflow) error {
	return database.DB.Create(workflow).Error
}

func UpdateWorkflow(id int64, workflow *Workflow) error {
	return database.DB.Model(&Workflow{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"name":               workflow.Name,
			"description":        workflow.Description,
			"workflow_json":      workflow.WorkflowJSON,
			"category":           workflow.Category,
			"tags":               workflow.Tags,
			"positive_node_id":   workflow.PositiveNodeID,
			"positive_field_name": workflow.PositiveFieldName,
			"negative_node_id":   workflow.NegativeNodeID,
			"negative_field_name": workflow.NegativeFieldName,
			"is_default":         workflow.IsDefault,
			"is_public":          workflow.IsPublic,
			"updated_at":         time.Now(),
		}).Error
}

func DeleteWorkflow(id int64) error {
	return database.DB.Delete(&Workflow{}, id).Error
}

func GetWorkflowByID(id int64) (*Workflow, error) {
	var workflow Workflow
	err := database.DB.First(&workflow, id).Error
	if err != nil {
		return nil, err
	}
	return &workflow, nil
}

func GetWorkflowList(page, pageSize int, category, search string) ([]*Workflow, int64, error) {
	var workflows []*Workflow
	var total int64

	query := database.DB.Model(&Workflow{})

	// 构建查询条件
	if category != "" {
		query = query.Where("category = ?", category)
	}
	if search != "" {
		searchPattern := "%" + search + "%"
		query = query.Where("name LIKE ? OR description LIKE ? OR tags LIKE ?", 
			searchPattern, searchPattern, searchPattern)
	}

	// 获取总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	offset := (page - 1) * pageSize
	if err := query.Order("updated_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&workflows).Error; err != nil {
		return nil, 0, err
	}

	return workflows, total, nil
}

func IncrementWorkflowUsage(id int64) error {
	return database.DB.Model(&Workflow{}).
		Where("id = ?", id).
		UpdateColumn("usage_count", gorm.Expr("usage_count + 1")).
		Update("updated_at", time.Now()).Error
}
