package models

import "time"

// FeatureFlag stores toggleable system features.
type FeatureFlag struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Key       string    `gorm:"column:key;size:100;uniqueIndex;not null" json:"key"`
	IsEnabled bool      `gorm:"column:is_enabled;default:false" json:"is_enabled"`
	UpdatedAt time.Time `gorm:"column:updated_at" json:"updated_at"`
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
}

func (FeatureFlag) TableName() string { return "feature_flags" }
