package models

import "time"

// UserActivityLog stores request-level user activity for audit and troubleshooting.
type UserActivityLog struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	UserID     uint      `gorm:"column:user_id;index;not null" json:"user_id"`
	User       *User     `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Method     string    `gorm:"column:method;size:16;index" json:"method"`
	Path       string    `gorm:"column:path;size:255;index" json:"path"`
	StatusCode int       `gorm:"column:status_code;index" json:"status_code"`
	IPAddress  *string   `gorm:"column:ip_address;size:100" json:"ip_address,omitempty"`
	UserAgent  *string   `gorm:"column:user_agent;type:text" json:"user_agent,omitempty"`
	CreatedAt  time.Time `gorm:"column:created_at;index" json:"created_at"`
}

func (UserActivityLog) TableName() string { return "user_activity_logs" }
