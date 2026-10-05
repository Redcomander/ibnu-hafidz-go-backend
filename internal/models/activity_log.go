package models

import "time"

// UserActivityLog stores request-level user activity for audit and troubleshooting.
type UserActivityLog struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	UserID      uint      `gorm:"column:user_id;index" json:"user_id"`
	User        *User     `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Method      string    `gorm:"column:method;size:16;index" json:"method"`
	Path        string    `gorm:"column:path;size:255;index" json:"path"`
	StatusCode  int       `gorm:"column:status_code;index" json:"status_code"`
	DurationMs  *int64    `gorm:"column:duration_ms;index" json:"duration_ms,omitempty"`
	IPAddress   *string   `gorm:"column:ip_address;size:100" json:"ip_address,omitempty"`
	UserAgent   *string   `gorm:"column:user_agent;type:text" json:"user_agent,omitempty"`
	DeviceName  *string   `gorm:"column:device_name;size:255" json:"device_name,omitempty"`
	CountryCode *string   `gorm:"column:country_code;size:10" json:"country_code,omitempty"`
	CreatedAt   time.Time `gorm:"column:created_at;index" json:"created_at"`
}

func (UserActivityLog) TableName() string { return "user_activity_logs" }
