package handlers

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"gorm.io/gorm"
)

type AbsensiEkstraSessionHandler struct {
	db *gorm.DB
}

func NewAbsensiEkstraSessionHandler(db *gorm.DB) *AbsensiEkstraSessionHandler {
	return &AbsensiEkstraSessionHandler{db: db}
}

// List returns all sessions for a group
func (h *AbsensiEkstraSessionHandler) List(c *fiber.Ctx) error {
	groupID := c.Params("group_id")

	page := c.QueryInt("page", 1)
	perPage := c.QueryInt("per_page", 15)
	status := c.Query("status")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	query := h.db.Model(&models.AbsensiEkstraSession{}).
		Where("absensi_ekstra_group_id = ?", groupID).
		Preload("CreatedBy")

	if status != "" && status != "all" {
		query = query.Where("status = ?", status)
	}
	if startDate != "" {
		query = query.Where("tanggal >= ?", startDate)
	}
	if endDate != "" {
		query = query.Where("tanggal <= ?", endDate)
	}

	var total int64
	query.Count(&total)

	var sessions []models.AbsensiEkstraSession
	offset := (page - 1) * perPage
	query.Order("tanggal desc, created_at desc").Limit(perPage).Offset(offset).Find(&sessions)

	// Add record counts to each session
	type SessionWithStats struct {
		models.AbsensiEkstraSession
		RecordCount int64                  `json:"record_count"`
		Statistics  map[string]interface{} `json:"statistics"`
	}

	var result []SessionWithStats
	for _, s := range sessions {
		var count int64
		h.db.Model(&models.AbsensiEkstraSessionRecord{}).
			Where("absensi_ekstra_session_id = ?", s.ID).
			Count(&count)
		stats := s.GetStatistics(h.db)
		result = append(result, SessionWithStats{
			AbsensiEkstraSession: s,
			RecordCount:          count,
			Statistics:           stats,
		})
	}

	return c.JSON(fiber.Map{
		"data":        result,
		"total":       total,
		"page":        page,
		"per_page":    perPage,
		"total_pages": (total + int64(perPage) - 1) / int64(perPage),
	})
}

// Get returns a single session with records
func (h *AbsensiEkstraSessionHandler) Get(c *fiber.Ctx) error {
	sessionID := c.Params("session_id")

	var session models.AbsensiEkstraSession
	if err := h.db.
		Preload("CreatedBy").
		Preload("Records.Student").
		Preload("Records.SubmittedBy").
		Preload("Group").
		First(&session, sessionID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "Session not found",
		})
	}

	statistics := session.GetStatistics(h.db)

	// Check whether current user can edit
	userID := c.Locals("userID").(uint)
	canEdit := session.Status == "open"

	// Check if user is group admin
	var isGroupAdmin int64
	h.db.Model(&models.AbsensiEkstraGroupSupervisor{}).
		Where("absensi_ekstra_group_id = ? AND user_id = ? AND role = ?", session.AbsensiEkstraGroupID, userID, "admin").
		Count(&isGroupAdmin)
	if isGroupAdmin > 0 {
		canEdit = true
	}

	// Check global permission
	var fullUser models.User
	h.db.Preload("Roles.Permissions").First(&fullUser, userID)
	if fullUser.HasPermission("absensi_ekstra.view_all") {
		canEdit = true
	}

	return c.JSON(fiber.Map{
		"session":    session,
		"statistics": statistics,
		"can_edit":   canEdit,
	})
}

// Create creates a new session for a group
func (h *AbsensiEkstraSessionHandler) Create(c *fiber.Ctx) error {
	groupID := c.Params("group_id")
	userID := c.Locals("userID").(uint)

	var group models.AbsensiEkstraGroup
	if err := h.db.First(&group, groupID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "Group not found",
		})
	}

	// Check group has students
	var studentCount int64
	h.db.Model(&models.AbsensiEkstraGroupStudent{}).
		Where("absensi_ekstra_group_id = ? AND tanggal_keluar IS NULL", group.ID).
		Count(&studentCount)

	if studentCount == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "validation_error",
			Message: "Grup ini belum memiliki santri. Tambahkan santri terlebih dahulu.",
		})
	}

	type CreateSessionInput struct {
		NamaSesi string `json:"nama_sesi"`
		Tanggal  string `json:"tanggal"`
	}

	var input CreateSessionInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "bad_request",
			Message: "Invalid input",
		})
	}

	if input.NamaSesi == "" || input.Tanggal == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "validation_error",
			Message: "nama_sesi and tanggal are required",
		})
	}

	tanggal, err := time.Parse("2006-01-02", input.Tanggal)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "validation_error",
			Message: "Invalid date format (expected YYYY-MM-DD)",
		})
	}

	session := models.AbsensiEkstraSession{
		AbsensiEkstraGroupID: group.ID,
		NamaSesi:             input.NamaSesi,
		Tanggal:              tanggal,
		Status:               "open",
		CreatedByID:          userID,
	}

	if err := h.db.Create(&session).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to create session: " + err.Error(),
		})
	}

	// Pre-fill records with 'alpha' for all active students in the group
	var studentIDs []uint
	h.db.Model(&models.AbsensiEkstraGroupStudent{}).
		Where("absensi_ekstra_group_id = ? AND tanggal_keluar IS NULL", group.ID).
		Pluck("student_id", &studentIDs)

	for _, sid := range studentIDs {
		h.db.Create(&models.AbsensiEkstraSessionRecord{
			AbsensiEkstraSessionID: session.ID,
			StudentID:              sid,
			Kehadiran:              "alpha",
			SubmittedByID:          userID,
		})
	}

	h.db.Preload("CreatedBy").Preload("Records.Student").First(&session, session.ID)

	return c.Status(fiber.StatusCreated).JSON(session)
}

// SubmitAttendance batch-updates attendance records
func (h *AbsensiEkstraSessionHandler) SubmitAttendance(c *fiber.Ctx) error {
	sessionID := c.Params("session_id")
	userID := c.Locals("userID").(uint)

	var session models.AbsensiEkstraSession
	if err := h.db.First(&session, sessionID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "Session not found",
		})
	}

	type AttendanceRecord struct {
		StudentID  uint   `json:"student_id"`
		Kehadiran  string `json:"kehadiran"`
		Keterangan string `json:"keterangan"`
	}
	type SubmitInput struct {
		Attendance []AttendanceRecord `json:"attendance"`
	}

	var input SubmitInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "bad_request",
			Message: "Invalid input",
		})
	}

	// Update or create each record
	for _, rec := range input.Attendance {
		validStatuses := map[string]bool{
			"hadir": true, "izin": true, "sakit": true, "alpha": true, "terlambat": true,
		}
		if !validStatuses[rec.Kehadiran] {
			continue
		}

		var existing models.AbsensiEkstraSessionRecord
		result := h.db.Where("absensi_ekstra_session_id = ? AND student_id = ?", session.ID, rec.StudentID).
			First(&existing)

		if result.RowsAffected > 0 {
			existing.Kehadiran = rec.Kehadiran
			existing.Keterangan = rec.Keterangan
			existing.SubmittedByID = userID
			h.db.Save(&existing)
		} else {
			h.db.Create(&models.AbsensiEkstraSessionRecord{
				AbsensiEkstraSessionID: session.ID,
				StudentID:              rec.StudentID,
				Kehadiran:              rec.Kehadiran,
				Keterangan:             rec.Keterangan,
				SubmittedByID:          userID,
			})
		}
	}

	return c.JSON(fiber.Map{"message": "Data kehadiran berhasil disimpan"})
}

// Close closes a session
func (h *AbsensiEkstraSessionHandler) Close(c *fiber.Ctx) error {
	sessionID := c.Params("session_id")

	var session models.AbsensiEkstraSession
	if err := h.db.First(&session, sessionID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "Session not found",
		})
	}

	session.Status = "closed"
	h.db.Save(&session)

	return c.JSON(fiber.Map{"message": "Sesi berhasil ditutup"})
}

// Delete deletes a session (and its records)
func (h *AbsensiEkstraSessionHandler) Delete(c *fiber.Ctx) error {
	sessionID := c.Params("session_id")

	var session models.AbsensiEkstraSession
	if err := h.db.First(&session, sessionID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "Session not found",
		})
	}

	// Delete records first
	h.db.Where("absensi_ekstra_session_id = ?", session.ID).Delete(&models.AbsensiEkstraSessionRecord{})
	h.db.Delete(&session)

	return c.JSON(fiber.Map{"message": "Sesi berhasil dihapus"})
}

// Statistics returns attendance statistics for a group over a date range
func (h *AbsensiEkstraSessionHandler) Statistics(c *fiber.Ctx) error {
	groupID := c.Params("group_id")
	startDate := c.Query("start_date", time.Now().AddDate(0, -1, 0).Format("2006-01-02"))
	endDate := c.Query("end_date", time.Now().Format("2006-01-02"))

	// Get sessions in date range
	var sessions []models.AbsensiEkstraSession
	h.db.Where("absensi_ekstra_group_id = ? AND tanggal BETWEEN ? AND ?", groupID, startDate, endDate).
		Preload("Records.Student").
		Order("tanggal desc").
		Find(&sessions)

	// Get active students from group
	var students []models.Student
	h.db.Raw(`
		SELECT s.* FROM students s
		JOIN absensi_ekstra_group_students gs ON gs.student_id = s.id
		WHERE gs.absensi_ekstra_group_id = ? AND gs.tanggal_keluar IS NULL
		ORDER BY s.nama_lengkap
	`, groupID).Scan(&students)

	// Calculate per-student statistics
	sessionIDs := make([]uint, len(sessions))
	for i, s := range sessions {
		sessionIDs[i] = s.ID
	}

	type StudentStat struct {
		Student    models.Student `json:"student"`
		Total      int64          `json:"total"`
		Hadir      int64          `json:"hadir"`
		Izin       int64          `json:"izin"`
		Sakit      int64          `json:"sakit"`
		Alpha      int64          `json:"alpha"`
		Terlambat  int64          `json:"terlambat"`
		Percentage float64        `json:"percentage"`
	}

	var studentStats []StudentStat

	for _, student := range students {
		var total, hadir, izin, sakit, alpha, terlambat int64

		if len(sessionIDs) > 0 {
			h.db.Model(&models.AbsensiEkstraSessionRecord{}).
				Where("student_id = ? AND absensi_ekstra_session_id IN ?", student.ID, sessionIDs).
				Count(&total)
			h.db.Model(&models.AbsensiEkstraSessionRecord{}).
				Where("student_id = ? AND absensi_ekstra_session_id IN ? AND kehadiran = ?", student.ID, sessionIDs, "hadir").
				Count(&hadir)
			h.db.Model(&models.AbsensiEkstraSessionRecord{}).
				Where("student_id = ? AND absensi_ekstra_session_id IN ? AND kehadiran = ?", student.ID, sessionIDs, "izin").
				Count(&izin)
			h.db.Model(&models.AbsensiEkstraSessionRecord{}).
				Where("student_id = ? AND absensi_ekstra_session_id IN ? AND kehadiran = ?", student.ID, sessionIDs, "sakit").
				Count(&sakit)
			h.db.Model(&models.AbsensiEkstraSessionRecord{}).
				Where("student_id = ? AND absensi_ekstra_session_id IN ? AND kehadiran = ?", student.ID, sessionIDs, "alpha").
				Count(&alpha)
			h.db.Model(&models.AbsensiEkstraSessionRecord{}).
				Where("student_id = ? AND absensi_ekstra_session_id IN ? AND kehadiran = ?", student.ID, sessionIDs, "terlambat").
				Count(&terlambat)
		}

		pct := float64(0)
		if total > 0 {
			pct = float64(hadir) / float64(total) * 100
		}

		studentStats = append(studentStats, StudentStat{
			Student:    student,
			Total:      total,
			Hadir:      hadir,
			Izin:       izin,
			Sakit:      sakit,
			Alpha:      alpha,
			Terlambat:  terlambat,
			Percentage: pct,
		})
	}

	return c.JSON(fiber.Map{
		"sessions":       sessions,
		"student_stats":  studentStats,
		"total_sessions": len(sessions),
		"start_date":     startDate,
		"end_date":       endDate,
	})
}

// ExportPDF exports attendance statistics as PDF
func (h *AbsensiEkstraSessionHandler) ExportPDF(c *fiber.Ctx) error {
	groupID := c.Params("group_id")
	startDate := c.Query("start_date", time.Now().AddDate(0, -1, 0).Format("2006-01-02"))
	endDate := c.Query("end_date", time.Now().Format("2006-01-02"))

	var group models.AbsensiEkstraGroup
	if err := h.db.First(&group, groupID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "Group not found",
		})
	}

	// For now, return a placeholder
	_ = startDate
	_ = endDate

	return c.JSON(fiber.Map{
		"message": fmt.Sprintf("PDF export for group '%s' is not yet implemented. Coming soon!", group.NamaGrup),
	})
}
