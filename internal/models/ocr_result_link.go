package models

import (
	"time"

	"gorm.io/gorm"
)

// OCRResultLink stores an OCR scan outcome with academic context mapping.
type OCRResultLink struct {
	ID uint `gorm:"primaryKey" json:"id"`

	Source   string   `gorm:"column:source;size:30;index;not null" json:"source"`
	Filename *string  `gorm:"column:filename;size:255" json:"filename,omitempty"`
	Score    *float64 `gorm:"column:score" json:"score,omitempty"`
	Correct  *int     `gorm:"column:correct_count" json:"correct,omitempty"`
	Wrong    *int     `gorm:"column:wrong_count" json:"wrong,omitempty"`
	Blank    *int     `gorm:"column:blank_count" json:"blank,omitempty"`

	RawResult *string `gorm:"column:raw_result;type:longtext" json:"raw_result,omitempty"`

	LessonType *string `gorm:"column:lesson_type;size:30" json:"lesson_type,omitempty"`
	LessonID   *uint   `gorm:"column:lesson_id;index" json:"lesson_id,omitempty"`
	KelasID    *uint   `gorm:"column:kelas_id;index" json:"kelas_id,omitempty"`
	TeacherID  *uint   `gorm:"column:teacher_id;index" json:"teacher_id,omitempty"`
	StudentID  *uint   `gorm:"column:student_id;index" json:"student_id,omitempty"`

	AnswerKeyID *uint `gorm:"column:answer_key_id;index" json:"answer_key_id,omitempty"`

	ScannedByID uint  `gorm:"column:scanned_by_id;index;not null" json:"scanned_by_id"`
	ScannedBy   *User `gorm:"foreignKey:ScannedByID" json:"scanned_by,omitempty"`

	Lesson  *Lesson  `gorm:"foreignKey:LessonID" json:"lesson,omitempty"`
	Kelas   *Kelas   `gorm:"foreignKey:KelasID" json:"kelas,omitempty"`
	Teacher *User    `gorm:"foreignKey:TeacherID" json:"teacher,omitempty"`
	Student *Student `gorm:"foreignKey:StudentID" json:"student,omitempty"`

	CreatedAt time.Time      `gorm:"column:created_at" json:"created_at"`
	UpdatedAt time.Time      `gorm:"column:updated_at" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;index" json:"-"`
}

func (OCRResultLink) TableName() string { return "ocr_result_links" }
