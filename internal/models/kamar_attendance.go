package models

import (
	"time"
)

// KamarAttendanceSession represents a room attendance cycle for a room.
// It intentionally separates room membership from attendance records so the same
// student can be tracked across multiple sessions without altering room assignment.
type KamarAttendanceSession struct {
	ID              uint       `gorm:"primaryKey" json:"id"`
	KamarID         uint       `gorm:"column:kamar_id;not null;index" json:"kamar_id"`
	Name            string     `gorm:"column:name;size:255;not null" json:"name"`
	SessionType     string     `gorm:"column:session_type;size:50;default:'daily';not null" json:"session_type"`
	WeekStartDate   *time.Time `gorm:"column:week_start_date;type:date" json:"week_start_date,omitempty"`
	CreatedByUserID *uint      `gorm:"column:created_by_user_id;index" json:"created_by_user_id,omitempty"`
	Notes           *string    `gorm:"column:notes;type:text" json:"notes,omitempty"`
	StartsAt        *time.Time `gorm:"column:starts_at" json:"starts_at,omitempty"`
	EndsAt          *time.Time `gorm:"column:ends_at" json:"ends_at,omitempty"`
	IsActive        bool       `gorm:"column:is_active;default:true;not null" json:"is_active"`
	CreatedAt       time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt       time.Time  `gorm:"column:updated_at" json:"updated_at"`

	Kamar   Kamar                   `gorm:"foreignKey:KamarID" json:"kamar,omitempty"`
	Creator *User                   `gorm:"foreignKey:CreatedByUserID" json:"creator,omitempty"`
	Records []KamarAttendanceRecord `gorm:"foreignKey:KamarAttendanceSessionID" json:"records,omitempty"`
}

func (KamarAttendanceSession) TableName() string { return "kamar_attendance_sessions" }

// KamarAttendanceRecord stores a single student status inside a room attendance session.
type KamarAttendanceRecord struct {
	ID                       uint      `gorm:"primaryKey" json:"id"`
	KamarAttendanceSessionID uint      `gorm:"column:kamar_attendance_session_id;not null;index" json:"kamar_attendance_session_id"`
	StudentID                uint      `gorm:"column:student_id;not null;index" json:"student_id"`
	Status                   string    `gorm:"column:status;size:50;default:'hadir';not null" json:"status"`
	Notes                    *string   `gorm:"column:notes;type:text" json:"notes,omitempty"`
	SubmittedBy              *uint     `gorm:"column:submitted_by;index" json:"submitted_by,omitempty"`
	CreatedAt                time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt                time.Time `gorm:"column:updated_at" json:"updated_at"`

	Session   KamarAttendanceSession `gorm:"foreignKey:KamarAttendanceSessionID" json:"session,omitempty"`
	Student   Student                `gorm:"foreignKey:StudentID" json:"student,omitempty"`
	Submitter *User                  `gorm:"foreignKey:SubmittedBy" json:"submitter,omitempty"`
}

func (KamarAttendanceRecord) TableName() string { return "kamar_attendance_records" }
