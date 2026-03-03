package models

import (
	"time"

	"gorm.io/gorm"
)

type LessonType string

const (
	LessonTypeFormal   LessonType = "formal"
	LessonTypeDiniyyah LessonType = "diniyyah"
)

// Lesson represents a subject/mata pelajaran
type Lesson struct {
	ID          uint            `gorm:"primaryKey" json:"id"`
	Name        string          `gorm:"column:nama;size:255;not null" json:"name"`
	Type        LessonType      `gorm:"size:50;not null;index" json:"type"` // formal, diniyyah
	Code        *string         `gorm:"size:50;unique" json:"code"`         // e.g. MTK, ARB
	Description string          `gorm:"type:text" json:"description"`
	CreatedAt   time.Time       `gorm:"column:created_at" json:"created_at"`
	UpdatedAt   time.Time       `gorm:"column:updated_at" json:"updated_at"`
	DeletedAt   gorm.DeletedAt  `gorm:"column:deleted_at;index" json:"-"`
	Assignments []LessonTeacher `gorm:"foreignKey:LessonID" json:"assignments"`
}

func (Lesson) TableName() string { return "lessons" }
