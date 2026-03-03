package models

import (
	"time"
)

// DiniyyahLesson represents a subject in the Diniyyah curriculum
type DiniyyahLesson struct {
	ID          uint                    `gorm:"primaryKey" json:"id"`
	Name        string                  `gorm:"column:nama;size:255;not null" json:"name"`
	CreatedAt   time.Time               `gorm:"column:created_at" json:"created_at"`
	UpdatedAt   time.Time               `gorm:"column:updated_at" json:"updated_at"`
	Assignments []DiniyyahLessonTeacher `gorm:"foreignKey:DiniyyahLessonID" json:"assignments"`
}

func (DiniyyahLesson) TableName() string { return "diniyyah_lessons" }
