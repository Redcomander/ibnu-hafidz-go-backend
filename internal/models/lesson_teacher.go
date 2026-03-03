package models

import (
	"time"
)

// LessonTeacher represents the assignment of a teacher to a formal lesson in a specific class
type LessonTeacher struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	LessonID  uint      `gorm:"column:lesson_id;not null" json:"lesson_id"`
	KelasID   uint      `gorm:"column:kelas_id;not null" json:"kelas_id"`
	UserID    uint      `gorm:"column:user_id;not null" json:"user_id"`
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at" json:"updated_at"`

	// Relationships
	Lesson  *Lesson `gorm:"foreignKey:LessonID" json:"lesson,omitempty"`
	Kelas   *Kelas  `gorm:"foreignKey:KelasID" json:"kelas,omitempty"`
	Teacher *User   `gorm:"foreignKey:UserID" json:"teacher,omitempty"` // Mapped to user_id
}

func (LessonTeacher) TableName() string { return "lesson_kelas_teachers" }
