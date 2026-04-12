package models

import (
	"time"

	"gorm.io/gorm"
)

type Schedule struct {
	ID                  uint           `gorm:"primaryKey" json:"id"`
	Type                string         `gorm:"column:type;size:30;not null;default:normal" json:"type"`
	LessonTeacherID     uint           `gorm:"column:lesson_kelas_teacher_id;not null;index" json:"lesson_teacher_id"`
	Day                 string         `gorm:"column:hari;size:20;not null" json:"day"` // Senin, Selasa, ...
	StartTime           string         `gorm:"column:jam_mulai;type:time;not null" json:"start_time"`
	EndTime             string         `gorm:"column:jam_selesai;type:time;not null" json:"end_time"`
	SubstituteTeacherID *uint          `gorm:"index" json:"substitute_teacher_id"`
	SubstituteDate      *time.Time     `gorm:"type:date" json:"substitute_date"`
	CreatedAt           time.Time      `gorm:"column:created_at" json:"created_at"`
	UpdatedAt           time.Time      `gorm:"column:updated_at" json:"updated_at"`
	DeletedAt           gorm.DeletedAt `gorm:"column:deleted_at;index" json:"-"`

	// Relationships
	Assignment        LessonTeacher `gorm:"foreignKey:LessonTeacherID" json:"assignment"`
	SubstituteTeacher *User         `gorm:"foreignKey:SubstituteTeacherID" json:"substitute_teacher"`

	// Dynamic fields for frontend
	AttendanceCounts     map[string]int64 `gorm:"-" json:"attendance_counts"`
	HasTeacherAttendance bool             `gorm:"-" json:"has_teacher_attendance_today"`
	HasAttendance        bool             `gorm:"-" json:"has_attendance_today"`
}

func (Schedule) TableName() string { return "jadwal_formal" }
