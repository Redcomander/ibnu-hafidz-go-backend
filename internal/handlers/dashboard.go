package handlers

import (
	"sort"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"gorm.io/gorm"
)

type DashboardHandler struct {
	db *gorm.DB
}

func NewDashboardHandler(db *gorm.DB) *DashboardHandler {
	return &DashboardHandler{db: db}
}

type attendanceCountSummary struct {
	Hadir int64 `gorm:"column:hadir"`
	Izin  int64 `gorm:"column:izin"`
	Sakit int64 `gorm:"column:sakit"`
	Alpha int64 `gorm:"column:alpha"`
}

type dashboardAttendanceSummary struct {
	Hadir int64
	Izin  int64
	Sakit int64
	Alpha int64
}

func summarizeAttendance(query *gorm.DB) attendanceCountSummary {
	var summary attendanceCountSummary
	query.Select(`
		COALESCE(SUM(CASE WHEN LOWER(status) = 'hadir' THEN 1 ELSE 0 END), 0) AS hadir,
		COALESCE(SUM(CASE WHEN LOWER(status) = 'izin' THEN 1 ELSE 0 END), 0) AS izin,
		COALESCE(SUM(CASE WHEN LOWER(status) = 'sakit' THEN 1 ELSE 0 END), 0) AS sakit,
		COALESCE(SUM(CASE WHEN LOWER(status) IN ('alpha', 'tidak_hadir') THEN 1 ELSE 0 END), 0) AS alpha
	`).Scan(&summary)
	return summary
}

func getTeacherAttendanceSummaryWithSubstitutes(db *gorm.DB, monthStart, monthEnd string, attendanceType string, teacherID *uint) dashboardAttendanceSummary {
	summary := dashboardAttendanceSummary{}

	baseQuery := db.Model(&models.TeacherAttendance{}).Where("date >= ? AND date < ?", monthStart, monthEnd)
	substituteQuery := db.Table("substitute_logs")
	statusQuery := db.Table("substitute_logs")

	switch attendanceType {
	case "diniyyah":
		baseQuery = baseQuery.Where("jadwal_diniyyah_id IS NOT NULL")
		substituteQuery = db.Table("substitute_logs_diniyyah").Where("date >= ? AND date < ?", monthStart, monthEnd)
		statusQuery = db.Table("substitute_logs_diniyyah").Where("date >= ? AND date < ?", monthStart, monthEnd)
		if teacherID != nil {
			baseQuery = baseQuery.Where("user_id = ?", *teacherID)
			substituteQuery = substituteQuery.Where("substitute_teacher_id = ?", *teacherID)
			statusQuery = statusQuery.Where("original_teacher_id = ?", *teacherID)
		}
	default:
		baseQuery = baseQuery.Where("jadwal_formal_id IS NOT NULL")
		substituteQuery = substituteQuery.
			Where("date >= ? AND date < ?", monthStart, monthEnd).
			Where("deleted_at IS NULL").
			Where("jadwal_formal_id IS NOT NULL")
		statusQuery = statusQuery.
			Where("date >= ? AND date < ?", monthStart, monthEnd).
			Where("deleted_at IS NULL").
			Where("jadwal_formal_id IS NOT NULL")
		if teacherID != nil {
			baseQuery = baseQuery.Where("user_id = ?", *teacherID)
			substituteQuery = substituteQuery.Where("substitute_teacher_id = ?", *teacherID)
			statusQuery = statusQuery.Where("original_teacher_id = ?", *teacherID)
		}
	}

	base := summarizeAttendance(baseQuery)
	summary.Hadir = base.Hadir
	summary.Izin = base.Izin
	summary.Sakit = base.Sakit
	summary.Alpha = base.Alpha

	var substituteCount int64
	substituteQuery.Count(&substituteCount)
	summary.Hadir += substituteCount

	statusSummary := summarizeAttendance(statusQuery)
	summary.Izin += statusSummary.Izin
	summary.Sakit += statusSummary.Sakit
	summary.Alpha += statusSummary.Alpha

	return summary
}

func countLatestSubstituteLogs(db *gorm.DB, monthStart, monthEnd string, attendanceType string, teacherID *uint) int64 {
	latestIDs := db.Model(&models.SubstituteLog{}).
		Select("MAX(id)").
		Where("date >= ? AND date < ?", monthStart, monthEnd)

	switch attendanceType {
	case "diniyyah":
		latestIDs = latestIDs.
			Where("jadwal_diniyyah_id IS NOT NULL").
			Group("date, jadwal_diniyyah_id")
	default:
		latestIDs = latestIDs.
			Where("jadwal_formal_id IS NOT NULL").
			Group("date, jadwal_formal_id")
	}

	query := db.Model(&models.SubstituteLog{}).Where("id IN (?)", latestIDs)
	if teacherID != nil {
		query = query.Where("substitute_teacher_id = ?", *teacherID)
	}

	var count int64
	query.Count(&count)
	return count
}

func countScheduleSubstitutes(db *gorm.DB, monthStart, monthEnd string, attendanceType string, teacherID *uint) int64 {
	var count int64

	switch attendanceType {
	case "diniyyah":
		query := db.Model(&models.DiniyyahSchedule{}).
			Where("substitute_teacher_id IS NOT NULL AND substitute_date >= ? AND substitute_date < ?", monthStart, monthEnd)
		if teacherID != nil {
			query = query.Where("substitute_teacher_id = ?", *teacherID)
		}
		query.Count(&count)
	default:
		query := db.Model(&models.Schedule{}).
			Where("substitute_teacher_id IS NOT NULL AND substitute_date >= ? AND substitute_date < ?", monthStart, monthEnd)
		if teacherID != nil {
			query = query.Where("substitute_teacher_id = ?", *teacherID)
		}
		query.Count(&count)
	}

	return count
}

func countLegacyDiniyyahSubstituteLogs(db *gorm.DB, monthStart, monthEnd string, teacherID *uint) int64 {
	query := db.Table("substitute_logs_diniyyah").
		Where("date >= ? AND date < ?", monthStart, monthEnd)
	if teacherID != nil {
		query = query.Where("substitute_teacher_id = ?", *teacherID)
	}

	var count int64
	query.Count(&count)
	return count
}

func countActiveHalaqohSubstituteLogs(db *gorm.DB, monthStart, monthEnd string, teacherID *uint) int64 {
	query := db.Model(&models.HalaqohSubstituteLog{}).
		Where("is_active = ? AND date >= ? AND date < ?", true, monthStart, monthEnd)
	if teacherID != nil {
		query = query.Where("substitute_teacher_id = ?", *teacherID)
	}

	var count int64
	query.Count(&count)
	return count
}

type DashboardSubstituteRecord struct {
	Type            string `json:"type"`
	Date            string `json:"date"`
	Lesson          string `json:"lesson"`
	Kelas           string `json:"kelas"`
	Tingkat         string `json:"tingkat"`
	OriginalTeacher string `json:"original_teacher"`
	Reason          string `json:"reason"`
	Session         string `json:"session"`
}

func getSubstituteRecords(db *gorm.DB, userID uint, monthStart string, monthEnd string) []DashboardSubstituteRecord {
	var records []DashboardSubstituteRecord

	// 1. Formal Substitute Logs
	var formalSubs []models.SubstituteLog
	db.Preload("JadwalFormal.Assignment.Lesson").
		Preload("JadwalFormal.Assignment.Kelas").
		Preload("OriginalTeacher").
		Where("substitute_teacher_id = ? AND date >= ? AND date < ?", userID, monthStart, monthEnd).
		Find(&formalSubs)

	for _, sub := range formalSubs {
		lessonName := "-"
		kelasName := "-"
		tingkat := "-"
		if sub.JadwalFormal != nil {
			if sub.JadwalFormal.Assignment.Lesson != nil {
				lessonName = sub.JadwalFormal.Assignment.Lesson.Name
			}
			if sub.JadwalFormal.Assignment.Kelas != nil {
				kelasName = sub.JadwalFormal.Assignment.Kelas.Nama
				tingkat = sub.JadwalFormal.Assignment.Kelas.Tingkat
			}
		}

		reason := sub.Reason
		if reason == "" {
			reason = "-"
		}

		records = append(records, DashboardSubstituteRecord{
			Type:            "Formal",
			Date:            sub.Date.Format("2006-01-02"),
			Lesson:          lessonName,
			Kelas:           kelasName,
			Tingkat:         tingkat,
			OriginalTeacher: sub.OriginalTeacher.Name,
			Reason:          reason,
			Session:         "",
		})
	}

	// 2. Diniyyah Substitute Logs
	var diniyyahSubs []models.SubstituteDiniyyahLog
	db.Preload("JadwalDiniyyah.Assignment.DiniyyahLesson").
		Preload("JadwalDiniyyah.Assignment.Kelas").
		Preload("OriginalTeacher").
		Where("substitute_teacher_id = ? AND date >= ? AND date < ?", userID, monthStart, monthEnd).
		Find(&diniyyahSubs)

	for _, sub := range diniyyahSubs {
		lessonName := "-"
		kelasName := "-"
		tingkat := "-"
		if sub.JadwalDiniyyah != nil {
			if sub.JadwalDiniyyah.Assignment.DiniyyahLesson != nil {
				lessonName = sub.JadwalDiniyyah.Assignment.DiniyyahLesson.Name
			}
			if sub.JadwalDiniyyah.Assignment.Kelas != nil {
				kelasName = sub.JadwalDiniyyah.Assignment.Kelas.Nama
				tingkat = sub.JadwalDiniyyah.Assignment.Kelas.Tingkat
			}
		}

		reason := sub.Reason
		if reason == "" {
			reason = "-"
		}

		records = append(records, DashboardSubstituteRecord{
			Type:            "Diniyyah",
			Date:            sub.Date.Format("2006-01-02"),
			Lesson:          lessonName,
			Kelas:           kelasName,
			Tingkat:         tingkat,
			OriginalTeacher: sub.OriginalTeacher.Name,
			Reason:          reason,
			Session:         "",
		})
	}

	// 3. Halaqoh Substitute Logs
	var halaqohSubs []models.HalaqohSubstituteLog
	db.Preload("Assignment.Teacher").
		Where("substitute_teacher_id = ? AND date >= ? AND date < ?", userID, monthStart, monthEnd).
		Find(&halaqohSubs)

	for _, sub := range halaqohSubs {
		sessionVal := "All"
		if sub.Session != nil {
			sessionVal = *sub.Session
		}

		reason := "-"
		if sub.Reason != nil && *sub.Reason != "" {
			reason = *sub.Reason
		}

		origTeacher := "-"
		if sub.Assignment.Teacher.Name != "" {
			origTeacher = sub.Assignment.Teacher.Name
		} else if sub.OriginalTeacher.Name != "" {
			origTeacher = sub.OriginalTeacher.Name
		}

		records = append(records, DashboardSubstituteRecord{
			Type:            "Halaqoh",
			Date:            sub.Date.Format("2006-01-02"),
			Lesson:          "Halaqoh",
			Kelas:           "-",
			Tingkat:         "-",
			OriginalTeacher: origTeacher,
			Reason:          reason,
			Session:         sessionVal,
		})
	}

	// Sort records by date ascending
	sort.Slice(records, func(i, j int) bool {
		return records[i].Date < records[j].Date
	})

	return records
}

// Stats returns dashboard statistics based on user role
func (h *DashboardHandler) Stats(c *fiber.Ctx) error {
	user, ok := c.Locals("user").(*models.User)
	if !ok || user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "User not authenticated"})
	}

	isAdmin := user.HasRole("admin") || user.HasRole("super_admin")
	isTeacher := user.HasRole("teacher") || user.HasRole("guru") || user.HasRole("musyrif")

	// Get today's day name in Indonesian (lowercase for DB match)
	today := time.Now()
	todayStart := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
	todayEnd := todayStart.AddDate(0, 0, 1)
	days := []string{"ahad", "senin", "selasa", "rabu", "kamis", "jumat", "sabtu"}
	hariIni := days[today.Weekday()]

	// Accept optional month/year query params for period filtering
	qMonth := c.QueryInt("month", int(today.Month()))
	qYear := c.QueryInt("year", today.Year())

	// Compute date range for the selected period
	monthStartTime := time.Date(qYear, time.Month(qMonth), 1, 0, 0, 0, 0, today.Location())
	monthEndTime := monthStartTime.AddDate(0, 1, 0)
	monthStart := monthStartTime.Format("2006-01-02")
	monthEnd := monthEndTime.Format("2006-01-02")

	response := fiber.Map{}

	// ==========================================
	// ADMIN STATS (Global Data)
	// ==========================================
	if isAdmin {
		var studentCount, userCount, kelasCount, kamarCount, articleCount, prestasiCount, vendorCount int64
		var totalVisitors, visitorsThisMonth, visitorsLastMonth int64

		h.db.Model(&models.Student{}).Count(&studentCount)
		h.db.Model(&models.User{}).Count(&userCount)
		h.db.Model(&models.Kelas{}).Count(&kelasCount)
		h.db.Model(&models.Kamar{}).Count(&kamarCount)
		h.db.Model(&models.Prestasi{}).Count(&prestasiCount)
		h.db.Table("articles").Where("status = ?", "published").Count(&articleCount)
		h.db.Table("laundry_accounts").Where("is_vendor = ?", true).Count(&vendorCount)

		// Visitor Analytics
		h.db.Model(&models.Visitor{}).Count(&totalVisitors)
		h.db.Model(&models.Visitor{}).Where("created_at >= ? AND created_at < ?", monthStartTime, monthEndTime).Count(&visitorsThisMonth)

		lastMonthStartTime := monthStartTime.AddDate(0, -1, 0)
		lastMonthEndTime := monthStartTime
		h.db.Model(&models.Visitor{}).Where("created_at >= ? AND created_at < ?", lastMonthStartTime, lastMonthEndTime).Count(&visitorsLastMonth)

		visitorTrend := 0.0
		if visitorsLastMonth > 0 {
			visitorTrend = float64(visitorsThisMonth-visitorsLastMonth) / float64(visitorsLastMonth) * 100
		} else if visitorsThisMonth > 0 {
			visitorTrend = 100.0
		}

		// 1. Global Formal Schedule for Today
		var schedules []models.Schedule
		h.db.Preload("Assignment.Teacher").
			Preload("Assignment.Lesson").
			Preload("Assignment.Kelas").
			Where("hari = ?", hariIni).
			Order("jam_mulai asc").
			Limit(5).
			Find(&schedules)

		// 2. Global Today's Attendance Summary
		todayAttendance := summarizeAttendance(
			h.db.Model(&models.TeacherAttendance{}).Where("date >= ? AND date < ?", todayStart, todayEnd),
		)

		// 3. Recent Published Articles
		var recentArticles []models.Article
		h.db.Preload("Author").Preload("Category").
			Where("status = ?", "published").
			Order("created_at desc").
			Limit(4).
			Find(&recentArticles)

		response["stats"] = fiber.Map{
			"total_students": studentCount,
			"total_teachers": userCount,
			"total_kelas":    kelasCount,
			"total_kamars":   kamarCount,
			"total_articles": articleCount,
			"total_prestasi": prestasiCount,
			"total_vendors":  vendorCount,
			"visitors": fiber.Map{
				"total":      totalVisitors,
				"this_month": visitorsThisMonth,
				"last_month": visitorsLastMonth,
				"trend":      visitorTrend,
			},
		}
		response["schedule"] = fiber.Map{
			"hari":  hariIni,
			"items": schedules,
		}
		response["attendance"] = fiber.Map{
			"hadir": todayAttendance.Hadir,
			"izin":  todayAttendance.Izin,
			"sakit": todayAttendance.Sakit,
			"alpa":  todayAttendance.Alpha,
			"total": todayAttendance.Hadir + todayAttendance.Izin + todayAttendance.Sakit + todayAttendance.Alpha,
		}
		response["recent_articles"] = recentArticles
	}

	// ==========================================
	// GLOBAL ATTENDANCE (Formal, Diniyyah, Halaqoh)
	// ==========================================
	globalFormal := getTeacherAttendanceSummaryWithSubstitutes(h.db, monthStart, monthEnd, "formal", nil)
	gFPengganti := countScheduleSubstitutes(h.db, monthStart, monthEnd, "formal", nil)
	if logCount := countLatestSubstituteLogs(h.db, monthStart, monthEnd, "formal", nil); logCount > gFPengganti {
		gFPengganti = logCount
	}
	gFHadir, gFIzin, gFSakit, gFAlpa := globalFormal.Hadir, globalFormal.Izin, globalFormal.Sakit, globalFormal.Alpha
	gFTotal := gFHadir + gFIzin + gFSakit + gFAlpa

	globalDiniyyah := getTeacherAttendanceSummaryWithSubstitutes(h.db, monthStart, monthEnd, "diniyyah", nil)
	gdPengganti := countScheduleSubstitutes(h.db, monthStart, monthEnd, "diniyyah", nil)
	if logCount := countLatestSubstituteLogs(h.db, monthStart, monthEnd, "diniyyah", nil); logCount > gdPengganti {
		gdPengganti = logCount
	}
	if legacyLogCount := countLegacyDiniyyahSubstituteLogs(h.db, monthStart, monthEnd, nil); legacyLogCount > gdPengganti {
		gdPengganti = legacyLogCount
	}
	gdHadir, gdIzin, gdSakit, gdAlpa := globalDiniyyah.Hadir, globalDiniyyah.Izin, globalDiniyyah.Sakit, globalDiniyyah.Alpha
	gdTotal := gdHadir + gdIzin + gdSakit + gdAlpa

	globalHalaqoh := summarizeAttendance(
		h.db.Model(&models.HalaqohTeacherAttendance{}).Where("date >= ? AND date < ?", monthStart, monthEnd),
	)
	ghPengganti := countActiveHalaqohSubstituteLogs(h.db, monthStart, monthEnd, nil)
	ghHadir, ghIzin, ghSakit, ghAlpa := globalHalaqoh.Hadir, globalHalaqoh.Izin, globalHalaqoh.Sakit, globalHalaqoh.Alpha
	ghTotal := ghHadir + ghIzin + ghSakit + ghAlpa

	calcGlobPct := func(hadir, total int64) float64 {
		if total == 0 {
			return 0.0
		}
		return float64(hadir) / float64(total) * 100
	}

	response["global_attendance"] = fiber.Map{
		"formal": fiber.Map{
			"hadir":      gFHadir,
			"izin":       gFIzin,
			"sakit":      gFSakit,
			"alpha":      gFAlpa,
			"pengganti":  gFPengganti,
			"total":      gFTotal,
			"percentage": calcGlobPct(gFHadir, gFTotal),
		},
		"diniyyah": fiber.Map{
			"hadir":      gdHadir,
			"izin":       gdIzin,
			"sakit":      gdSakit,
			"alpha":      gdAlpa,
			"pengganti":  gdPengganti,
			"total":      gdTotal,
			"percentage": calcGlobPct(gdHadir, gdTotal),
		},
		"halaqoh": fiber.Map{
			"hadir":      ghHadir,
			"izin":       ghIzin,
			"sakit":      ghSakit,
			"alpha":      ghAlpa,
			"pengganti":  ghPengganti,
			"total":      ghTotal,
			"percentage": calcGlobPct(ghHadir, ghTotal),
		},
	}

	// ==========================================
	// PERSONAL STATS (for any logged-in user who has schedules)
	// ==========================================
	// 1. Personal Schedule for Today
	var personalSchedules []models.Schedule
	h.db.Preload("Assignment").
		Preload("Assignment.Lesson").
		Preload("Assignment.Kelas").
		Joins("JOIN lesson_kelas_teachers ON lesson_kelas_teachers.id = jadwal_formal.lesson_kelas_teacher_id").
		Where("jadwal_formal.hari = ? AND lesson_kelas_teachers.user_id = ?", hariIni, user.ID).
		Order("jadwal_formal.jam_mulai asc").
		Find(&personalSchedules)

	var personalDiniyyahSchedules []models.DiniyyahSchedule
	h.db.Preload("Assignment").
		Preload("Assignment.DiniyyahLesson").
		Preload("Assignment.Kelas").
		Joins("JOIN diniyyah_kelas_teachers dkt ON dkt.id = jadwal_diniyyahs.diniyyah_kelas_teacher_id").
		Where("jadwal_diniyyahs.hari = ? AND dkt.user_id = ?", hariIni, user.ID).
		Order("jadwal_diniyyahs.jam_mulai asc").
		Find(&personalDiniyyahSchedules)

		// 2. Personal Attendance Stats Breakdown

	// 2a. Formal Stats
	personalFormal := getTeacherAttendanceSummaryWithSubstitutes(h.db, monthStart, monthEnd, "formal", &user.ID)
	fPengganti := countScheduleSubstitutes(h.db, monthStart, monthEnd, "formal", &user.ID)
	if logCount := countLatestSubstituteLogs(h.db, monthStart, monthEnd, "formal", &user.ID); logCount > fPengganti {
		fPengganti = logCount
	}
	fHadir, fIzin, fSakit, fAlpa := personalFormal.Hadir, personalFormal.Izin, personalFormal.Sakit, personalFormal.Alpha
	fTotal := fHadir + fIzin + fSakit + fAlpa

	// 2b. Diniyyah Stats
	personalDiniyyah := getTeacherAttendanceSummaryWithSubstitutes(h.db, monthStart, monthEnd, "diniyyah", &user.ID)
	dPengganti := countScheduleSubstitutes(h.db, monthStart, monthEnd, "diniyyah", &user.ID)
	if logCount := countLatestSubstituteLogs(h.db, monthStart, monthEnd, "diniyyah", &user.ID); logCount > dPengganti {
		dPengganti = logCount
	}
	if legacyLogCount := countLegacyDiniyyahSubstituteLogs(h.db, monthStart, monthEnd, &user.ID); legacyLogCount > dPengganti {
		dPengganti = legacyLogCount
	}
	dHadir, dIzin, dSakit, dAlpa := personalDiniyyah.Hadir, personalDiniyyah.Izin, personalDiniyyah.Sakit, personalDiniyyah.Alpha
	dTotal := dHadir + dIzin + dSakit + dAlpa

	// 2c. Halaqoh Stats
	personalHalaqoh := summarizeAttendance(
		h.db.Model(&models.HalaqohTeacherAttendance{}).Where("user_id = ? AND date >= ? AND date < ?", user.ID, monthStart, monthEnd),
	)
	hPengganti := countActiveHalaqohSubstituteLogs(h.db, monthStart, monthEnd, &user.ID)
	hHadir, hIzin, hSakit, hAlpa := personalHalaqoh.Hadir, personalHalaqoh.Izin, personalHalaqoh.Sakit, personalHalaqoh.Alpha
	hTotal := hHadir + hIzin + hSakit + hAlpa

	// Calculate Percentages safely
	calcPct := func(hadir, total int64) float64 {
		if total == 0 {
			return 0.0
		}
		return float64(hadir) / float64(total) * 100
	}

	fPct := calcPct(fHadir, fTotal)
	dPct := calcPct(dHadir, dTotal)
	hPct := calcPct(hHadir, hTotal)

	// 3. Combined Teacher Recap (for the bottom panel)
	combHadir := fHadir + dHadir + hHadir
	combIzin := fIzin + dIzin + hIzin
	combSakit := fSakit + dSakit + hSakit
	combAlpa := fAlpa + dAlpa + hAlpa
	combPengganti := fPengganti + dPengganti + hPengganti
	combTotal := fTotal + dTotal + hTotal
	combPct := calcPct(combHadir, combTotal)

	// 4. Substitute Records List
	substituteRecords := getSubstituteRecords(h.db, user.ID, monthStart, monthEnd)

	response["teacher"] = fiber.Map{
		"personal_schedule":          personalSchedules,
		"personal_diniyyah_schedule": personalDiniyyahSchedules,
		"substitute_records":         substituteRecords,
		"formal_stats": fiber.Map{
			"hadir":      fHadir,
			"izin":       fIzin,
			"sakit":      fSakit,
			"alpha":      fAlpa,
			"pengganti":  fPengganti,
			"total":      fTotal,
			"percentage": fPct,
		},
		"diniyyah_stats": fiber.Map{
			"hadir":      dHadir,
			"izin":       dIzin,
			"sakit":      dSakit,
			"alpha":      dAlpa,
			"pengganti":  dPengganti,
			"total":      dTotal,
			"percentage": dPct,
		},
		"halaqoh_stats": fiber.Map{
			"hadir":      hHadir,
			"izin":       hIzin,
			"sakit":      hSakit,
			"alpha":      hAlpa,
			"pengganti":  hPengganti,
			"total":      hTotal,
			"percentage": hPct,
		},
		"teacher_recap": fiber.Map{
			"hadir":      combHadir,
			"izin":       combIzin,
			"sakit":      combSakit,
			"alpha":      combAlpa,
			"pengganti":  combPengganti,
			"total":      combTotal,
			"percentage": combPct,
		},
	}

	response["roles"] = fiber.Map{
		"is_admin":   isAdmin,
		"is_teacher": isTeacher,
	}

	return c.JSON(response)
}

// VisitorStats returns visitor chart data
func (h *DashboardHandler) VisitorStats(c *fiber.Ctx) error {
	days := c.QueryInt("days", 30)
	startDate := time.Now().AddDate(0, 0, -days)

	type DailyVisitor struct {
		Date  string `json:"date"`
		Count int64  `json:"count"`
	}

	var stats []DailyVisitor
	h.db.Table("visitors").
		Select("DATE(created_at) as date, COUNT(*) as count").
		Where("created_at >= ?", startDate).
		Group("DATE(created_at)").
		Order("date asc").
		Scan(&stats)

	return c.JSON(fiber.Map{
		"visitor_stats": stats,
		"period_days":   days,
	})
}

// DebugSchema returns table schema for debugging
func (h *DashboardHandler) DebugSchema(c *fiber.Ctx) error {
	var columns []struct {
		Field   string
		Type    string
		Null    string
		Key     string
		Default *string
		Extra   string
	}
	h.db.Raw("DESCRIBE jadwal_formal").Scan(&columns)
	return c.JSON(columns)
}
