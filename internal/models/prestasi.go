package models

import (
	"time"

	"gorm.io/gorm"
)

// Prestasi — student achievement / accomplishment
type Prestasi struct {
	ID           uint           `gorm:"primaryKey" json:"id"`
	NamaPrestasi string         `gorm:"type:varchar(255);not null" json:"nama_prestasi"`
	Kategori     string         `gorm:"type:varchar(100);not null" json:"kategori"`
	Tingkat      *string        `gorm:"type:varchar(100)" json:"tingkat"`
	Tanggal      *string        `gorm:"type:date" json:"tanggal"`
	Deskripsi    *string        `gorm:"type:text" json:"deskripsi"`
	PhotoPath    *string        `gorm:"type:varchar(500)" json:"photo_path"`
	WebpPath     *string        `gorm:"type:varchar(500)" json:"webp_path"`
	StudentID    *uint          `gorm:"index" json:"student_id"`
	UserID       uint           `gorm:"not null" json:"user_id"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`

	// Relationships
	Student *Student `gorm:"foreignKey:StudentID" json:"student,omitempty"`
	User    *User    `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

func (Prestasi) TableName() string { return "prestasis" }
