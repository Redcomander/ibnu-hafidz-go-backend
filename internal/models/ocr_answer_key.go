package models

import (
	"time"

	"gorm.io/gorm"
)

// OcrAnswerKey stores a named set of correct answers used for OCR scoring.
// It is the persistent source of truth so answer keys survive OCR service resets.
type OcrAnswerKey struct {
	ID uint `gorm:"primaryKey" json:"id"`

	// Name is a human-readable label for the answer key (e.g. "Ujian Matematika Kelas 8").
	Name string `gorm:"column:name;size:255;not null" json:"name"`

	// Answers is a JSON object string mapping question number → correct option.
	// Example: {"1":"A","2":"B","3":"C"}
	Answers string `gorm:"column:answers;type:longtext;not null" json:"answers"`

	// TotalCount is the number of questions covered by this key.
	TotalCount int `gorm:"column:total_count;default:0" json:"total_count"`

	CreatedAt time.Time      `gorm:"column:created_at" json:"created_at"`
	UpdatedAt time.Time      `gorm:"column:updated_at" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;index" json:"-"`
}

func (OcrAnswerKey) TableName() string { return "ocr_answer_keys" }
