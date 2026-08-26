package models

import (
	"time"
)

// HalaqohAssignmentChangeRequest records a teacher-initiated change to their own halaqoh assignment list.
// Admin/super_admin can approve or reject the request; normal teachers cannot mutate live assignment data directly.
type HalaqohAssignmentChangeRequest struct {
	ID                    uint       `gorm:"primaryKey" json:"id"`
	TeacherID             uint       `gorm:"column:teacher_id;not null;index" json:"teacher_id"`
	RequestedByUserID     uint       `gorm:"column:requested_by_user_id;not null;index" json:"requested_by_user_id"`
	RequestType           string     `gorm:"column:request_type;size:50;not null" json:"request_type"`
	TargetTeacherID       uint       `gorm:"column:target_teacher_id;not null;index" json:"target_teacher_id"`
	StudentIDs            []uint     `gorm:"column:student_ids;serializer:json;type:json" json:"student_ids,omitempty"`
	PreviousStudentIDs    []uint     `gorm:"column:previous_student_ids;serializer:json;type:json" json:"previous_student_ids,omitempty"`
	HelperTeacherID       *uint      `gorm:"column:helper_teacher_id;index" json:"helper_teacher_id,omitempty"`
	PreviousHelperTeacher *uint      `gorm:"column:previous_helper_teacher_id;index" json:"previous_helper_teacher_id,omitempty"`
	Status                string     `gorm:"column:status;size:20;default:'pending';not null" json:"status"`
	Reason                string     `gorm:"column:reason;type:text" json:"reason,omitempty"`
	ReviewedBy            *uint      `gorm:"column:reviewed_by;index" json:"reviewed_by,omitempty"`
	ReviewedAt            *time.Time `gorm:"column:reviewed_at" json:"reviewed_at,omitempty"`
	CreatedAt             time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt             time.Time  `gorm:"column:updated_at" json:"updated_at"`

	Teacher         User      `gorm:"foreignKey:TeacherID" json:"teacher,omitempty"`
	RequestedBy     User      `gorm:"foreignKey:RequestedByUserID" json:"requested_by,omitempty"`
	Reviewer        *User     `gorm:"foreignKey:ReviewedBy" json:"reviewer,omitempty"`
	StudentDetails  []Student `gorm:"-" json:"student_details,omitempty"`
	RequestedByName string    `gorm:"-" json:"requested_by_name,omitempty"`
}

func (HalaqohAssignmentChangeRequest) TableName() string {
	return "halaqoh_assignment_change_requests"
}
