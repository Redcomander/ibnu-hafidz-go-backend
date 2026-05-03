package models

import (
	"time"

	"gorm.io/gorm"
)

// TeacherAttendance represents daily attendance for a teacher on a specific schedule
type TeacherAttendance struct {
	ID               uint           `gorm:"primaryKey" json:"id"`
	JadwalFormalID   *uint          `gorm:"column:jadwal_formal_id;index" json:"jadwal_formal_id"`
	JadwalDiniyyahID *uint          `gorm:"column:jadwal_diniyyah_id;index" json:"jadwal_diniyyah_id"`
	UserID           uint           `gorm:"column:user_id;not null;index" json:"user_id"` // Original Assigned Teacher
	Date             time.Time      `gorm:"type:date;not null;index" json:"date"`
	Status           string         `gorm:"type:enum('Hadir', 'Izin', 'Sakit', 'Alpha');not null" json:"status"`
	Notes            string         `gorm:"type:text" json:"notes"`
	PhotoPath        string         `gorm:"type:varchar(255)" json:"photo_path"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `gorm:"index" json:"-"`

	// Relationships
	JadwalFormal   *Schedule         `gorm:"foreignKey:JadwalFormalID" json:"jadwal_formal,omitempty"`
	JadwalDiniyyah *DiniyyahSchedule `gorm:"foreignKey:JadwalDiniyyahID" json:"jadwal_diniyyah,omitempty"`
	User           User              `gorm:"foreignKey:UserID" json:"user"`
}

func (TeacherAttendance) TableName() string { return "teacher_attendances" }

// TeacherAttendanceSnapshot keeps immutable lesson/kelas labels at attendance write time,
// so historical records remain stable even if the schedule assignment is edited later.
type TeacherAttendanceSnapshot struct {
	ID                  uint      `gorm:"primaryKey" json:"id"`
	TeacherAttendanceID uint      `gorm:"column:teacher_attendance_id;uniqueIndex;not null" json:"teacher_attendance_id"`
	Lesson              string    `gorm:"column:lesson;type:varchar(255);not null" json:"lesson"`
	Kelas               string    `gorm:"column:kelas;type:varchar(255);not null" json:"kelas"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

func (TeacherAttendanceSnapshot) TableName() string { return "teacher_attendance_snapshots" }

// SubstituteLog records history of substitute assignments
type SubstituteLog struct {
	ID                  uint           `gorm:"primaryKey" json:"id"`
	JadwalFormalID      *uint          `gorm:"column:jadwal_formal_id;index" json:"jadwal_formal_id"`
	JadwalDiniyyahID    *uint          `gorm:"column:jadwal_diniyyah_id;index" json:"jadwal_diniyyah_id"`
	OriginalTeacherID   uint           `gorm:"column:original_teacher_id;not null;index" json:"original_teacher_id"`
	SubstituteTeacherID uint           `gorm:"column:substitute_teacher_id;not null;index" json:"substitute_teacher_id"`
	Date                time.Time      `gorm:"type:date;not null" json:"date"`
	Status              string         `gorm:"type:varchar(50)" json:"status"` // Status of original teacher (Sakit/Izin/etc)
	Reason              string         `gorm:"type:text" json:"reason"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
	DeletedAt           gorm.DeletedAt `gorm:"index" json:"-"`

	// Relationships
	JadwalFormal      *Schedule         `gorm:"foreignKey:JadwalFormalID" json:"jadwal_formal,omitempty"`
	JadwalDiniyyah    *DiniyyahSchedule `gorm:"foreignKey:JadwalDiniyyahID" json:"jadwal_diniyyah,omitempty"`
	OriginalTeacher   User              `gorm:"foreignKey:OriginalTeacherID" json:"original_teacher"`
	SubstituteTeacher User              `gorm:"foreignKey:SubstituteTeacherID" json:"substitute_teacher"`
}

func (SubstituteLog) TableName() string { return "substitute_logs" }

// SubstituteLogSnapshot keeps immutable recap labels for substitute history,
// so records remain editable/readable even when schedule assignment labels change.
type SubstituteLogSnapshot struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	SubstituteLogID uint      `gorm:"column:substitute_log_id;uniqueIndex;not null" json:"substitute_log_id"`
	Lesson          string    `gorm:"column:lesson;type:varchar(255);not null" json:"lesson"`
	Kelas           string    `gorm:"column:kelas;type:varchar(255);not null" json:"kelas"`
	OriginalTeacher string    `gorm:"column:original_teacher;type:varchar(255);not null" json:"original_teacher"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (SubstituteLogSnapshot) TableName() string { return "substitute_log_snapshots" }

type SubstituteDiniyyahLog struct {
	ID                  uint      `gorm:"primaryKey" json:"id"`
	JadwalDiniyyahID    *uint     `gorm:"column:jadwal_diniyyah_id;index" json:"jadwal_diniyyah_id"`
	OriginalTeacherID   uint      `gorm:"column:original_teacher_id;not null;index" json:"original_teacher_id"`
	SubstituteTeacherID uint      `gorm:"column:substitute_teacher_id;not null;index" json:"substitute_teacher_id"`
	Date                time.Time `gorm:"type:date;not null" json:"date"`
	Status              string    `gorm:"type:varchar(50)" json:"status"`
	Reason              string    `gorm:"type:text" json:"reason"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`

	JadwalDiniyyah    *DiniyyahSchedule `gorm:"foreignKey:JadwalDiniyyahID" json:"jadwal_diniyyah,omitempty"`
	OriginalTeacher   User              `gorm:"foreignKey:OriginalTeacherID" json:"original_teacher"`
	SubstituteTeacher User              `gorm:"foreignKey:SubstituteTeacherID" json:"substitute_teacher"`
}

func (SubstituteDiniyyahLog) TableName() string { return "substitute_logs_diniyyah" }

type SubstituteDiniyyahLogSnapshot struct {
	ID                      uint      `gorm:"primaryKey" json:"id"`
	SubstituteDiniyyahLogID uint      `gorm:"column:substitute_diniyyah_log_id;uniqueIndex;not null" json:"substitute_diniyyah_log_id"`
	Lesson                  string    `gorm:"column:lesson;type:varchar(255);not null" json:"lesson"`
	Kelas                   string    `gorm:"column:kelas;type:varchar(255);not null" json:"kelas"`
	OriginalTeacher         string    `gorm:"column:original_teacher;type:varchar(255);not null" json:"original_teacher"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
}

func (SubstituteDiniyyahLogSnapshot) TableName() string { return "substitute_diniyyah_log_snapshots" }
