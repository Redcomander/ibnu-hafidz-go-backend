package models

import (
	"time"

	"gorm.io/gorm"
)

// StudentArrival records student arrival per day.
type StudentArrival struct {
	ID                uint           `gorm:"primaryKey" json:"id"`
	StudentID         uint           `gorm:"column:student_id;not null;index;uniqueIndex:ux_student_arrivals_daily,priority:1" json:"student_id"`
	TanggalKedatangan time.Time      `gorm:"column:tanggal_kedatangan;type:date;not null;index;uniqueIndex:ux_student_arrivals_daily,priority:2" json:"tanggal_kedatangan"`
	DicatatOlehIP     *string        `gorm:"column:dicatat_oleh_ip;size:45" json:"dicatat_oleh_ip,omitempty"`
	CreatedAt         time.Time      `gorm:"column:created_at" json:"created_at"`
	UpdatedAt         time.Time      `gorm:"column:updated_at" json:"updated_at"`
	DeletedAt         gorm.DeletedAt `gorm:"column:deleted_at;index" json:"-"`

	Student *Student `gorm:"foreignKey:StudentID" json:"student,omitempty"`
}

func (StudentArrival) TableName() string { return "student_arrivals" }

// UserArrival records teacher/user arrival per day.
type UserArrival struct {
	ID                uint           `gorm:"primaryKey" json:"id"`
	UserID            uint           `gorm:"column:user_id;not null;index;uniqueIndex:ux_user_arrivals_daily,priority:1" json:"user_id"`
	TanggalKedatangan time.Time      `gorm:"column:tanggal_kedatangan;type:date;not null;index;uniqueIndex:ux_user_arrivals_daily,priority:2" json:"tanggal_kedatangan"`
	DicatatOlehIP     *string        `gorm:"column:dicatat_oleh_ip;size:45" json:"dicatat_oleh_ip,omitempty"`
	CreatedAt         time.Time      `gorm:"column:created_at" json:"created_at"`
	UpdatedAt         time.Time      `gorm:"column:updated_at" json:"updated_at"`
	DeletedAt         gorm.DeletedAt `gorm:"column:deleted_at;index" json:"-"`

	User *User `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

func (UserArrival) TableName() string { return "user_arrivals" }

// StudentArrivalSetting stores global settings for student arrival handling.
type StudentArrivalSetting struct {
	ID                uint           `gorm:"primaryKey" json:"id"`
	StudentDeadlineAt *time.Time     `gorm:"column:student_deadline_at" json:"student_deadline_at,omitempty"`
	CreatedAt         time.Time      `gorm:"column:created_at" json:"created_at"`
	UpdatedAt         time.Time      `gorm:"column:updated_at" json:"updated_at"`
	DeletedAt         gorm.DeletedAt `gorm:"column:deleted_at;index" json:"-"`
}

func (StudentArrivalSetting) TableName() string { return "student_arrival_settings" }
