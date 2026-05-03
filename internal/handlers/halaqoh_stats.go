package handlers

import (
	"strconv"

	"github.com/ibnu-hafidz/web-v2/internal/models"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// HalaqohStatsHandler provides handlers for halaqoh attendance statistics.
type HalaqohStatsHandler struct {
	db *gorm.DB
}

// NewHalaqohStatsHandler creates a new HalaqohStatsHandler.
func NewHalaqohStatsHandler(db *gorm.DB) *HalaqohStatsHandler {
	return &HalaqohStatsHandler{db: db}
}

// StudentStatistics returns halaqoh attendance statistics for students.
func (h *HalaqohStatsHandler) StudentStatistics(c *fiber.Ctx) error {
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	teacherID := c.Query("teacher_id")
	gender := c.Query("gender")

	// Base query
	query := h.db.Model(&models.HalaqohAttendance{}).
		Preload("HalaqohAssignment").
		Preload("HalaqohAssignment.Student").
		Preload("HalaqohAssignment.Teacher")

	if startDate != "" && endDate != "" {
		query = query.Where("date BETWEEN ? AND ?", startDate, endDate)
	}

	if teacherID != "" {
		query = query.Joins("JOIN halaqoh_assignments ON halaqoh_assignments.id = halaqoh_attendances.halaqoh_assignment_id").
			Where("halaqoh_assignments.user_id = ?", teacherID)
	}

	if gender != "" {
		// Need to join through halaqoh_assignments to students if teacherID wasn't joined
		if teacherID == "" {
			query = query.Joins("JOIN halaqoh_assignments ON halaqoh_assignments.id = halaqoh_attendances.halaqoh_assignment_id")
		}
		query = query.Joins("JOIN students ON students.id = halaqoh_assignments.student_id").
			Where("students.jenis_kelamin = ?", gender)
	}

	var records []models.HalaqohAttendance
	if err := query.Find(&records).Error; err != nil {
		return c.Status(500).JSON(models.ErrorResponse{
			Error:   "Database error",
			Message: "Failed to fetch student statistics",
		})
	}

	hadir, izin, sakit, alpa := 0, 0, 0, 0
	izinStudents := make(map[string]bool)
	sakitStudents := make(map[string]bool)
	alpaStudents := make(map[string]bool)

	for _, record := range records {
		switch record.Status {
		case "hadir":
			hadir++
		case "izin":
			izin++
			if record.HalaqohAssignment.Student.ID != 0 {
				izinStudents[record.HalaqohAssignment.Student.NamaLengkap] = true
			}
		case "sakit":
			sakit++
			if record.HalaqohAssignment.Student.ID != 0 {
				sakitStudents[record.HalaqohAssignment.Student.NamaLengkap] = true
			}
		case "alpa":
			alpa++
			if record.HalaqohAssignment.Student.ID != 0 {
				alpaStudents[record.HalaqohAssignment.Student.NamaLengkap] = true
			}
		}
	}

	// Helper to extract keys
	getKeys := func(m map[string]bool) []string {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		return keys
	}

	chartData := []map[string]interface{}{
		{"name": "Hadir", "value": hadir},
		{"name": "Izin", "value": izin},
		{"name": "Sakit", "value": sakit},
		{"name": "Alpa", "value": alpa},
	}

	return c.JSON(fiber.Map{
		"chartData":     chartData,
		"izinStudents":  getKeys(izinStudents),
		"sakitStudents": getKeys(sakitStudents),
		"alpaStudents":  getKeys(alpaStudents),
	})
}

// TeacherStatistics returns halaqoh attendance statistics for teachers.
func (h *HalaqohStatsHandler) TeacherStatistics(c *fiber.Ctx) error {
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	teacherIDStr := c.Query("teacher_id")
	gender := c.Query("gender")

	// 1. Fetch Teacher Attendances
	attQuery := h.db.Model(&models.HalaqohTeacherAttendance{}).Preload("Teacher")
	if startDate != "" && endDate != "" {
		attQuery = attQuery.Where("date BETWEEN ? AND ?", startDate, endDate)
	}
	if teacherIDStr != "" {
		attQuery = attQuery.Where("user_id = ?", teacherIDStr)
	}
	if gender != "" {
		attQuery = attQuery.Joins("JOIN users ON users.id = halaqoh_teacher_attendances.user_id").
			Where("users.gender = ?", gender)
	}

	var attendances []models.HalaqohTeacherAttendance
	if err := attQuery.Find(&attendances).Error; err != nil {
		return c.Status(500).JSON(models.ErrorResponse{Error: "Failed to fetch teacher attendances"})
	}

	// 2. Fetch Substitute Logs
	subQuery := h.db.Model(&models.HalaqohSubstituteLog{}).
		Preload("OriginalTeacher").
		Preload("SubstituteTeacher")
	if startDate != "" && endDate != "" {
		subQuery = subQuery.Where("date BETWEEN ? AND ?", startDate, endDate)
	}
	if teacherIDStr != "" {
		subQuery = subQuery.Where("original_teacher_id = ? OR substitute_teacher_id = ?", teacherIDStr, teacherIDStr)
	}
	if gender != "" {
		// Matching Laravel logic: filter if either original or substitute matches gender
		subQuery = subQuery.Joins("LEFT JOIN users as orig ON orig.id = halaqoh_substitute_logs.original_teacher_id").
			Joins("LEFT JOIN users as sub ON sub.id = halaqoh_substitute_logs.substitute_teacher_id").
			Where("orig.gender = ? OR sub.gender = ?", gender, gender)
	}

	var subLogs []models.HalaqohSubstituteLog
	if err := subQuery.Find(&subLogs).Error; err != nil {
		return c.Status(500).JSON(models.ErrorResponse{Error: "Failed to fetch substitute logs"})
	}

	// 3. Aggregate Stats
	stats := map[string]int{"Hadir": 0, "Izin": 0, "Sakit": 0, "Alpha": 0}
	sessions := []string{"Shubuh", "Ashar", "Isya"}
	sessionStats := make(map[string]map[string]int)
	for _, s := range sessions {
		sessionStats[s] = map[string]int{"Hadir": 0, "Izin": 0, "Sakit": 0, "Alpha": 0}
	}
	substitutionCount := 0

	type TeacherSummary struct {
		ID         uint   `json:"id"`
		Name       string `json:"name"`
		Hadir      int    `json:"hadir"`
		Izin       int    `json:"izin"`
		Sakit      int    `json:"sakit"`
		Alpha      int    `json:"alpha"`
		Substitute int    `json:"substitute"`
	}
	teacherMap := make(map[uint]*TeacherSummary)

	getTeacher := func(id uint, name string) *TeacherSummary {
		if _, exists := teacherMap[id]; !exists {
			teacherMap[id] = &TeacherSummary{ID: id, Name: name}
		}
		return teacherMap[id]
	}

	type SubstituteHistory struct {
		ID        uint   `json:"id"`
		Tanggal   string `json:"tanggal"`
		Sesi      string `json:"sesi"`
		GuruAsli  string `json:"guru_asli"`
		Status    string `json:"status"`
		Pengganti string `json:"pengganti"`
	}
	var history []SubstituteHistory

	var teacherID uint
	if teacherIDStr != "" {
		tID, _ := strconv.ParseUint(teacherIDStr, 10, 32)
		teacherID = uint(tID)
	}

	// Process attendances
	for _, a := range attendances {
		st := capitalize(a.Status)

		// Global
		stats[st]++
		if _, ok := sessionStats[a.Session]; ok {
			sessionStats[a.Session][st]++
		}

		// Per Teacher
		if a.Teacher.ID != 0 {
			ts := getTeacher(a.Teacher.ID, a.Teacher.Name)
			switch st {
			case "Hadir":
				ts.Hadir++
			case "Izin":
				ts.Izin++
			case "Sakit":
				ts.Sakit++
			case "Alpha":
				ts.Alpha++
			}
		}
	}

	// Process substitute logs
	for _, log := range subLogs {
		st := ""
		if log.Status != nil {
			st = capitalize(*log.Status)
		}
		sess := ""
		if log.Session != nil {
			sess = *log.Session
		}

		// Global counts
		if teacherID == 0 || log.OriginalTeacherID == teacherID {
			stats[st]++
			if _, ok := sessionStats[sess]; ok {
				sessionStats[sess][st]++
			}
		}
		if teacherID == 0 || log.SubstituteTeacherID == teacherID {
			substitutionCount++
		}

		// Per teacher (Original Teacher)
		if log.OriginalTeacher.ID != 0 {
			ts := getTeacher(log.OriginalTeacher.ID, log.OriginalTeacher.Name)
			switch st {
			case "Hadir":
				ts.Hadir++
			case "Izin":
				ts.Izin++
			case "Sakit":
				ts.Sakit++
			case "Alpha":
				ts.Alpha++
			}
		}

		// Per teacher (Substitute)
		penggantiName := "-"
		if log.SubstituteTeacher.ID != 0 {
			ts := getTeacher(log.SubstituteTeacher.ID, log.SubstituteTeacher.Name)
			ts.Substitute++
			penggantiName = log.SubstituteTeacher.Name
		}

		// Formatted history
		statusLabel := "-"
		if log.Status != nil {
			statusLabel = *log.Status
		}
		history = append(history, SubstituteHistory{
			ID:        log.ID,
			Tanggal:   log.Date.Format("2006-01-02"),
			Sesi:      sess,
			GuruAsli:  log.OriginalTeacher.Name,
			Status:    statusLabel,
			Pengganti: penggantiName,
		})
	}

	// Prepare summaries array
	var teacherSummaries []TeacherSummary
	for _, ts := range teacherMap {
		teacherSummaries = append(teacherSummaries, *ts)
	}

	// Absence History (Izin, Sakit, Alpha) for table display
	type AbsenceEntry struct {
		ID      uint   `json:"id"`
		Date    string `json:"date"`
		Teacher string `json:"teacher"`
		Session string `json:"session"`
		Status  string `json:"status"`
		Notes   string `json:"notes"`
		Source  string `json:"source,omitempty"`
	}
	var absenceHistory []AbsenceEntry
	for _, a := range attendances {
		if a.Status != "Hadir" {
			notes := ""
			if a.Notes != nil {
				notes = *a.Notes
			}
			tName := ""
			if a.Teacher.ID != 0 {
				tName = a.Teacher.Name
			}
			absenceHistory = append(absenceHistory, AbsenceEntry{
				ID:      a.ID,
				Date:    a.Date.Format("2006-01-02"),
				Teacher: tName,
				Session: a.Session,
				Status:  a.Status,
				Notes:   notes,
				Source:  "teacher_attendance",
			})
		}
	}

	for _, log := range subLogs {
		if log.Status == nil {
			continue
		}
		status := capitalize(*log.Status)
		if status == "Hadir" || status == "" {
			continue
		}
		if teacherID != 0 && log.OriginalTeacherID != teacherID {
			continue
		}

		notes := ""
		if log.Reason != nil {
			notes = *log.Reason
		}
		sess := ""
		if log.Session != nil {
			sess = *log.Session
		}
		absenceHistory = append(absenceHistory, AbsenceEntry{
			ID:      log.ID,
			Date:    log.Date.Format("2006-01-02"),
			Teacher: log.OriginalTeacher.Name,
			Session: sess,
			Status:  status,
			Notes:   notes,
			Source:  "substitute_log",
		})
	}

	return c.JSON(fiber.Map{
		"stats":             stats,
		"sessionStats":      sessionStats,
		"substitutionCount": substitutionCount,
		"teachers":          teacherSummaries,
		"history":           history,
		"absence_history":   absenceHistory,
	})
}

// Helper to capitalize status strings
func capitalize(s string) string {
	if len(s) == 0 {
		return ""
	}
	// Note: Laravel uses "Alpa" for students, but "Alpha" in teacher stats often. Normalize to Alpha.
	if s == "alpa" || s == "Alpa" {
		return "Alpha"
	}
	if s == "hadir" {
		return "Hadir"
	}
	if s == "izin" {
		return "Izin"
	}
	if s == "sakit" {
		return "Sakit"
	}
	return s
}
