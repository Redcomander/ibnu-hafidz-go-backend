package models

import (
	"time"

	"gorm.io/gorm"
)

// Notification represents a system notification for a user
type Notification struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	UserID    uint           `gorm:"column:user_id;not null;index" json:"user_id"`
	User      User           `gorm:"foreignKey:UserID" json:"-"`
	Title     string         `gorm:"column:title;size:255;not null" json:"title"`
	Message   string         `gorm:"column:message;type:text;not null" json:"message"`
	Type      string         `gorm:"column:type;size:50;default:'info'" json:"type"` // info, success, warning, error
	IsRead    bool           `gorm:"column:is_read;default:false" json:"is_read"`
	ActionURL *string        `gorm:"column:action_url;size:255" json:"action_url,omitempty"`
	CreatedAt time.Time      `gorm:"column:created_at" json:"created_at"`
	UpdatedAt time.Time      `gorm:"column:updated_at" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;index" json:"-"`
}

func (Notification) TableName() string { return "notifications" }
