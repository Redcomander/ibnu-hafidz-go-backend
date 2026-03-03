package models

import (
	"time"
)

// DiniyyahLessonTeacher represents the assignment of a teacher to a diniyyah lesson in a specific class
type DiniyyahLessonTeacher struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	DiniyyahLessonID uint      `gorm:"column:diniyyah_lesson_id;not null" json:"diniyyah_lesson_id"`
	KelasID          uint      `gorm:"column:kelas_id;not null" json:"kelas_id"`
	UserID           uint      `gorm:"column:user_id;not null" json:"user_id"`
	CreatedAt        time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt        time.Time `gorm:"column:updated_at" json:"updated_at"`

	// Relationships
	DiniyyahLesson *DiniyyahLesson `gorm:"foreignKey:DiniyyahLessonID" json:"diniyyah_lesson,omitempty"`
	Kelas          *Kelas          `gorm:"foreignKey:KelasID" json:"kelas,omitempty"`
	Teacher        *User           `gorm:"foreignKey:UserID" json:"teacher,omitempty"`
}

func (DiniyyahLessonTeacher) TableName() string { return "diniyyah_kelas_teachers" }
