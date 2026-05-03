package models

import (
	"time"
)

// ──────────────────────────────────────────────────────────────
// HalaqohAssignment — matches Laravel `halaqoh_assignments` table
// 1-to-1 teacher→student assignment for Quran memorization
// ──────────────────────────────────────────────────────────────
type HalaqohAssignment struct {
	ID                  uint       `gorm:"primaryKey" json:"id"`
	UserID              uint       `gorm:"column:user_id;not null;index" json:"user_id"`                    // Main teacher
	StudentID           uint       `gorm:"column:student_id;not null;index" json:"student_id"`              // Assigned student
	HelperTeacherID     *uint      `gorm:"column:helper_teacher_id;index" json:"helper_teacher_id"`         // Optional helper teacher
	SubstituteTeacherID *uint      `gorm:"column:substitute_teacher_id;index" json:"substitute_teacher_id"` // Legacy substitute
	SubstituteDate      *time.Time `gorm:"column:substitute_date;type:date" json:"substitute_date"`         // Legacy
	SubstituteStatus    *string    `gorm:"column:substitute_status;size:50" json:"substitute_status"`       // Legacy
	SubstituteReason    *string    `gorm:"column:substitute_reason;size:500" json:"substitute_reason"`      // Legacy
	SubstituteSession   *string    `gorm:"column:substitute_session;size:50" json:"substitute_session"`     // Legacy
	Active              bool       `gorm:"column:active;default:true;not null" json:"active"`               // Soft state
	CreatedAt           time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt           time.Time  `gorm:"column:updated_at" json:"updated_at"`

	// Relationships
	Teacher           User    `gorm:"foreignKey:UserID" json:"teacher,omitempty"`
	Student           Student `gorm:"foreignKey:StudentID" json:"student,omitempty"`
	HelperTeacher     *User   `gorm:"foreignKey:HelperTeacherID" json:"helper_teacher,omitempty"`
	SubstituteTeacher *User   `gorm:"foreignKey:SubstituteTeacherID" json:"substitute_teacher,omitempty"`

	// Dynamic fields for frontend
	HasAttendanceToday map[string]bool `gorm:"-" json:"has_attendance_today,omitempty"` // per-session
}

func (HalaqohAssignment) TableName() string { return "halaqoh_assignments" }

// ──────────────────────────────────────────────────────────────
// HalaqohAttendance — matches Laravel `halaqoh_attendances` table
// Student attendance per session per day
// ──────────────────────────────────────────────────────────────
type HalaqohAttendance struct {
	ID                  uint      `gorm:"primaryKey" json:"id"`
	HalaqohAssignmentID uint      `gorm:"column:halaqoh_assignment_id;not null;index" json:"halaqoh_assignment_id"`
	StudentID           *uint     `gorm:"column:student_id;index" json:"student_id"`
	Date                time.Time `gorm:"column:date;type:date;not null" json:"date"`
	Session             string    `gorm:"column:session;type:enum('Shubuh','Ashar','Isya');not null" json:"session"`
	Status              string    `gorm:"column:status;type:enum('hadir','izin','sakit','alpa');default:'hadir'" json:"status"`
	Notes               *string   `gorm:"column:notes;type:text" json:"notes"`
	SubmittedBy         *uint     `gorm:"column:submitted_by;index" json:"submitted_by"`
	CreatedAt           time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt           time.Time `gorm:"column:updated_at" json:"updated_at"`

	// Relationships
	HalaqohAssignment HalaqohAssignment          `gorm:"foreignKey:HalaqohAssignmentID" json:"halaqoh_assignment,omitempty"`
	Student           *Student                   `gorm:"foreignKey:StudentID" json:"student,omitempty"`
	Submitter         *User                      `gorm:"foreignKey:SubmittedBy" json:"submitter,omitempty"`
	Snapshot          *HalaqohAttendanceSnapshot `gorm:"foreignKey:HalaqohAttendanceID" json:"snapshot,omitempty"`
}

func (HalaqohAttendance) TableName() string { return "halaqoh_attendances" }

// HalaqohAttendanceSnapshot stores immutable labels at submit-time.
// This keeps history stable if assignment teacher/student mapping changes later.
type HalaqohAttendanceSnapshot struct {
	ID                  uint      `gorm:"primaryKey" json:"id"`
	HalaqohAttendanceID uint      `gorm:"column:halaqoh_attendance_id;uniqueIndex;not null" json:"halaqoh_attendance_id"`
	TeacherName         string    `gorm:"column:teacher_name;type:varchar(255);not null" json:"teacher_name"`
	StudentName         string    `gorm:"column:student_name;type:varchar(255);not null" json:"student_name"`
	CreatedAt           time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt           time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (HalaqohAttendanceSnapshot) TableName() string { return "halaqoh_attendance_snapshots" }

// ──────────────────────────────────────────────────────────────
// HalaqohTeacherAttendance — matches Laravel `halaqoh_teacher_attendances` table
// Teacher self-attendance per session
// ──────────────────────────────────────────────────────────────
type HalaqohTeacherAttendance struct {
	ID                     uint      `gorm:"primaryKey" json:"id"`
	UserID                 uint      `gorm:"column:user_id;not null;index" json:"user_id"` // Who actually attended
	TeacherID              *uint     `gorm:"column:teacher_id;index" json:"teacher_id"`    // Main teacher ref
	Date                   time.Time `gorm:"column:date;type:date;not null" json:"date"`
	Session                string    `gorm:"column:session;size:50;not null" json:"session"` // Shubuh, Ashar, Isya
	Status                 string    `gorm:"column:status;size:50;not null" json:"status"`   // Hadir, Izin, Sakit, Alpha
	Notes                  *string   `gorm:"column:notes;type:text" json:"notes"`
	PhotoPath              *string   `gorm:"column:photo_path;size:255" json:"photo_path"`
	SubmittedBy            *uint     `gorm:"column:submitted_by;index" json:"submitted_by"`
	HalaqohSubstituteLogID *uint     `gorm:"column:halaqoh_substitute_log_id;index" json:"halaqoh_substitute_log_id"`
	CreatedAt              time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt              time.Time `gorm:"column:updated_at" json:"updated_at"`

	// Relationships
	Teacher       User                  `gorm:"foreignKey:UserID" json:"teacher,omitempty"`
	SubstituteLog *HalaqohSubstituteLog `gorm:"foreignKey:HalaqohSubstituteLogID" json:"substitute_log,omitempty"`
}

func (HalaqohTeacherAttendance) TableName() string { return "halaqoh_teacher_attendances" }

// ──────────────────────────────────────────────────────────────
// HalaqohSubstituteLog — matches Laravel `halaqoh_substitute_logs` table
// Per-session substitute tracking
// ──────────────────────────────────────────────────────────────
type HalaqohSubstituteLog struct {
	ID                  uint      `gorm:"primaryKey" json:"id"`
	HalaqohAssignmentID *uint     `gorm:"column:halaqoh_assignment_id;index" json:"halaqoh_assignment_id"`
	OriginalTeacherID   uint      `gorm:"column:original_teacher_id;not null;index" json:"original_teacher_id"`
	SubstituteTeacherID uint      `gorm:"column:substitute_teacher_id;not null;index" json:"substitute_teacher_id"`
	Date                time.Time `gorm:"column:date;type:date;not null" json:"date"`
	Session             *string   `gorm:"column:session;size:50" json:"session"` // Shubuh, Ashar, Isya
	Status              *string   `gorm:"column:status;type:enum('Hadir','Izin','Sakit','Alpha')" json:"status"`
	Reason              *string   `gorm:"column:reason;size:500" json:"reason"`
	IsActive            bool      `gorm:"column:is_active;default:true" json:"is_active"`
	CreatedAt           time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt           time.Time `gorm:"column:updated_at" json:"updated_at"`

	// Relationships
	Assignment        *HalaqohAssignment `gorm:"foreignKey:HalaqohAssignmentID" json:"assignment,omitempty"`
	OriginalTeacher   User               `gorm:"foreignKey:OriginalTeacherID" json:"original_teacher,omitempty"`
	SubstituteTeacher User               `gorm:"foreignKey:SubstituteTeacherID" json:"substitute_teacher,omitempty"`
}

func (HalaqohSubstituteLog) TableName() string { return "halaqoh_substitute_logs" }
