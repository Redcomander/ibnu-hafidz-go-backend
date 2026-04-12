package models

import (
	"time"

	"gorm.io/gorm"
)

type SidebarMenuSetting struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	MenuKey   string         `gorm:"column:menu_key;size:100;not null;uniqueIndex" json:"menu_key"`
	IsActive  bool           `gorm:"column:is_active;not null;default:true" json:"is_active"`
	CreatedAt time.Time      `gorm:"column:created_at" json:"created_at"`
	UpdatedAt time.Time      `gorm:"column:updated_at" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;index" json:"-"`
}

func (SidebarMenuSetting) TableName() string { return "sidebar_menu_settings" }
