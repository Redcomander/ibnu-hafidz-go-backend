package handlers

import (
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"gorm.io/gorm"
)

type AbsensiHandler struct {
	db *gorm.DB
}

type assignableTeacherResponse struct {
	ID    uint          `json:"id"`
	Name  string        `json:"name"`
	Email string        `json:"email"`
	Roles []models.Role `json:"roles,omitempty"`
}

func NewAbsensiHandler(db *gorm.DB) *AbsensiHandler {
	return &AbsensiHandler{db: db}
}

func (h *AbsensiHandler) getUserFromContext(c *fiber.Ctx) (*models.User, error) {
	if u, ok := c.Locals("user").(*models.User); ok && u != nil {
		return u, nil
	}

	userID, ok := c.Locals("userID").(uint)
	if !ok || userID == 0 {
		return nil, fiber.NewError(fiber.StatusUnauthorized, "User not authenticated")
	}

	var user models.User
	if err := h.db.Preload("Roles.Permissions").First(&user, userID).Error; err != nil {
		return nil, fiber.NewError(fiber.StatusUnauthorized, "User not found")
	}

	return &user, nil
}

func canManageTeacherAttendance(user *models.User) bool {
	return user.HasRole("super_admin") ||
		user.HasRole("admin") ||
		user.HasRole("staff") ||
		user.HasRole("tim_presensi") ||
		user.HasPermission("schedule.substitute")
}

func canAccessScheduledAttendance(user *models.User, originalTeacherID uint, substituteTeacherID *uint, substituteDate *time.Time, targetDate string) bool {
	if user == nil {
		return false
	}
	if canManageTeacherAttendance(user) || user.ID == originalTeacherID {
		return true
	}
	if substituteTeacherID != nil && user.ID == *substituteTeacherID {
		if substituteDate == nil {
			return true
		}
		return substituteDate.Format("2006-01-02") == targetDate
	}
	return false
}

// ListAssignableTeachers returns all active users that can be selected as substitute teachers.
func (h *AbsensiHandler) ListAssignableTeachers(c *fiber.Ctx) error {
	user, err := h.getUserFromContext(c)
	if err != nil {
		return err
	}
	if !canManageTeacherAttendance(user) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Not authorized to assign substitute teachers"})
	}

	search := strings.TrimSpace(c.Query("search"))
	page := c.QueryInt("page", 1)
	if page < 1 {
		page = 1
	}
	perPage := c.QueryInt("per_page", 20)
	if perPage < 1 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}

	idBaseQuery := h.db.Model(&models.User{}).
		Where("users.deleted_at IS NULL")

	if search != "" {
		like := "%" + search + "%"
		idBaseQuery = idBaseQuery.Where("users.name LIKE ? OR users.email LIKE ?", like, like)
	}

	idBaseQuery = idBaseQuery.Select("users.id, users.name").Distinct()

	var total int64
	if err := h.db.Table("(?) as assignable_ids", idBaseQuery).Count(&total).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to count assignable teachers"})
	}

	offset := (page - 1) * perPage
	type assignableTeacherRow struct {
		ID   uint
		Name string
	}
	var rows []assignableTeacherRow
	if err := idBaseQuery.
		Order("users.name ASC").
		Limit(perPage).
		Offset(offset).
		Find(&rows).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch assignable teachers"})
	}

	ids := make([]uint, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}

	if len(ids) == 0 {
		return c.JSON(BuildPaginatedResponse([]assignableTeacherResponse{}, total, page, perPage))
	}

	var users []models.User
	if err := h.db.Model(&models.User{}).
		Where("users.id IN ?", ids).
		Preload("Roles").
		Find(&users).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load assignable teacher details"})
	}

	byID := make(map[uint]models.User, len(users))
	for _, u := range users {
		byID[u.ID] = u
	}

	result := make([]assignableTeacherResponse, 0, len(ids))
	for _, id := range ids {
		u, ok := byID[id]
		if !ok {
			continue
		}
		result = append(result, assignableTeacherResponse{
			ID:    u.ID,
			Name:  u.Name,
			Email: u.Email,
			Roles: u.Roles,
		})
	}

	return c.JSON(BuildPaginatedResponse(result, total, page, perPage))
}

// dateRange returns start (inclusive) and end (exclusive) for a date string.
// This avoids using DATE() function which prevents index usage.
func dateRange(dateStr string) (string, string) {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		t = time.Now()
	}
	start := t.Format("2006-01-02")
	end := t.AddDate(0, 0, 1).Format("2006-01-02")
	return start, end
}

func isDiniyyahAttendanceType(typeStr string) bool {
	return strings.EqualFold(strings.TrimSpace(typeStr), "diniyyah")
}

func isRamadhanAttendanceType(typeStr string) bool {
	v := strings.TrimSpace(strings.ToLower(typeStr))
	return v == "ramadhan" || v == "ramadan"
}

func applyFormalScheduleTypeFilter(db *gorm.DB, scheduleAlias string, typeStr string) *gorm.DB {
	if isRamadhanAttendanceType(typeStr) {
		return db.Where(scheduleAlias+".type = ?", "ramadhan")
	}
	return db.Where(scheduleAlias + ".type IS NULL OR " + scheduleAlias + ".type = '' OR " + scheduleAlias + ".type = 'normal' OR " + scheduleAlias + ".type = 'formal'")
}

// GetAttendance retrieves the attendance form for a specific schedule and date
func (h *AbsensiHandler) GetAttendance(c *fiber.Ctx) error {
	user, err := h.getUserFromContext(c)
	if err != nil {
		return err
	}

	jadwalID := c.Query("jadwal_id")
	dateStr := c.Query("date")
	typeStr := c.Query("type", "formal") // formal or diniyyah

	if jadwalID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Jadwal ID is required"})
	}

	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		date = time.Now()
		dateStr = date.Format("2006-01-02")
	}

	// 1. Fetch Schedule with relationships
	var students []models.Student
	var assignedTeacherID uint
	var substituteTeacherID *uint
	var substituteDate *time.Time

	if typeStr == "diniyyah" {
		var jadwal models.DiniyyahSchedule
		if err := h.db.Preload("Assignment.Kelas.Students").Preload("Assignment.Teacher").Preload("SubstituteTeacher").First(&jadwal, jadwalID).Error; err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Schedule not found"})
		}
		students = jadwal.Assignment.Kelas.Students
		assignedTeacherID = jadwal.Assignment.UserID
		substituteTeacherID = jadwal.SubstituteTeacherID
		substituteDate = jadwal.SubstituteDate
	} else {
		var jadwal models.Schedule
		if err := h.db.Preload("Assignment.Kelas.Students").Preload("Assignment.Teacher").Preload("SubstituteTeacher").First(&jadwal, jadwalID).Error; err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Schedule not found"})
		}
		students = jadwal.Assignment.Kelas.Students
		assignedTeacherID = jadwal.Assignment.UserID
		substituteTeacherID = jadwal.SubstituteTeacherID
		substituteDate = jadwal.SubstituteDate
	}

	if !canAccessScheduledAttendance(user, assignedTeacherID, substituteTeacherID, substituteDate, dateStr) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Anda tidak memiliki akses ke jadwal ini"})
	}

	// 2. Fetch Existing Attendance — use date range instead of DATE()
	type AttendanceResponse struct {
		StudentID uint   `json:"student_id"`
		Status    string `json:"status"`
		Catatan   string `json:"catatan"`
	}
	existingMap := make(map[uint]AttendanceResponse)
	start, end := dateRange(dateStr)

	if typeStr == "diniyyah" {
		var records []models.AbsensiDiniyyah
		h.db.Where("jadwal_diniyyah_id = ? AND tanggal >= ? AND tanggal < ?", jadwalID, start, end).Find(&records)
		for _, r := range records {
			existingMap[r.StudentID] = AttendanceResponse{StudentID: r.StudentID, Status: r.Status, Catatan: r.Catatan}
		}
	} else {
		var records []models.Absensi
		h.db.Where("jadwal_formal_id = ? AND tanggal >= ? AND tanggal < ?", jadwalID, start, end).Find(&records)
		for _, r := range records {
			existingMap[r.StudentID] = AttendanceResponse{StudentID: r.StudentID, Status: r.Status, Catatan: r.Catatan}
		}
	}

	// 3. Construct Response
	sort.SliceStable(students, func(i, j int) bool {
		return strings.ToLower(students[i].NamaLengkap) < strings.ToLower(students[j].NamaLengkap)
	})

	var response []fiber.Map
	for _, s := range students {
		status := "hadir" // Default
		catatan := ""
		if val, ok := existingMap[s.ID]; ok {
			status = val.Status
			catatan = val.Catatan
		}

		nis := ""
		if s.NISN != nil {
			nis = *s.NISN
		}

		response = append(response, fiber.Map{
			"student_id":   s.ID,
			"student_name": s.NamaLengkap,
			"nis":          nis,
			"status":       status,
			"catatan":      catatan,
		})
	}

	return c.JSON(fiber.Map{
		"date":                  dateStr,
		"students":              response,
		"assigned_teacher_id":   assignedTeacherID,
		"substitute_teacher_id": substituteTeacherID,
	})
}

// SubmitAttendance saves student attendance
func (h *AbsensiHandler) SubmitAttendance(c *fiber.Ctx) error {
	type AttendanceInput struct {
		StudentID uint   `json:"student_id"`
		Status    string `json:"status"` // hadir, izin, sakit, alpa
		Catatan   string `json:"catatan"`
	}

	type SubmitRequest struct {
		JadwalID uint              `json:"jadwal_id"`
		Date     string            `json:"date"`
		Type     string            `json:"type"` // formal or diniyyah
		Records  []AttendanceInput `json:"records"`
	}

	user, err := h.getUserFromContext(c)
	if err != nil {
		return err
	}

	var req SubmitRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	date, err := time.Parse("2006-01-02", req.Date)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid date format"})
	}

	start, end := dateRange(req.Date)

	var originalTeacherID uint
	var substituteTeacherID *uint
	var substituteDate *time.Time
	if req.Type == "diniyyah" {
		var jadwal models.DiniyyahSchedule
		if err := h.db.Preload("Assignment").First(&jadwal, req.JadwalID).Error; err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Diniyyah schedule not found"})
		}
		originalTeacherID = jadwal.Assignment.UserID
		substituteTeacherID = jadwal.SubstituteTeacherID
		substituteDate = jadwal.SubstituteDate
	} else {
		var jadwal models.Schedule
		if err := h.db.Preload("Assignment").First(&jadwal, req.JadwalID).Error; err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Schedule not found"})
		}
		originalTeacherID = jadwal.Assignment.UserID
		substituteTeacherID = jadwal.SubstituteTeacherID
		substituteDate = jadwal.SubstituteDate
	}
	if !canAccessScheduledAttendance(user, originalTeacherID, substituteTeacherID, substituteDate, req.Date) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Anda tidak memiliki akses untuk mengisi absensi"})
	}

	tx := h.db.Begin()

	for _, record := range req.Records {
		if req.Type == "diniyyah" {
			// Upsert Diniyyah
			var absensi models.AbsensiDiniyyah
			err := tx.Where("jadwal_diniyyah_id = ? AND student_id = ? AND tanggal >= ? AND tanggal < ?",
				req.JadwalID, record.StudentID, start, end).First(&absensi).Error

			if err == gorm.ErrRecordNotFound {
				absensi = models.AbsensiDiniyyah{
					JadwalDiniyyahID: &req.JadwalID,
					StudentID:        record.StudentID,
					Tanggal:          date,
					Status:           record.Status,
					Catatan:          record.Catatan,
				}
				if err := tx.Create(&absensi).Error; err != nil {
					tx.Rollback()
					return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to save attendance"})
				}
			} else {
				absensi.Status = record.Status
				absensi.Catatan = record.Catatan
				if err := tx.Save(&absensi).Error; err != nil {
					tx.Rollback()
					return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update attendance"})
				}
			}

		} else {
			// Upsert Formal
			var absensi models.Absensi
			err := tx.Where("jadwal_formal_id = ? AND student_id = ? AND tanggal >= ? AND tanggal < ?",
				req.JadwalID, record.StudentID, start, end).First(&absensi).Error

			if err == gorm.ErrRecordNotFound {
				absensi = models.Absensi{
					JadwalFormalID: req.JadwalID,
					StudentID:      record.StudentID,
					Tanggal:        date,
					Status:         record.Status,
					Catatan:        record.Catatan,
				}
				if err := tx.Create(&absensi).Error; err != nil {
					tx.Rollback()
					return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to save attendance"})
				}
			} else {
				absensi.Status = record.Status
				absensi.Catatan = record.Catatan
				if err := tx.Save(&absensi).Error; err != nil {
					tx.Rollback()
					return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update attendance"})
				}
			}
		}
	}

	tx.Commit()
	return c.JSON(fiber.Map{"message": "Attendance saved successfully"})
}

// SubmitTeacherAttendance saves teacher attendance for both formal and diniyyah
func (h *AbsensiHandler) SubmitTeacherAttendance(c *fiber.Ctx) error {
	user, err := h.getUserFromContext(c)
	if err != nil {
		return err
	}

	var req struct {
		JadwalID uint   `json:"jadwal_id" form:"jadwal_id"`
		Date     string `json:"date" form:"date"`
		Type     string `json:"type" form:"type"` // "formal" or "diniyyah"
		Status   string `json:"status" form:"status"`
		Notes    string `json:"notes" form:"notes"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.Type == "" {
		req.Type = "formal" // Default to formal for backward compatibility
	}
	if req.Date == "" {
		req.Date = time.Now().Format("2006-01-02")
	}

	var originalTeacherID uint
	var jadwalFormalID *uint
	var jadwalDiniyyahID *uint
	var substituteTeacherID *uint
	var substituteDate *time.Time

	if req.Type == "diniyyah" {
		// Load DiniyyahSchedule
		var jadwal models.DiniyyahSchedule
		if err := h.db.Preload("Assignment").First(&jadwal, req.JadwalID).Error; err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Diniyyah schedule not found"})
		}
		originalTeacherID = jadwal.Assignment.UserID
		substituteTeacherID = jadwal.SubstituteTeacherID
		substituteDate = jadwal.SubstituteDate
		jadwalDiniyyahID = &req.JadwalID
	} else {
		// Load formal Schedule
		var jadwal models.Schedule
		if err := h.db.Preload("Assignment").First(&jadwal, req.JadwalID).Error; err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Schedule not found"})
		}
		originalTeacherID = jadwal.Assignment.UserID
		substituteTeacherID = jadwal.SubstituteTeacherID
		substituteDate = jadwal.SubstituteDate
		jadwalFormalID = &req.JadwalID
	}

	if !canManageTeacherAttendance(user) {
		isAssignedTeacher := user.ID == originalTeacherID
		isSubstituteTeacher := substituteTeacherID != nil && user.ID == *substituteTeacherID
		if !isAssignedTeacher && !isSubstituteTeacher {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Anda tidak memiliki akses untuk mengisi absensi guru"})
		}

		if isSubstituteTeacher {
			if substituteDate == nil || substituteDate.Format("2006-01-02") != req.Date {
				return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Anda hanya dapat mengisi sebagai pengganti pada tanggal penugasan"})
			}
		}
	}

	date, _ := time.Parse("2006-01-02", req.Date)
	start, end := dateRange(req.Date)

	// Handle Photo Upload
	photoPath := ""
	if file, err := c.FormFile("photo"); err == nil {
		filename := "teacher_attendance/" + req.Date + "_" + file.Filename
		if err := c.SaveFile(file, "./public/uploads/"+filename); err == nil {
			photoPath = "uploads/" + filename
		}
	}

	// Upsert TeacherAttendance — use date range instead of DATE()
	var attendance models.TeacherAttendance
	var findErr error

	if req.Type == "diniyyah" {
		findErr = h.db.Where("jadwal_diniyyah_id = ? AND user_id = ? AND date >= ? AND date < ?",
			req.JadwalID, originalTeacherID, start, end).First(&attendance).Error
	} else {
		findErr = h.db.Where("jadwal_formal_id = ? AND user_id = ? AND date >= ? AND date < ?",
			req.JadwalID, originalTeacherID, start, end).First(&attendance).Error
	}

	if findErr == gorm.ErrRecordNotFound {
		attendance = models.TeacherAttendance{
			JadwalFormalID:   jadwalFormalID,
			JadwalDiniyyahID: jadwalDiniyyahID,
			UserID:           originalTeacherID,
			Date:             date,
			Status:           req.Status,
			Notes:            req.Notes,
			PhotoPath:        photoPath,
		}
		h.db.Create(&attendance)
	} else {
		attendance.Status = req.Status
		attendance.Notes = req.Notes
		if photoPath != "" {
			attendance.PhotoPath = photoPath
		}
		h.db.Save(&attendance)
	}

	return c.JSON(fiber.Map{"message": "Teacher attendance saved"})
}

// AssignSubstitute assigns a substitute teacher to a schedule
func (h *AbsensiHandler) AssignSubstitute(c *fiber.Ctx) error {
	user, err := h.getUserFromContext(c)
	if err != nil {
		return err
	}

	// Allow admin, super_admin, staff, tim_presensi, or users with schedule.substitute permission
	if !(user.HasRole("super_admin") || user.HasRole("admin") || user.HasRole("staff") || user.HasRole("tim_presensi") || user.HasPermission("schedule.substitute")) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Anda tidak memiliki akses untuk menugaskan guru pengganti"})
	}

	var req struct {
		JadwalID            uint   `json:"jadwal_id"`
		SubstituteTeacherID *uint  `json:"substitute_teacher_id"` // Nullable to remove
		Date                string `json:"date"`
		Reason              string `json:"reason"`
		Status              string `json:"status"` // Status of original teacher (Sakit/Izin)
		Type                string `json:"type"`   // "formal" or "diniyyah"
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.Type == "" {
		req.Type = "formal"
	}

	tx := h.db.Begin()
	date, _ := time.Parse("2006-01-02", req.Date)
	var originalTeacherID uint
	var jadwalFormalID *uint
	var jadwalDiniyyahID *uint
	hasLegacyDiniyyahLogs := tx.Migrator().HasTable(&models.SubstituteDiniyyahLog{})

	if req.Type == "diniyyah" {
		var jadwal models.DiniyyahSchedule
		if err := tx.Preload("Assignment").First(&jadwal, req.JadwalID).Error; err != nil {
			tx.Rollback()
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Diniyyah schedule not found"})
		}
		jadwal.SubstituteTeacherID = req.SubstituteTeacherID
		if req.SubstituteTeacherID != nil {
			jadwal.SubstituteDate = &date
		} else {
			jadwal.SubstituteDate = nil
		}
		if err := tx.Save(&jadwal).Error; err != nil {
			tx.Rollback()
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update schedule"})
		}
		originalTeacherID = jadwal.Assignment.UserID
		jadwalDiniyyahID = &req.JadwalID
	} else {
		var jadwal models.Schedule
		if err := tx.Preload("Assignment").First(&jadwal, req.JadwalID).Error; err != nil {
			tx.Rollback()
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Schedule not found"})
		}
		jadwal.SubstituteTeacherID = req.SubstituteTeacherID
		if req.SubstituteTeacherID != nil {
			jadwal.SubstituteDate = &date
		} else {
			jadwal.SubstituteDate = nil
		}
		if err := tx.Save(&jadwal).Error; err != nil {
			tx.Rollback()
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update schedule"})
		}
		originalTeacherID = jadwal.Assignment.UserID
		jadwalFormalID = &req.JadwalID
	}

	if jadwalFormalID != nil {
		cleanupLogs := tx.Where("date = ?", date).
			Where("jadwal_formal_id = ?", *jadwalFormalID)
		if err := cleanupLogs.Delete(&models.SubstituteLog{}).Error; err != nil {
			tx.Rollback()
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to refresh substitute log"})
		}
	}

	if jadwalDiniyyahID != nil && hasLegacyDiniyyahLogs {
		if err := tx.Where("date = ? AND jadwal_diniyyah_id = ?", date, *jadwalDiniyyahID).Delete(&models.SubstituteDiniyyahLog{}).Error; err != nil {
			tx.Rollback()
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to refresh diniyyah substitute log"})
		}
	}

	// 2. Create Substitute Log
	if req.SubstituteTeacherID != nil {
		if jadwalFormalID != nil {
			logEntry := models.SubstituteLog{
				JadwalFormalID:      jadwalFormalID,
				OriginalTeacherID:   originalTeacherID,
				SubstituteTeacherID: *req.SubstituteTeacherID,
				Date:                date,
				Status:              req.Status,
				Reason:              req.Reason,
			}
			if err := tx.Create(&logEntry).Error; err != nil {
				tx.Rollback()
				log.Println("Failed to create log:", err)
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create log"})
			}
		}

		if jadwalDiniyyahID != nil && hasLegacyDiniyyahLogs {
			legacyLogEntry := models.SubstituteDiniyyahLog{
				JadwalDiniyyahID:    *jadwalDiniyyahID,
				OriginalTeacherID:   originalTeacherID,
				SubstituteTeacherID: *req.SubstituteTeacherID,
				Date:                date,
				Status:              req.Status,
				Reason:              req.Reason,
			}
			if err := tx.Create(&legacyLogEntry).Error; err != nil {
				tx.Rollback()
				log.Println("Failed to create legacy diniyyah log:", err)
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create diniyyah log"})
			}
		}
	}

	tx.Commit()
	return c.JSON(fiber.Map{"message": "Substitute assigned successfully"})
}

// DeleteSubstituteHistory deletes a substitute history record. Only super_admin can do this.
func (h *AbsensiHandler) DeleteSubstituteHistory(c *fiber.Ctx) error {
	user, err := h.getUserFromContext(c)
	if err != nil {
		return err
	}
	if !user.HasRole("super_admin") {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Hanya super_admin yang dapat menghapus riwayat guru pengganti"})
	}

	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil || id == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "ID riwayat tidak valid"})
	}

	typeStr := c.Query("type", "formal")
	tx := h.db.Begin()

	if isDiniyyahAttendanceType(typeStr) {
		var logEntry models.SubstituteDiniyyahLog
		if err := tx.First(&logEntry, uint(id)).Error; err != nil {
			tx.Rollback()
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Riwayat guru pengganti diniyyah tidak ditemukan"})
		}

		tx.Model(&models.DiniyyahSchedule{}).
			Where("id = ? AND substitute_teacher_id = ? AND substitute_date = ?", logEntry.JadwalDiniyyahID, logEntry.SubstituteTeacherID, logEntry.Date).
			Updates(map[string]interface{}{"substitute_teacher_id": nil, "substitute_date": nil})

		if err := tx.Delete(&logEntry).Error; err != nil {
			tx.Rollback()
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Gagal menghapus riwayat guru pengganti diniyyah"})
		}
	} else {
		var logEntry models.SubstituteLog
		if err := tx.First(&logEntry, uint(id)).Error; err != nil {
			tx.Rollback()
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Riwayat guru pengganti formal tidak ditemukan"})
		}

		if logEntry.JadwalFormalID != nil {
			tx.Model(&models.Schedule{}).
				Where("id = ? AND substitute_teacher_id = ? AND substitute_date = ?", *logEntry.JadwalFormalID, logEntry.SubstituteTeacherID, logEntry.Date).
				Updates(map[string]interface{}{"substitute_teacher_id": nil, "substitute_date": nil})
		}

		if err := tx.Delete(&logEntry).Error; err != nil {
			tx.Rollback()
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Gagal menghapus riwayat guru pengganti formal"})
		}
	}

	tx.Commit()
	return c.JSON(fiber.Map{"message": "Riwayat guru pengganti berhasil dihapus"})
}

// GetStatistics returns aggregated attendance statistics for the stats page.
func (h *AbsensiHandler) GetStatistics(c *fiber.Ctx) error {
	typeStr := c.Query("type", "formal") // formal or diniyyah
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	kelasID := c.Query("kelas_id")
	gender := c.Query("gender")
	jenjang := c.Query("jenjang") // smp or sma
	status := c.Query("status")   // izin, sakit, alpa

	// Default to current month
	if startDate == "" {
		now := time.Now()
		startDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
	}
	if endDate == "" {
		endDate = time.Now().Format("2006-01-02")
	}

	// Parse end date + 1 day for exclusive range
	endT, _ := time.Parse("2006-01-02", endDate)
	endExclusive := endT.AddDate(0, 0, 1).Format("2006-01-02")

	// ── Student Attendance ──
	type StudentEntry struct {
		Name    string `json:"name"`
		Catatan string `json:"catatan"`
	}

	var izinStudents, sakitStudents, alpaStudents []StudentEntry

	// ── Laravel pattern: single query, derive counts + details from same result set ──
	// This mirrors the old AbsensiController@statistics approach:
	//   1. Build ONE query with ALL filters (jenjang, kelas, gender, status)
	//   2. Fetch all matching rows
	//   3. Count statuses and extract detail lists from that single result

	type AbsensiRow struct {
		Status  string `json:"status"`
		Name    string `json:"name"`
		Catatan string `json:"catatan"`
	}

	table := "absensis"
	if isDiniyyahAttendanceType(typeStr) {
		table = "absensi_diniyyahs"
	}

	q := h.db.Table(table).
		Select("students.nama_lengkap as name, "+table+".status, "+table+".catatan").
		Joins("JOIN students ON students.id = "+table+".student_id").
		Where(table+".tanggal >= ? AND "+table+".tanggal < ?", startDate, endExclusive).
		Where(table + ".deleted_at IS NULL")
	if !isDiniyyahAttendanceType(typeStr) {
		q = q.Joins("JOIN jadwal_formal jf ON jf.id = " + table + ".jadwal_formal_id")
		q = applyFormalScheduleTypeFilter(q, "jf", typeStr)
	}

	// Filter by kelas
	if kelasID != "" {
		q = q.Where("students.kelas_id = ?", kelasID)
	}

	// Filter by gender or jenjang (both need the kelas table)
	if gender != "" || jenjang != "" {
		q = q.Joins("JOIN kelas kf ON kf.id = students.kelas_id")
		if gender != "" {
			q = q.Where("kf.gender = ?", gender)
		}
		if jenjang == "smp" {
			q = q.Where("(kf.nama LIKE ? OR kf.nama LIKE ? OR kf.nama LIKE ? OR kf.tingkat LIKE ? OR kf.tingkat LIKE ? OR kf.tingkat LIKE ?)",
				"%7%", "%8%", "%9%", "%7%", "%8%", "%9%")
		} else if jenjang == "sma" {
			q = q.Where("(kf.nama LIKE ? OR kf.nama LIKE ? OR kf.nama LIKE ? OR kf.tingkat LIKE ? OR kf.tingkat LIKE ? OR kf.tingkat LIKE ?)",
				"%10%", "%11%", "%12%", "%10%", "%11%", "%12%")
		}
	}

	// Filter by status
	if status != "" {
		q = q.Where(table+".status = ?", status)
	}

	// Fetch ALL matching rows (same as Laravel's chunk+merge)
	var rows []AbsensiRow
	q.Scan(&rows)

	// Derive counts and detail lists from the same result set
	countsMap := map[string]int{"hadir": 0, "izin": 0, "sakit": 0, "alpa": 0}
	for _, r := range rows {
		countsMap[r.Status]++
		switch r.Status {
		case "izin":
			izinStudents = append(izinStudents, StudentEntry{Name: r.Name, Catatan: r.Catatan})
		case "sakit":
			sakitStudents = append(sakitStudents, StudentEntry{Name: r.Name, Catatan: r.Catatan})
		case "alpa":
			alpaStudents = append(alpaStudents, StudentEntry{Name: r.Name, Catatan: r.Catatan})
		}
	}

	// Ensure non-nil slices for JSON
	if izinStudents == nil {
		izinStudents = []StudentEntry{}
	}
	if sakitStudents == nil {
		sakitStudents = []StudentEntry{}
	}
	if alpaStudents == nil {
		alpaStudents = []StudentEntry{}
	}

	// ── Teacher Attendance ──
	type TeacherSummaryEntry struct {
		ID         uint   `json:"id"`
		Name       string `json:"name"`
		Avatar     string `json:"avatar"`
		Hadir      int    `json:"hadir"`
		Izin       int    `json:"izin"`
		Sakit      int    `json:"sakit"`
		Alpha      int    `json:"alpha"`
		Substitute int    `json:"substitute"`
	}

	var teacherSummary []TeacherSummaryEntry
	teacherCountsMap := map[string]int{"Hadir": 0, "Izin": 0, "Sakit": 0, "Alpha": 0}

	// 1. Aggregate Global Teacher Counts
	teacherQ := h.db.Table("teacher_attendances").
		Where("date >= ? AND date < ?", startDate, endExclusive).
		Where("deleted_at IS NULL")

	if isDiniyyahAttendanceType(typeStr) {
		teacherQ = teacherQ.Where("jadwal_diniyyah_id IS NOT NULL")
	} else {
		teacherQ = teacherQ.Where("jadwal_formal_id IS NOT NULL").Joins("JOIN jadwal_formal jf ON jf.id = teacher_attendances.jadwal_formal_id")
		teacherQ = applyFormalScheduleTypeFilter(teacherQ, "jf", typeStr)
	}

	type TeacherStatusResult struct {
		Status string
		Count  int
	}
	var teacherStatusResults []TeacherStatusResult
	teacherQ.Select("status, count(*) as count").Group("status").Scan(&teacherStatusResults)

	for _, ts := range teacherStatusResults {
		teacherCountsMap[ts.Status] = ts.Count
	}

	// 2. Fetch Teacher Summary (Per-teacher counts)
	// Query from teacher_attendances and JOIN to users for names.
	summaryQ := h.db.Table("teacher_attendances ta").
		Select("u.id, u.name, u.foto_guru as avatar, "+
			"SUM(CASE WHEN ta.status = 'Hadir' THEN 1 ELSE 0 END) as hadir, "+
			"SUM(CASE WHEN ta.status = 'Izin' THEN 1 ELSE 0 END) as izin, "+
			"SUM(CASE WHEN ta.status = 'Sakit' THEN 1 ELSE 0 END) as sakit, "+
			"SUM(CASE WHEN ta.status = 'Alpha' THEN 1 ELSE 0 END) as alpha").
		Joins("JOIN users u ON u.id = ta.user_id").
		Where("ta.date >= ? AND ta.date < ?", startDate, endExclusive).
		Where("ta.deleted_at IS NULL")

	if isDiniyyahAttendanceType(typeStr) {
		summaryQ = summaryQ.Where("ta.jadwal_diniyyah_id IS NOT NULL")
	} else {
		summaryQ = summaryQ.Where("ta.jadwal_formal_id IS NOT NULL").Joins("JOIN jadwal_formal jf ON jf.id = ta.jadwal_formal_id")
		summaryQ = applyFormalScheduleTypeFilter(summaryQ, "jf", typeStr)
	}

	summaryQ.Group("u.id, u.name, u.foto_guru").Scan(&teacherSummary)

	// 3. Substitute History & Substitute Counts for Summary
	type SubHistoryEntry struct {
		Date              time.Time `json:"date"`
		Lesson            string    `json:"lesson"`
		Kelas             string    `json:"kelas"`
		OriginalTeacher   string    `json:"original_teacher"`
		SubstituteTeacher string    `json:"substitute_teacher"`
		Status            string    `json:"status"`
		Reason            string    `json:"reason"`
	}
	var substituteHistory []SubHistoryEntry

	if !isDiniyyahAttendanceType(typeStr) {
		h.db.Table("substitute_logs").
			Select("substitute_logs.date, lessons.nama as lesson, kelas.nama as kelas, original.name as original_teacher, substitute.name as substitute_teacher, substitute_logs.status, substitute_logs.reason").
			Joins("JOIN jadwal_formal ON jadwal_formal.id = substitute_logs.jadwal_formal_id").
			Joins("JOIN lesson_kelas_teachers ON lesson_kelas_teachers.id = jadwal_formal.lesson_kelas_teacher_id").
			Joins("JOIN lessons ON lessons.id = lesson_kelas_teachers.lesson_id").
			Joins("JOIN kelas ON kelas.id = lesson_kelas_teachers.kelas_id").
			Joins("JOIN users original ON original.id = substitute_logs.original_teacher_id").
			Joins("JOIN users substitute ON substitute.id = substitute_logs.substitute_teacher_id").
			Where("substitute_logs.date >= ? AND substitute_logs.date < ?", startDate, endExclusive).
			Where("substitute_logs.deleted_at IS NULL").
			Scopes(func(db *gorm.DB) *gorm.DB { return applyFormalScheduleTypeFilter(db, "jadwal_formal", typeStr) }).
			Order("substitute_logs.date DESC").
			Scan(&substituteHistory)
	} else if isDiniyyahAttendanceType(typeStr) {
		h.db.Table("substitute_logs").
			Select("substitute_logs.date, diniyyah_lessons.nama as lesson, kelas.nama as kelas, original.name as original_teacher, substitute.name as substitute_teacher, substitute_logs.status, substitute_logs.reason").
			Joins("JOIN jadwal_diniyyahs ON jadwal_diniyyahs.id = substitute_logs.jadwal_diniyyah_id").
			Joins("JOIN diniyyah_kelas_teachers ON diniyyah_kelas_teachers.id = jadwal_diniyyahs.diniyyah_kelas_teacher_id").
			Joins("JOIN diniyyah_lessons ON diniyyah_lessons.id = diniyyah_kelas_teachers.diniyyah_lesson_id").
			Joins("JOIN kelas ON kelas.id = diniyyah_kelas_teachers.kelas_id").
			Joins("JOIN users original ON original.id = substitute_logs.original_teacher_id").
			Joins("JOIN users substitute ON substitute.id = substitute_logs.substitute_teacher_id").
			Where("substitute_logs.date >= ? AND substitute_logs.date < ?", startDate, endExclusive).
			Where("substitute_logs.deleted_at IS NULL").
			Order("substitute_logs.date DESC").
			Scan(&substituteHistory)
	}

	// Add substitute counts to teacher summary
	type SubCount struct {
		ID    uint
		Count int
	}
	var subCounts []SubCount
	subQ := h.db.Table("substitute_logs").
		Select("substitute_teacher_id as id, count(*) as count").
		Where("date >= ? AND date < ?", startDate, endExclusive).
		Where("deleted_at IS NULL")

	if isDiniyyahAttendanceType(typeStr) {
		subQ = subQ.Where("jadwal_diniyyah_id IS NOT NULL")
	} else {
		subQ = subQ.Where("jadwal_formal_id IS NOT NULL").Joins("JOIN jadwal_formal jf ON jf.id = substitute_logs.jadwal_formal_id")
		subQ = applyFormalScheduleTypeFilter(subQ, "jf", typeStr)
	}

	subQ.Group("substitute_teacher_id").Scan(&subCounts)

	subMap := make(map[uint]int)
	for _, sc := range subCounts {
		subMap[sc.ID] = sc.Count
	}

	for i := range teacherSummary {
		teacherSummary[i].Substitute = subMap[teacherSummary[i].ID]
	}

	// Build chart data for student bar chart
	chartData := []fiber.Map{
		{"name": "Hadir", "value": countsMap["hadir"]},
		{"name": "Izin", "value": countsMap["izin"]},
		{"name": "Sakit", "value": countsMap["sakit"]},
		{"name": "Alpa", "value": countsMap["alpa"]},
	}

	return c.JSON(fiber.Map{
		"student_counts":     countsMap,
		"teacher_counts":     teacherCountsMap,
		"izin_students":      izinStudents,
		"sakit_students":     sakitStudents,
		"alpa_students":      alpaStudents,
		"chart_data":         chartData,
		"teacher_summary":    teacherSummary,
		"substitute_history": substituteHistory,
	})
}

// GetTeacherStatistics returns aggregated attendance statistics for teachers.
func (h *AbsensiHandler) GetTeacherStatistics(c *fiber.Ctx) error {
	typeStr := c.Query("type", "formal") // formal or diniyyah
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	teacherID := c.Query("teacher_id")
	gender := c.Query("gender")
	kelasID := c.Query("kelas_id")

	// Default to current month
	if startDate == "" {
		now := time.Now()
		startDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
	}
	if endDate == "" {
		endDate = time.Now().Format("2006-01-02")
	}

	endT, _ := time.Parse("2006-01-02", endDate)
	endExclusive := endT.AddDate(0, 0, 1).Format("2006-01-02")

	// ── Teacher Attendance ──
	type TeacherSummaryEntry struct {
		ID         uint   `json:"id"`
		Name       string `json:"name"`
		Avatar     string `json:"avatar"`
		Hadir      int    `json:"hadir"`
		Izin       int    `json:"izin"`
		Sakit      int    `json:"sakit"`
		Alpha      int    `json:"alpha"`
		Substitute int    `json:"substitute"`
	}

	var teacherSummary []TeacherSummaryEntry
	teacherCountsMap := map[string]int{"Hadir": 0, "Izin": 0, "Sakit": 0, "Alpha": 0, "Substitute": 0}

	// 1. Fetch Teacher Summary (Per-teacher counts)
	summaryQ := h.db.Table("teacher_attendances ta").
		Select("u.id, u.name, u.foto_guru as avatar, "+
			"SUM(CASE WHEN ta.status = 'Hadir' THEN 1 ELSE 0 END) as hadir, "+
			"SUM(CASE WHEN ta.status = 'Izin' THEN 1 ELSE 0 END) as izin, "+
			"SUM(CASE WHEN ta.status = 'Sakit' THEN 1 ELSE 0 END) as sakit, "+
			"SUM(CASE WHEN ta.status = 'Alpha' THEN 1 ELSE 0 END) as alpha").
		Joins("JOIN users u ON u.id = ta.user_id").
		Where("ta.date >= ? AND ta.date < ?", startDate, endExclusive).
		Where("ta.deleted_at IS NULL")

	if isDiniyyahAttendanceType(typeStr) {
		summaryQ = summaryQ.Where("ta.jadwal_diniyyah_id IS NOT NULL")
		if kelasID != "" {
			summaryQ = summaryQ.Joins("JOIN jadwal_diniyyahs jd ON jd.id = ta.jadwal_diniyyah_id").
				Joins("JOIN diniyyah_kelas_teachers dkt ON dkt.id = jd.diniyyah_kelas_teacher_id").
				Where("dkt.kelas_id = ?", kelasID)
		}
	} else {
		summaryQ = summaryQ.Where("ta.jadwal_formal_id IS NOT NULL").Joins("JOIN jadwal_formal jf ON jf.id = ta.jadwal_formal_id")
		summaryQ = applyFormalScheduleTypeFilter(summaryQ, "jf", typeStr)
	}

	if teacherID != "" {
		summaryQ = summaryQ.Where("ta.user_id = ?", teacherID)
	}
	if gender != "" {
		summaryQ = summaryQ.Where("u.gender = ?", gender)
	}

	summaryQ.Group("u.id, u.name, u.foto_guru").Scan(&teacherSummary)

	// Update Global Counts from Summary
	for _, t := range teacherSummary {
		teacherCountsMap["Hadir"] += t.Hadir
		teacherCountsMap["Izin"] += t.Izin
		teacherCountsMap["Sakit"] += t.Sakit
		teacherCountsMap["Alpha"] += t.Alpha
	}

	// 2. Substitute History
	type SubHistoryEntry struct {
		ID                uint      `json:"id"`
		Date              time.Time `json:"date"`
		Lesson            string    `json:"lesson"`
		Kelas             string    `json:"kelas"`
		OriginalTeacher   string    `json:"original_teacher"`
		OriginalStatus    string    `json:"original_status"`
		SubstituteTeacher string    `json:"substitute_teacher"`
		Reason            string    `json:"reason"`
	}
	var substituteHistory []SubHistoryEntry

	if !isDiniyyahAttendanceType(typeStr) {
		subQ := h.db.Table("substitute_logs").
			Select("substitute_logs.id, substitute_logs.date, lessons.nama as lesson, kelas.nama as kelas, "+
				"original.name as original_teacher, substitute_logs.status as original_status, "+
				"substitute.name as substitute_teacher, substitute_logs.reason").
			Joins("JOIN jadwal_formal ON jadwal_formal.id = substitute_logs.jadwal_formal_id").
			Joins("JOIN lesson_kelas_teachers lkt ON lkt.id = jadwal_formal.lesson_kelas_teacher_id").
			Joins("JOIN lessons ON lessons.id = lkt.lesson_id").
			Joins("JOIN kelas ON kelas.id = lkt.kelas_id").
			Joins("JOIN users original ON original.id = substitute_logs.original_teacher_id").
			Joins("JOIN users substitute ON substitute.id = substitute_logs.substitute_teacher_id").
			Where("substitute_logs.date >= ? AND substitute_logs.date < ?", startDate, endExclusive).
			Where("substitute_logs.deleted_at IS NULL")
		subQ = applyFormalScheduleTypeFilter(subQ, "jadwal_formal", typeStr)

		if teacherID != "" {
			subQ = subQ.Where("(substitute_logs.original_teacher_id = ? OR substitute_logs.substitute_teacher_id = ?)", teacherID, teacherID)
		}
		if gender != "" {
			subQ = subQ.Where("(original.gender = ? OR substitute.gender = ?)", gender, gender)
		}
		subQ.Order("substitute_logs.date DESC").Scan(&substituteHistory)
	} else if isDiniyyahAttendanceType(typeStr) {
		subQ := h.db.Table("substitute_logs_diniyyah").
			Select("substitute_logs_diniyyah.id, substitute_logs_diniyyah.date, diniyyah_lessons.nama as lesson, kelas.nama as kelas, "+
				"original.name as original_teacher, substitute_logs_diniyyah.status as original_status, "+
				"substitute.name as substitute_teacher, substitute_logs_diniyyah.reason").
			Joins("JOIN jadwal_diniyyahs jd ON jd.id = substitute_logs_diniyyah.jadwal_diniyyah_id").
			Joins("JOIN diniyyah_kelas_teachers dkt ON dkt.id = jd.diniyyah_kelas_teacher_id").
			Joins("JOIN diniyyah_lessons ON diniyyah_lessons.id = dkt.diniyyah_lesson_id").
			Joins("JOIN kelas ON kelas.id = dkt.kelas_id").
			Joins("JOIN users original ON original.id = substitute_logs_diniyyah.original_teacher_id").
			Joins("JOIN users substitute ON substitute.id = substitute_logs_diniyyah.substitute_teacher_id").
			Where("substitute_logs_diniyyah.date >= ? AND substitute_logs_diniyyah.date < ?", startDate, endExclusive)

		if teacherID != "" {
			subQ = subQ.Where("(substitute_logs_diniyyah.original_teacher_id = ? OR substitute_logs_diniyyah.substitute_teacher_id = ?)", teacherID, teacherID)
		}
		if gender != "" {
			subQ = subQ.Where("(original.gender = ? OR substitute.gender = ?)", gender, gender)
		}
		if kelasID != "" {
			subQ = subQ.Where("dkt.kelas_id = ?", kelasID)
		}
		subQ.Order("substitute_logs_diniyyah.date DESC").Scan(&substituteHistory)
	}

	// Add substitute counts to teacher summary
	type SubCount struct {
		ID     uint
		Name   string
		Avatar string
		Count  int
	}
	var subCounts []SubCount
	if isDiniyyahAttendanceType(typeStr) {
		subQ := h.db.Table("substitute_logs_diniyyah").
			Select("substitute_logs_diniyyah.substitute_teacher_id as id, u.name, u.foto_guru as avatar, count(*) as count").
			Joins("JOIN users u ON u.id = substitute_logs_diniyyah.substitute_teacher_id").
			Where("substitute_logs_diniyyah.date >= ? AND substitute_logs_diniyyah.date < ?", startDate, endExclusive)

		if kelasID != "" {
			subQ = subQ.Joins("JOIN jadwal_diniyyahs jd ON jd.id = substitute_logs_diniyyah.jadwal_diniyyah_id").
				Joins("JOIN diniyyah_kelas_teachers dkt ON dkt.id = jd.diniyyah_kelas_teacher_id").
				Where("dkt.kelas_id = ?", kelasID)
		}
		if teacherID != "" {
			subQ = subQ.Where("substitute_logs_diniyyah.substitute_teacher_id = ?", teacherID)
		}
		if gender != "" {
			subQ = subQ.Where("u.gender = ?", gender)
		}
		subQ.Group("substitute_logs_diniyyah.substitute_teacher_id, u.name, u.foto_guru").Scan(&subCounts)
	} else {
		subQ := h.db.Table("substitute_logs").
			Select("substitute_logs.substitute_teacher_id as id, u.name, u.foto_guru as avatar, count(*) as count").
			Joins("JOIN users u ON u.id = substitute_logs.substitute_teacher_id").
			Where("substitute_logs.date >= ? AND substitute_logs.date < ?", startDate, endExclusive).
			Where("substitute_logs.deleted_at IS NULL").
			Where("substitute_logs.jadwal_formal_id IS NOT NULL").
			Joins("JOIN jadwal_formal jf ON jf.id = substitute_logs.jadwal_formal_id")
		subQ = applyFormalScheduleTypeFilter(subQ, "jf", typeStr)
		if teacherID != "" {
			subQ = subQ.Where("substitute_logs.substitute_teacher_id = ?", teacherID)
		}
		if gender != "" {
			subQ = subQ.Where("u.gender = ?", gender)
		}
		subQ.Group("substitute_logs.substitute_teacher_id, u.name, u.foto_guru").Scan(&subCounts)
	}

	inSummaryMap := make(map[uint]int) // Maps teacher ID to their index in teacherSummary
	for i, t := range teacherSummary {
		inSummaryMap[t.ID] = i
	}

	for _, sc := range subCounts {
		if idx, exists := inSummaryMap[sc.ID]; exists {
			teacherSummary[idx].Substitute = sc.Count
		} else {
			// Teacher only has substitute records
			teacherSummary = append(teacherSummary, TeacherSummaryEntry{
				ID:         sc.ID,
				Name:       sc.Name,
				Avatar:     sc.Avatar,
				Hadir:      0,
				Izin:       0,
				Sakit:      0,
				Alpha:      0,
				Substitute: sc.Count,
			})
		}
	}

	// Update Substitute Total Count
	for _, sub := range substituteHistory {
		// Only count substitute_teacher matches if teacher_id is filtered
		if teacherID != "" && teacherID == fmt.Sprint(sub.SubstituteTeacher) {
			teacherCountsMap["Substitute"]++
		} else if teacherID == "" {
			teacherCountsMap["Substitute"]++
		}
	}

	return c.JSON(fiber.Map{
		"teacher_counts":     teacherCountsMap,
		"teacher_summary":    teacherSummary,
		"substitute_history": substituteHistory,
	})
}
