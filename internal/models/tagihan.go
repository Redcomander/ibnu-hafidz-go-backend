package models

import (
	"time"

	"gorm.io/gorm"
)

// Tagihan stores customer billing information with source tracking and import metadata.
type Tagihan struct {
	ID            uint           `gorm:"primarykey" json:"id"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	NIS           *string        `gorm:"column:nis;size:64;index" json:"nis,omitempty"`
	Nama          string         `gorm:"column:nama;size:255;not null" json:"nama"`
	NoWhatsapp    string         `gorm:"column:no_whatsapp;size:32;not null;index" json:"no_whatsapp"`
	TotalTagihan  int64          `gorm:"column:total_tagihan;default:0;index" json:"total_tagihan"`
	StatusTagihan string         `gorm:"column:status_tagihan;size:50;not null;default:'belum_lunas';index" json:"status_tagihan"`
	HandlerID     *uint          `gorm:"column:handler_id;index" json:"handler_id,omitempty"`
	Handler       *User          `gorm:"foreignKey:HandlerID" json:"handler,omitempty"`
	SumberData    *string        `gorm:"column:sumber_data;size:100" json:"sumber_data,omitempty"`
	Catatan       *string        `gorm:"column:catatan;type:text" json:"catatan,omitempty"`
	LastContactAt *time.Time     `gorm:"column:last_contact_at" json:"last_contact_at,omitempty"`
	ImportBatchID *uint          `gorm:"column:import_batch_id;index" json:"import_batch_id,omitempty"`
	ImportBatch   *ImportBatch   `gorm:"foreignKey:ImportBatchID" json:"import_batch,omitempty"`
}

func (Tagihan) TableName() string { return "tagihan" }
