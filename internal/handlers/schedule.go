package handlers

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"gorm.io/gorm"
)

type ScheduleHandler struct {
	db *gorm.DB
}

func NewScheduleHandler(db *gorm.DB) *ScheduleHandler {
	return &ScheduleHandler{db: db}
}

func normalizedFormalScheduleType(typeStr string) string {
	v := strings.TrimSpace(strings.ToLower(typeStr))
	if v == "ramadhan" || v == "ramadan" {
		return "ramadhan"
	}
	return "normal"
}

// List returns schedules with attendance status
func (h *ScheduleHandler) List(c *fiber.Ctx) error {
	scheduleType := c.Query("type", "formal")
	classID := c.Query("class_id")
	teacherID := c.Query("teacher_id")
	day := c.Query("day")
	dateStr := c.Query("date", time.Now().Format("2006-01-02")) // Default to today
	search := strings.TrimSpace(c.Query("search"))
	gender := strings.TrimSpace(c.Query("gender"))

	// Validate format only — we use the string directly for MySQL DATE comparisons
	if _, err := time.Parse("2006-01-02", dateStr); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid date format"})
	}

	if scheduleType == "diniyyah" {
		var schedules []models.DiniyyahSchedule
		query := h.db.Preload("Assignment.Kelas").
			Preload("Assignment.Teacher").
			Preload("Assignment.DiniyyahLesson").
			Preload("SubstituteTeacher")

		if classID != "" || teacherID != "" || gender != "" || search != "" {
			query = query.Joins("JOIN diniyyah_kelas_teachers dkt ON jadwal_diniyyahs.diniyyah_kelas_teacher_id = dkt.id")
		}
		if classID != "" {
			query = query.Where("dkt.kelas_id = ?", classID)
		}
		if teacherID != "" {
			query = query.Where("dkt.user_id = ?", teacherID)
		}
		if gender != "" {
			query = query.Joins("JOIN kelas k ON dkt.kelas_id = k.id").
				Where("k.gender = ?", gender)
		}
		if search != "" {
			term := "%" + strings.ToLower(search) + "%"
			query = query.Joins("JOIN diniyyah_lessons dl ON dkt.diniyyah_lesson_id = dl.id").
				Joins("JOIN kelas k2 ON dkt.kelas_id = k2.id").
				Joins("JOIN users u ON dkt.user_id = u.id").
				Where("LOWER(dl.nama) LIKE ? OR LOWER(k2.nama) LIKE ? OR LOWER(u.name) LIKE ?", term, term, term)
		}
		if day != "" {
			query = query.Where("hari = ?", day)
		}

		if err := query.Find(&schedules).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch schedules", "details": err.Error()})
		}

		// Prevent index out of range if empty
		if len(schedules) == 0 {
			return c.JSON([]models.DiniyyahSchedule{})
		}

		// Batched Attendance Counts
		var scheduleIDs []uint
		for _, s := range schedules {
			scheduleIDs = append(scheduleIDs, s.ID)
		}

		// Date range for index-friendly queries
		t, _ := time.Parse("2006-01-02", dateStr)
		dateStart := t.Format("2006-01-02")
		dateEnd := t.AddDate(0, 0, 1).Format("2006-01-02")

		type CountResult struct {
			JadwalDiniyyahID uint
			Status           string
			Count            int64
		}
		var counts []CountResult
		h.db.Table("absensi_diniyyahs").
			Select("jadwal_diniyyah_id, status, count(*) as count").
			Where("jadwal_diniyyah_id IN ? AND tanggal >= ? AND tanggal < ?", scheduleIDs, dateStart, dateEnd).
			Where("deleted_at IS NULL").
			Group("jadwal_diniyyah_id, status").
			Scan(&counts)

		// Map counts back to schedules
		countsMap := make(map[uint]map[string]int64)
		for _, res := range counts {
			if _, ok := countsMap[res.JadwalDiniyyahID]; !ok {
				countsMap[res.JadwalDiniyyahID] = make(map[string]int64)
			}
			countsMap[res.JadwalDiniyyahID][res.Status] = res.Count
		}

		// Teacher attendance check for Diniyyah
		var dinTeacherAttIDs []uint
		h.db.Table("teacher_attendances").
			Where("jadwal_diniyyah_id IN ? AND date >= ? AND date < ?", scheduleIDs, dateStart, dateEnd).
			Pluck("jadwal_diniyyah_id", &dinTeacherAttIDs)
		dinTeacherAttMap := make(map[uint]bool)
		for _, id := range dinTeacherAttIDs {
			dinTeacherAttMap[id] = true
		}

		for i := range schedules {
			if statusCounts, ok := countsMap[schedules[i].ID]; ok {
				schedules[i].AttendanceCounts = statusCounts
				schedules[i].HasAttendance = true
			} else {
				schedules[i].AttendanceCounts = make(map[string]int64)
			}
			schedules[i].HasTeacherAttendance = dinTeacherAttMap[schedules[i].ID]
		}

		return c.JSON(schedules)
	}

	// Formal
	var schedules []models.Schedule
	query := h.db.Preload("Assignment.Kelas").
		Preload("Assignment.Teacher").
		Preload("Assignment.Lesson").
		Preload("SubstituteTeacher")

	if normalizedFormalScheduleType(scheduleType) == "ramadhan" {
		query = query.Where("jadwal_formal.type = ?", "ramadhan")
	} else {
		query = query.Where("jadwal_formal.type IS NULL OR jadwal_formal.type = '' OR jadwal_formal.type = 'normal' OR jadwal_formal.type = 'formal'")
	}

	if classID != "" || teacherID != "" || gender != "" || search != "" {
		query = query.Joins("JOIN lesson_kelas_teachers lkt ON jadwal_formal.lesson_kelas_teacher_id = lkt.id")
	}
	if classID != "" {
		query = query.Where("lkt.kelas_id = ?", classID)
	}
	if teacherID != "" {
		query = query.Where("lkt.user_id = ?", teacherID)
	}
	if gender != "" {
		query = query.Joins("JOIN kelas k ON lkt.kelas_id = k.id").
			Where("k.gender = ?", gender)
	}
	if search != "" {
		term := "%" + strings.ToLower(search) + "%"
		query = query.Joins("JOIN lessons ls ON lkt.lesson_id = ls.id").
			Joins("JOIN users u ON lkt.user_id = u.id").
			Joins("JOIN kelas k2 ON lkt.kelas_id = k2.id").
			Where("LOWER(ls.nama) LIKE ? OR LOWER(k2.nama) LIKE ? OR LOWER(u.name) LIKE ?", term, term, term)
	}
	if day != "" {
		query = query.Where("hari = ?", day)
	}

	if err := query.Find(&schedules).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch schedules", "details": err.Error()})
	}

	if len(schedules) == 0 {
		return c.JSON([]models.Schedule{})
	}

	// Batched Logic for Formal
	var scheduleIDs []uint
	for _, s := range schedules {
		scheduleIDs = append(scheduleIDs, s.ID)
	}

	// 1. Student Attendance Counts
	type CountResult struct {
		JadwalFormalID uint
		Status         string
		Count          int64
	}
	// Date range for index-friendly queries
	t, _ := time.Parse("2006-01-02", dateStr)
	dateStart := t.Format("2006-01-02")
	dateEnd := t.AddDate(0, 0, 1).Format("2006-01-02")

	var counts []CountResult
	h.db.Table("absensis").
		Select("jadwal_formal_id, status, count(*) as count").
		Where("jadwal_formal_id IN ? AND tanggal >= ? AND tanggal < ?", scheduleIDs, dateStart, dateEnd).
		Where("deleted_at IS NULL").
		Group("jadwal_formal_id, status").
		Scan(&counts)

	countsMap := make(map[uint]map[string]int64)
	for _, res := range counts {
		if _, ok := countsMap[res.JadwalFormalID]; !ok {
			countsMap[res.JadwalFormalID] = make(map[string]int64)
		}
		countsMap[res.JadwalFormalID][res.Status] = res.Count
	}

	// 2. Teacher Attendance Check — use date range for index
	var teacherAttendanceIDs []uint
	h.db.Table("teacher_attendances").
		Where("jadwal_formal_id IN ? AND date >= ? AND date < ?", scheduleIDs, dateStart, dateEnd).
		Pluck("jadwal_formal_id", &teacherAttendanceIDs)

	teacherAttendanceMap := make(map[uint]bool)
	for _, id := range teacherAttendanceIDs {
		teacherAttendanceMap[id] = true
	}

	// Populate Fields
	for i := range schedules {
		if statusCounts, ok := countsMap[schedules[i].ID]; ok {
			schedules[i].AttendanceCounts = statusCounts
			schedules[i].HasAttendance = true
		} else {
			schedules[i].AttendanceCounts = make(map[string]int64)
		}

		if _, ok := teacherAttendanceMap[schedules[i].ID]; ok {
			schedules[i].HasTeacherAttendance = true
		}
	}

	return c.JSON(schedules)
}

// Create adds a new schedule entry
func (h *ScheduleHandler) Create(c *fiber.Ctx) error {
	type Input struct {
		Type         string `json:"type"` // "formal" or "diniyyah"
		AssignmentID uint   `json:"assignment_id"`
		Day          string `json:"day"`
		StartTime    string `json:"start_time"`
		EndTime      string `json:"end_time"`
	}
	var input Input
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid input"})
	}

	if input.Type == "diniyyah" {
		schedule := models.DiniyyahSchedule{
			DiniyyahLessonTeacherID: input.AssignmentID,
			Day:                     input.Day,
			StartTime:               input.StartTime + ":00",
			EndTime:                 input.EndTime + ":00",
		}
		if err := h.db.Create(&schedule).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create schedule"})
		}
		return c.Status(fiber.StatusCreated).JSON(schedule)
	}

	// Formal
	schedule := models.Schedule{
		Type:            normalizedFormalScheduleType(input.Type),
		LessonTeacherID: input.AssignmentID,
		Day:             input.Day,
		StartTime:       input.StartTime + ":00",
		EndTime:         input.EndTime + ":00",
	}
	if err := h.db.Create(&schedule).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create schedule"})
	}
	return c.Status(fiber.StatusCreated).JSON(schedule)
}

// Delete removes a schedule entry
func (h *ScheduleHandler) Delete(c *fiber.Ctx) error {
	id := c.Params("id")
	scheduleType := c.Query("type", "formal")

	if scheduleType == "diniyyah" {
		if err := h.db.Delete(&models.DiniyyahSchedule{}, id).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to delete schedule"})
		}
	} else {
		if err := h.db.Delete(&models.Schedule{}, id).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to delete schedule"})
		}
	}

	return c.JSON(fiber.Map{"message": "Schedule deleted successfully"})
}

// Update modifies an existing schedule
func (h *ScheduleHandler) Update(c *fiber.Ctx) error {
	id := c.Params("id")
	type Input struct {
		Type         string `json:"type"` // "formal" or "diniyyah"
		AssignmentID uint   `json:"assignment_id"`
		Day          string `json:"day"`
		StartTime    string `json:"start_time"`
		EndTime      string `json:"end_time"`
	}
	var input Input
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid input"})
	}

	if input.Type == "diniyyah" {
		var schedule models.DiniyyahSchedule
		if err := h.db.First(&schedule, id).Error; err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Schedule not found"})
		}
		schedule.DiniyyahLessonTeacherID = input.AssignmentID
		schedule.Day = input.Day
		schedule.StartTime = input.StartTime + ":00"
		schedule.EndTime = input.EndTime + ":00"
		h.db.Save(&schedule)
		return c.JSON(schedule)
	}

	// Formal
	var schedule models.Schedule
	if err := h.db.First(&schedule, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Schedule not found"})
	}
	schedule.Type = normalizedFormalScheduleType(input.Type)
	schedule.LessonTeacherID = input.AssignmentID
	schedule.Day = input.Day
	schedule.StartTime = input.StartTime + ":00"
	schedule.EndTime = input.EndTime + ":00"
	h.db.Save(&schedule)
	return c.JSON(schedule)
}
