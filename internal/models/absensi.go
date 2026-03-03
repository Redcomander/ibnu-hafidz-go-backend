package models

import (
	"time"

	"gorm.io/gorm"
)

// Absensi represents student attendance for Formal schedules
type Absensi struct {
	ID             uint           `gorm:"primaryKey" json:"id"`
	JadwalFormalID uint           `gorm:"column:jadwal_formal_id;not null;index;uniqueIndex:idx_absensi_formal_student_date" json:"jadwal_formal_id"`
	StudentID      uint           `gorm:"column:student_id;not null;index;uniqueIndex:idx_absensi_formal_student_date" json:"student_id"`
	Status         string         `gorm:"type:enum('hadir', 'izin', 'sakit', 'alpa');default:'hadir';not null" json:"status"`
	Catatan        string         `gorm:"type:text" json:"catatan"`
	Tanggal        time.Time      `gorm:"type:date;not null;index;uniqueIndex:idx_absensi_formal_student_date" json:"tanggal"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`

	// Relationships
	JadwalFormal Schedule `gorm:"foreignKey:JadwalFormalID" json:"jadwal_formal"`
	Student      Student  `gorm:"foreignKey:StudentID" json:"student"`
}

func (Absensi) TableName() string { return "absensis" }

// AbsensiDiniyyah represents student attendance for Diniyyah schedules
type AbsensiDiniyyah struct {
	ID               uint           `gorm:"primaryKey" json:"id"`
	JadwalDiniyyahID *uint          `gorm:"column:jadwal_diniyyah_id;index;uniqueIndex:idx_absensi_diniyyah_student_date" json:"jadwal_diniyyah_id"`
	StudentID        uint           `gorm:"column:student_id;not null;index;uniqueIndex:idx_absensi_diniyyah_student_date" json:"student_id"`
	Status           string         `gorm:"type:enum('hadir', 'izin', 'sakit', 'alpa');default:'hadir';not null" json:"status"`
	Catatan          string         `gorm:"type:text" json:"catatan"`
	Tanggal          time.Time      `gorm:"type:date;not null;index;uniqueIndex:idx_absensi_diniyyah_student_date" json:"tanggal"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `gorm:"index" json:"-"`

	// Relationships
	JadwalDiniyyah *DiniyyahSchedule `gorm:"foreignKey:JadwalDiniyyahID" json:"jadwal_diniyyah"`
	Student        Student           `gorm:"foreignKey:StudentID" json:"student"`
}

func (AbsensiDiniyyah) TableName() string { return "absensi_diniyyahs" }
