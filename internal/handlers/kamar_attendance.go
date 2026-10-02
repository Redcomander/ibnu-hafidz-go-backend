package handlers

import (
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"gorm.io/gorm"
)

type KamarAttendanceHandler struct {
	db *gorm.DB
}

func NewKamarAttendanceHandler(db *gorm.DB) *KamarAttendanceHandler {
	return &KamarAttendanceHandler{db: db}
}

func (h *KamarAttendanceHandler) getKamarForAttendanceAccess(c *fiber.Ctx) (models.Kamar, error) {
	id := c.Params("id")
	var kamar models.Kamar
	if err := h.db.Preload("WaliKamar").Preload("SecondaryWalies").First(&kamar, id).Error; err != nil {
		return models.Kamar{}, fiber.NewError(fiber.StatusNotFound, "Kamar not found")
	}

	kamarHandler := NewKamarHandler(h.db)
	if err := kamarHandler.enforceKamarAccess(c, &kamar); err != nil {
		return models.Kamar{}, err
	}
	return kamar, nil
}

func (h *KamarAttendanceHandler) ListSessions(c *fiber.Ctx) error {
	_, err := h.getKamarForAttendanceAccess(c)
	if err != nil {
		return err
	}

	roomID := c.Params("id")
	var sessions []models.KamarAttendanceSession
	if err := h.db.Where("kamar_id = ?", roomID).Order("created_at desc").Find(&sessions).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to load room attendance sessions",
		})
	}

	return c.JSON(sessions)
}

func (h *KamarAttendanceHandler) CreateSession(c *fiber.Ctx) error {
	room, err := h.getKamarForAttendanceAccess(c)
	if err != nil {
		return err
	}

	type CreateSessionRequest struct {
		Name        string     `json:"name"`
		SessionType string     `json:"session_type"`
		WeekStart   *time.Time `json:"week_start_date"`
		Notes       string     `json:"notes"`
		StartsAt    *time.Time `json:"starts_at"`
		EndsAt      *time.Time `json:"ends_at"`
	}

	var req CreateSessionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "bad_request",
			Message: "Invalid request body",
		})
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "validation_error",
			Message: "Session name is required",
		})
	}

	sessionType := strings.TrimSpace(req.SessionType)
	if sessionType == "" {
		sessionType = "daily"
	}

	userIDValue, _ := c.Locals("userID").(uint)
	var userID *uint
	if userIDValue != 0 {
		userID = &userIDValue
	}

	notes := req.Notes
	var notesPtr *string
	if notes != "" {
		notesPtr = &notes
	}

	session := models.KamarAttendanceSession{
		KamarID:         room.ID,
		Name:            name,
		SessionType:     sessionType,
		WeekStartDate:   req.WeekStart,
		CreatedByUserID: userID,
		Notes:           notesPtr,
		StartsAt:        req.StartsAt,
		EndsAt:          req.EndsAt,
		IsActive:        true,
	}

	if err := h.db.Create(&session).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to create room attendance session",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(session)
}

func (h *KamarAttendanceHandler) GetSession(c *fiber.Ctx) error {
	room, err := h.getKamarForAttendanceAccess(c)
	if err != nil {
		return err
	}

	sessionID := c.Params("session_id")
	var session models.KamarAttendanceSession
	if err := h.db.Where("id = ? AND kamar_id = ?", sessionID, room.ID).Preload("Records.Student").First(&session).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "Attendance session not found",
		})
	}

	return c.JSON(session)
}

func (h *KamarAttendanceHandler) SubmitAttendance(c *fiber.Ctx) error {
	room, err := h.getKamarForAttendanceAccess(c)
	if err != nil {
		return err
	}

	sessionID := c.Params("session_id")
	var session models.KamarAttendanceSession
	if err := h.db.Where("id = ? AND kamar_id = ?", sessionID, room.ID).First(&session).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "Attendance session not found",
		})
	}

	type AttendanceRecordInput struct {
		StudentID uint   `json:"student_id"`
		Status    string `json:"status"`
		Notes     string `json:"notes"`
	}

	var req []AttendanceRecordInput
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "bad_request",
			Message: "Invalid attendance payload",
		})
	}

	if len(req) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "validation_error",
			Message: "No attendance records received",
		})
	}

	userIDValue, _ := c.Locals("userID").(uint)
	var userID *uint
	if userIDValue != 0 {
		userID = &userIDValue
	}

	created := 0
	updated := 0
	for _, item := range req {
		if item.StudentID == 0 {
			continue
		}

		var student models.Student
		if err := h.db.First(&student, item.StudentID).Error; err != nil {
			return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
				Error:   "not_found",
				Message: "One of the students was not found",
			})
		}

		status := strings.TrimSpace(strings.ToLower(item.Status))
		if status == "" {
			status = "hadir"
		}

		var record models.KamarAttendanceRecord
		if err := h.db.Where("kamar_attendance_session_id = ? AND student_id = ?", session.ID, student.ID).First(&record).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
					Error:   "server_error",
					Message: "Failed to save room attendance data",
				})
			}

			record = models.KamarAttendanceRecord{
				KamarAttendanceSessionID: session.ID,
				StudentID:                student.ID,
				Status:                   status,
				SubmittedBy:              userID,
			}
			if item.Notes != "" {
				notes := item.Notes
				record.Notes = &notes
			}
			if err := h.db.Create(&record).Error; err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
					Error:   "server_error",
					Message: "Failed to create room attendance record",
				})
			}
			created++
			continue
		}

		record.Status = status
		record.SubmittedBy = userID
		if item.Notes != "" {
			notes := item.Notes
			record.Notes = &notes
		} else {
			record.Notes = nil
		}
		if err := h.db.Save(&record).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
				Error:   "server_error",
				Message: "Failed to update room attendance record",
			})
		}
		updated++
	}

	return c.JSON(fiber.Map{
		"message": "Room attendance saved successfully",
		"created": created,
		"updated": updated,
	})
}
