package handlers

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"gorm.io/gorm"
)

// ──────────────────────────────────────────────────────────────
// HalaqohHandler
// ──────────────────────────────────────────────────────────────

type HalaqohHandler struct {
	db *gorm.DB
}

func NewHalaqohHandler(db *gorm.DB) *HalaqohHandler {
	return &HalaqohHandler{db: db}
}

// Session time windows — attendance can only be submitted within these windows
// unless user has bypass permission.
var sessionTimes = map[string][2]string{
	"Shubuh": {"04:30", "06:45"},
	"Ashar":  {"15:30", "17:30"},
	"Isya":   {"19:45", "22:15"},
}

// getUserFromContext safely retrieves the authenticated user.
func (h *HalaqohHandler) getUserFromContext(c *fiber.Ctx) (*models.User, error) {
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

// canBypassTimeRestriction checks if user is admin/tim_presensi/super_admin.
func canBypassTimeRestriction(user *models.User) bool {
	return user.HasPermission("halaqoh.view_all") ||
		user.HasRole("super_admin") ||
		user.HasRole("admin") ||
		user.HasRole("tim_presensi")
}

func (h *HalaqohHandler) canAccessAssignment(user *models.User, assignment *models.HalaqohAssignment, dateStr string) bool {
	if user == nil || assignment == nil {
		return false
	}
	if canBypassTimeRestriction(user) || user.ID == assignment.UserID {
		return true
	}
	if assignment.HelperTeacherID != nil && *assignment.HelperTeacherID == user.ID {
		return true
	}

	var count int64
	h.db.Model(&models.HalaqohSubstituteLog{}).
		Where("halaqoh_assignment_id = ? AND substitute_teacher_id = ? AND date = ? AND is_active = ?",
			assignment.ID, user.ID, dateStr, true).
		Count(&count)

	return count > 0
}

func (h *HalaqohHandler) canAccessTeacherAttendance(user *models.User, assignment *models.HalaqohAssignment, dateStr string) bool {
	if user == nil || assignment == nil {
		return false
	}
	if canBypassTimeRestriction(user) || user.ID == assignment.UserID {
		return true
	}

	var count int64
	h.db.Model(&models.HalaqohSubstituteLog{}).
		Where("halaqoh_assignment_id = ? AND substitute_teacher_id = ? AND date = ? AND is_active = ?",
			assignment.ID, user.ID, dateStr, true).
		Count(&count)

	return count > 0
}

func canManageHalaqohTeacherAttendance(user *models.User) bool {
	if user == nil {
		return false
	}

	return user.HasRole("super_admin") ||
		user.HasRole("admin") ||
		user.HasRole("staff") ||
		user.HasRole("tim_presensi") ||
		user.HasPermission("halaqoh.view_all") ||
		user.HasPermission("halaqoh-assignments.edit") ||
		user.HasPermission("halaqoh-assignments.delete")
}

// isWithinSessionTime checks if the current time is within the session window.
func isWithinSessionTime(session string) bool {
	times, ok := sessionTimes[session]
	if !ok {
		return false
	}
	now := time.Now()
	loc := now.Location()
	today := now.Format("2006-01-02")

	start, _ := time.ParseInLocation("2006-01-02 15:04", today+" "+times[0], loc)
	end, _ := time.ParseInLocation("2006-01-02 15:04", today+" "+times[1], loc)

	return now.After(start) && now.Before(end)
}

// ──────────────────────────────────────────────────────────────
// Assignment CRUD
// ──────────────────────────────────────────────────────────────

func (h *HalaqohHandler) ListAssignments(c *fiber.Ctx) error {
	user, err := h.getUserFromContext(c)
	if err != nil {
		return err
	}

	dateStr := c.Query("date")
	if dateStr == "" {
		dateStr = todayString()
	}

	canViewAll := canBypassTimeRestriction(user)

	// -- Session time info for frontend --
	sessions := []string{"Shubuh", "Ashar", "Isya"}
	type SessionTimeInfo struct {
		Session   string `json:"session"`
		Start     string `json:"start"`
		End       string `json:"end"`
		IsActive  bool   `json:"is_active"`
		CanBypass bool   `json:"can_bypass"`
	}
	var sessionTimeInfos []SessionTimeInfo
	for _, sess := range sessions {
		times := sessionTimes[sess]
		sessionTimeInfos = append(sessionTimeInfos, SessionTimeInfo{
			Session:   sess,
			Start:     times[0],
			End:       times[1],
			IsActive:  isWithinSessionTime(sess),
			CanBypass: canViewAll,
		})
	}

	type TeacherGroup struct {
		TeacherID   uint                       `json:"teacher_id"`
		TeacherName string                     `json:"teacher_name"`
		Assignments []models.HalaqohAssignment `json:"assignments"`
	}

	var assignments []models.HalaqohAssignment

	if canViewAll {
		h.db.Where("active = ?", true).
			Preload("Teacher").
			Preload("Student").
			Preload("HelperTeacher").
			Order("user_id, student_id").
			Find(&assignments)
	} else {
		var subTeacherIDs []uint
		h.db.Model(&models.HalaqohSubstituteLog{}).
			Where("substitute_teacher_id = ? AND date >= ? AND is_active = ?", user.ID, dateStr, true).
			Distinct().Pluck("original_teacher_id", &subTeacherIDs)

		if len(subTeacherIDs) == 0 {
			subTeacherIDs = []uint{0}
		}

		h.db.Where("active = ?", true).
			Where("user_id = ? OR helper_teacher_id = ? OR user_id IN ?", user.ID, user.ID, subTeacherIDs).
			Preload("Teacher").
			Preload("Student").
			Preload("HelperTeacher").
			Order("user_id, student_id").
			Find(&assignments)
	}

	// ── Batch: student attendance counts per (assignment, session) ──
	var assignmentIDs []uint
	for _, a := range assignments {
		assignmentIDs = append(assignmentIDs, a.ID)
	}

	type AttCountRow struct {
		HalaqohAssignmentID uint   `gorm:"column:halaqoh_assignment_id"`
		Session             string `gorm:"column:session"`
		Cnt                 int64  `gorm:"column:cnt"`
	}
	attCountMap := make(map[string]int64) // "assignmentID_session" -> count
	if len(assignmentIDs) > 0 {
		var rows []AttCountRow
		h.db.Model(&models.HalaqohAttendance{}).
			Select("halaqoh_assignment_id, session, COUNT(*) as cnt").
			Where("halaqoh_assignment_id IN ? AND date = ?", assignmentIDs, dateStr).
			Group("halaqoh_assignment_id, session").
			Scan(&rows)
		for _, r := range rows {
			attCountMap[fmt.Sprintf("%d_%s", r.HalaqohAssignmentID, r.Session)] = r.Cnt
		}
	}

	for i := range assignments {
		assignments[i].HasAttendanceToday = make(map[string]bool)
		for _, sess := range sessions {
			key := fmt.Sprintf("%d_%s", assignments[i].ID, sess)
			assignments[i].HasAttendanceToday[sess] = attCountMap[key] > 0
		}
	}

	// ── Group by teacher ──
	groupMap := make(map[uint]*TeacherGroup)
	var groupOrder []uint
	for _, a := range assignments {
		if _, ok := groupMap[a.UserID]; !ok {
			groupMap[a.UserID] = &TeacherGroup{
				TeacherID:   a.UserID,
				TeacherName: a.Teacher.Name,
				Assignments: []models.HalaqohAssignment{},
			}
			groupOrder = append(groupOrder, a.UserID)
		}
		groupMap[a.UserID].Assignments = append(groupMap[a.UserID].Assignments, a)
	}

	var groups []TeacherGroup
	for _, tid := range groupOrder {
		groups = append(groups, *groupMap[tid])
	}

	// ── Batch: student attendance status per (teacher, session) for badges ──
	// Compute: for each teacher group, total students and how many have attendance per session
	type StudentAttBadge struct {
		Session   string `json:"session"`
		Count     int64  `json:"count"`
		Total     int    `json:"total"`
		Completed bool   `json:"completed"`
		Partial   bool   `json:"partial"`
	}
	type TeacherBadgeInfo struct {
		TeacherID            uint              `json:"teacher_id"`
		StudentAttendance    []StudentAttBadge `json:"student_attendance"`
		TeacherAttendance    map[string]string `json:"teacher_attendance"`     // session -> status ("Hadir" etc) or ""
		HasTeacherAttendance map[string]bool   `json:"has_teacher_attendance"` // session -> bool
	}

	// Gather per-teacher student count and attendance count per session
	teacherBadges := make(map[uint]*TeacherBadgeInfo)
	for _, tid := range groupOrder {
		g := groupMap[tid]
		totalStudents := len(g.Assignments)

		var sessionBadges []StudentAttBadge
		for _, sess := range sessions {
			var count int64
			for _, a := range g.Assignments {
				key := fmt.Sprintf("%d_%s", a.ID, sess)
				if attCountMap[key] > 0 {
					count++
				}
			}
			sessionBadges = append(sessionBadges, StudentAttBadge{
				Session:   sess,
				Count:     count,
				Total:     totalStudents,
				Completed: count == int64(totalStudents) && totalStudents > 0,
				Partial:   count > 0 && count < int64(totalStudents),
			})
		}

		teacherBadges[tid] = &TeacherBadgeInfo{
			TeacherID:            tid,
			StudentAttendance:    sessionBadges,
			TeacherAttendance:    make(map[string]string),
			HasTeacherAttendance: make(map[string]bool),
		}
	}

	// ── Batch: teacher attendance per session for today ── (fixed GROUP BY)
	if len(groupOrder) > 0 {
		type TARow struct {
			UserID  uint   `gorm:"column:user_id"`
			Session string `gorm:"column:session"`
			Status  string `gorm:"column:status"`
		}
		var taRows []TARow
		h.db.Model(&models.HalaqohTeacherAttendance{}).
			Select("user_id, session, status").
			Where("user_id IN ? AND date = ?", groupOrder, dateStr).
			Find(&taRows)

		for _, r := range taRows {
			if badge, ok := teacherBadges[r.UserID]; ok {
				badge.TeacherAttendance[r.Session] = r.Status
				badge.HasTeacherAttendance[r.Session] = true
			}
		}
	}

	var badgesList []TeacherBadgeInfo
	for _, tid := range groupOrder {
		badgesList = append(badgesList, *teacherBadges[tid])
	}

	// ── Batch: substitute info per teacher for today ──
	type SubstituteInfo struct {
		TeacherID           uint   `json:"teacher_id"`
		SubstituteTeacherID uint   `json:"substitute_teacher_id"`
		Session             string `json:"session"`
		SubstituteName      string `json:"substitute_name"`
		Status              string `json:"status"`
		Reason              string `json:"reason"`
		HasSubstitute       bool   `json:"has_substitute"`
	}
	var allSubInfos []SubstituteInfo
	if len(groupOrder) > 0 {
		var subLogs []models.HalaqohSubstituteLog
		h.db.Where("original_teacher_id IN ? AND date = ? AND is_active = ?",
			groupOrder, dateStr, true).
			Preload("SubstituteTeacher").
			Find(&subLogs)

		for _, log := range subLogs {
			info := SubstituteInfo{
				TeacherID:           log.OriginalTeacherID,
				SubstituteTeacherID: log.SubstituteTeacherID,
				HasSubstitute:       true,
			}
			if log.Session != nil {
				info.Session = *log.Session
			}
			if log.SubstituteTeacher.ID != 0 {
				info.SubstituteName = log.SubstituteTeacher.Name
			}
			if log.Status != nil {
				info.Status = *log.Status
			}
			if log.Reason != nil {
				info.Reason = *log.Reason
			}
			allSubInfos = append(allSubInfos, info)
		}
	}

	// ── Access control info for each teacher group ──
	type AccessInfo struct {
		TeacherID           uint `json:"teacher_id"`
		CanAccessAttendance bool `json:"can_access_attendance"`
		CanManage           bool `json:"can_manage"`
		IsSubstitute        bool `json:"is_substitute"`
		IsHelper            bool `json:"is_helper"`
	}
	var accessInfos []AccessInfo
	canManage := user.HasPermission("halaqoh-assignments.edit") ||
		user.HasPermission("halaqoh-assignments.delete") ||
		user.HasRole("admin") || user.HasRole("super_admin") || user.HasRole("tim_presensi")

	for _, tid := range groupOrder {
		isOwn := user.ID == tid
		isHelper := false
		isSub := false

		for _, a := range groupMap[tid].Assignments {
			if a.HelperTeacherID != nil && *a.HelperTeacherID == user.ID {
				isHelper = true
				break
			}
		}

		// Check if current user is substitute for this teacher (in-memory lookup)
		for _, s := range allSubInfos {
			if s.TeacherID == tid && s.SubstituteTeacherID == user.ID {
				isSub = true
				break
			}
		}

		accessInfos = append(accessInfos, AccessInfo{
			TeacherID:           tid,
			CanAccessAttendance: isOwn || isSub || canManage || isHelper,
			CanManage:           canManage,
			IsSubstitute:        isSub,
			IsHelper:            isHelper,
		})
	}

	return c.JSON(fiber.Map{
		"groups":             groups,
		"badges":             badgesList,
		"substitute_infos":   allSubInfos,
		"access_infos":       accessInfos,
		"session_times":      sessionTimeInfos,
		"can_filter_by_date": canViewAll,
		"selected_date":      dateStr,
		"current_user_id":    user.ID,
	})
}

// CreateAssignment assigns students to a teacher.
func (h *HalaqohHandler) CreateAssignment(c *fiber.Ctx) error {
	type Req struct {
		UserID          uint   `json:"user_id"`
		StudentIDs      []uint `json:"student_ids"`
		HelperTeacherID *uint  `json:"helper_teacher_id"`
	}
	var req Req
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}
	if req.UserID == 0 || len(req.StudentIDs) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Teacher and at least one student required"})
	}

	tx := h.db.Begin()
	for _, sid := range req.StudentIDs {
		var existing models.HalaqohAssignment
		err := tx.Unscoped().Where("student_id = ?", sid).First(&existing).Error
		if err == nil {
			tx.Model(&existing).Updates(map[string]interface{}{
				"user_id":           req.UserID,
				"helper_teacher_id": req.HelperTeacherID,
				"active":            true,
			})
		} else {
			tx.Create(&models.HalaqohAssignment{
				UserID:          req.UserID,
				StudentID:       sid,
				HelperTeacherID: req.HelperTeacherID,
				Active:          true,
			})
		}
	}
	tx.Commit()

	return c.JSON(fiber.Map{"message": "Penugasan Halaqoh berhasil dibuat"})
}

// UpdateAssignment updates a teacher's student list and helper.
func (h *HalaqohHandler) UpdateAssignment(c *fiber.Ctx) error {
	teacherID, _ := strconv.ParseUint(c.Params("teacher_id"), 10, 64)
	if teacherID == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid teacher ID"})
	}

	type Req struct {
		UserID          uint   `json:"user_id"`
		StudentIDs      []uint `json:"student_ids"`
		HelperTeacherID *uint  `json:"helper_teacher_id"`
	}
	var req Req
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	tx := h.db.Begin()

	tx.Model(&models.HalaqohAssignment{}).
		Where("user_id = ? AND active = ?", teacherID, true).
		Update("active", false)

	if req.UserID != uint(teacherID) {
		tx.Model(&models.HalaqohAssignment{}).
			Where("user_id = ? AND active = ?", req.UserID, true).
			Update("active", false)
	}

	for _, sid := range req.StudentIDs {
		var existing models.HalaqohAssignment
		err := tx.Where("student_id = ?", sid).First(&existing).Error
		if err == nil {
			tx.Model(&existing).Updates(map[string]interface{}{
				"user_id":           req.UserID,
				"helper_teacher_id": req.HelperTeacherID,
				"active":            true,
			})
		} else {
			tx.Create(&models.HalaqohAssignment{
				UserID:          req.UserID,
				StudentID:       sid,
				HelperTeacherID: req.HelperTeacherID,
				Active:          true,
			})
		}
	}

	if req.UserID != uint(teacherID) {
		todayStr := todayString()
		tx.Model(&models.HalaqohSubstituteLog{}).
			Where("original_teacher_id = ? AND date >= ? AND is_active = ?", teacherID, todayStr, true).
			Update("original_teacher_id", req.UserID)
	}

	tx.Commit()

	return c.JSON(fiber.Map{"message": "Penugasan Halaqoh berhasil diperbarui"})
}

// DeleteAssignment soft-deletes (sets active=false).
func (h *HalaqohHandler) DeleteAssignment(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	if id == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid assignment ID"})
	}

	result := h.db.Model(&models.HalaqohAssignment{}).
		Where("id = ?", id).
		Update("active", false)
	if result.RowsAffected == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "Assignment not found"})
	}

	return c.JSON(fiber.Map{"message": "Penugasan Halaqoh dinonaktifkan"})
}

// DeleteAssignmentsByTeacher deactivates all assignments for a teacher.
func (h *HalaqohHandler) DeleteAssignmentsByTeacher(c *fiber.Ctx) error {
	teacherID, _ := strconv.ParseUint(c.Params("teacher_id"), 10, 64)
	if teacherID == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid teacher ID"})
	}

	h.db.Model(&models.HalaqohAssignment{}).
		Where("user_id = ? AND active = ?", teacherID, true).
		Update("active", false)

	return c.JSON(fiber.Map{"message": "Semua penugasan guru dinonaktifkan"})
}

// ──────────────────────────────────────────────────────────────
// Substitute Management
// ──────────────────────────────────────────────────────────────

func (h *HalaqohHandler) AssignSubstitute(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	if id == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid assignment ID"})
	}

	type Req struct {
		SubstituteTeacherID uint     `json:"substitute_teacher_id"`
		SubstituteDate      string   `json:"substitute_date"`
		Sessions            []string `json:"sessions"`
		SubstituteStatus    string   `json:"substitute_status"`
		SubstituteReason    string   `json:"substitute_reason"`
	}
	var req Req
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	var assignment models.HalaqohAssignment
	if err := h.db.First(&assignment, id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Assignment not found"})
	}

	for _, session := range req.Sessions {
		var existing models.HalaqohSubstituteLog
		err := h.db.Where("halaqoh_assignment_id = ? AND date = ? AND session = ? AND is_active = ?",
			id, req.SubstituteDate, session, true).First(&existing).Error

		var status string
		if s := req.SubstituteStatus; len(s) > 0 {
			status = strings.ToUpper(s[:1]) + s[1:]
		}
		if err == nil {
			h.db.Model(&existing).Updates(map[string]interface{}{
				"substitute_teacher_id": req.SubstituteTeacherID,
				"status":                status,
				"reason":                req.SubstituteReason,
			})
		} else {
			h.db.Create(&models.HalaqohSubstituteLog{
				HalaqohAssignmentID: uint(id),
				OriginalTeacherID:   assignment.UserID,
				SubstituteTeacherID: req.SubstituteTeacherID,
				Date:                parseDate(req.SubstituteDate),
				Session:             &session,
				Status:              &status,
				Reason:              &req.SubstituteReason,
				IsActive:            true,
			})
		}
	}

	return c.JSON(fiber.Map{"message": fmt.Sprintf("Guru pengganti berhasil ditugaskan untuk sesi: %s", strings.Join(req.Sessions, ", "))})
}

func (h *HalaqohHandler) UnassignSubstitute(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 64)

	type Req struct {
		Date     string   `json:"date"`
		Sessions []string `json:"sessions"`
	}
	var req Req
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	h.db.Where("halaqoh_assignment_id = ? AND date = ? AND session IN ?",
		id, req.Date, req.Sessions).
		Delete(&models.HalaqohSubstituteLog{})

	return c.JSON(fiber.Map{"message": "Penugasan guru pengganti berhasil dihapus"})
}

func (h *HalaqohHandler) DeleteSubstituteHistory(c *fiber.Ctx) error {
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

	if err := h.db.Delete(&models.HalaqohSubstituteLog{}, uint(id)).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Gagal menghapus riwayat guru pengganti halaqoh"})
	}

	return c.JSON(fiber.Map{"message": "Riwayat guru pengganti halaqoh berhasil dihapus"})
}

// DeleteTeacherAttendanceRecord deletes a single halaqoh_teacher_attendance row. super_admin only.
func (h *HalaqohHandler) DeleteTeacherAttendanceRecord(c *fiber.Ctx) error {
	user, err := h.getUserFromContext(c)
	if err != nil {
		return err
	}
	if !user.HasRole("super_admin") {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Hanya super_admin yang dapat menghapus data absensi guru halaqoh"})
	}
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil || id == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "ID tidak valid"})
	}
	var record models.HalaqohTeacherAttendance
	if err := h.db.First(&record, uint(id)).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Data absensi halaqoh tidak ditemukan"})
	}
	if err := h.db.Delete(&record).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Gagal menghapus data absensi halaqoh"})
	}
	return c.JSON(fiber.Map{"message": "Data absensi guru halaqoh berhasil dihapus"})
}

// UpdateTeacherAttendanceRecord updates status/notes of a halaqoh_teacher_attendance. super_admin only.
func (h *HalaqohHandler) UpdateTeacherAttendanceRecord(c *fiber.Ctx) error {
	user, err := h.getUserFromContext(c)
	if err != nil {
		return err
	}
	if !user.HasRole("super_admin") {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Hanya super_admin yang dapat mengubah data absensi guru halaqoh"})
	}
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil || id == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "ID tidak valid"})
	}
	var record models.HalaqohTeacherAttendance
	if err := h.db.First(&record, uint(id)).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Data absensi halaqoh tidak ditemukan"})
	}
	var req struct {
		Status string `json:"status"`
		Notes  string `json:"notes"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Body permintaan tidak valid"})
	}
	validStatuses := map[string]bool{"Hadir": true, "Izin": true, "Sakit": true, "Alpha": true}
	if req.Status != "" && !validStatuses[req.Status] {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Status tidak valid"})
	}
	updates := map[string]interface{}{"notes": req.Notes}
	if req.Status != "" {
		updates["status"] = req.Status
	}
	if err := h.db.Model(&record).Updates(updates).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Gagal mengubah data absensi halaqoh"})
	}
	return c.JSON(fiber.Map{"message": "Data absensi guru halaqoh berhasil diperbarui"})
}

// ──────────────────────────────────────────────────────────────
// Student Attendance
// ──────────────────────────────────────────────────────────────

func (h *HalaqohHandler) GetAttendance(c *fiber.Ctx) error {
	assignmentID, _ := strconv.ParseUint(c.Params("assignment_id"), 10, 64)
	dateStr := c.Query("date")
	if dateStr == "" {
		dateStr = todayString()
	}

	user, err := h.getUserFromContext(c)
	if err != nil {
		return err
	}

	var assignment models.HalaqohAssignment
	if err := h.db.Preload("Teacher").Preload("Student").First(&assignment, assignmentID).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Assignment not found"})
	}
	if !h.canAccessAssignment(user, &assignment, dateStr) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Anda tidak memiliki akses ke absensi halaqoh ini"})
	}

	var allAssignments []models.HalaqohAssignment
	h.db.Where("user_id = ? AND active = ?", assignment.UserID, true).
		Preload("Student").
		Find(&allAssignments)

	var allAssignmentIDs []uint
	for _, a := range allAssignments {
		allAssignmentIDs = append(allAssignmentIDs, a.ID)
	}

	var existingAttendances []models.HalaqohAttendance
	if len(allAssignmentIDs) > 0 {
		h.db.Where("halaqoh_assignment_id IN ? AND date = ?", allAssignmentIDs, dateStr).
			Find(&existingAttendances)
	}

	attendanceMap := make(map[string]models.HalaqohAttendance)
	for _, att := range existingAttendances {
		key := fmt.Sprintf("%d_%s_%d", att.HalaqohAssignmentID, att.Session, *att.StudentID)
		attendanceMap[key] = att
	}

	sessions := []string{"Shubuh", "Ashar", "Isya"}
	substituteMap := make(map[string]*models.HalaqohSubstituteLog)
	if len(allAssignmentIDs) > 0 {
		var logs []models.HalaqohSubstituteLog
		h.db.Where("halaqoh_assignment_id IN ? AND date = ? AND is_active = ?",
			allAssignmentIDs, dateStr, true).
			Preload("SubstituteTeacher").
			Find(&logs)
		for i := range logs {
			if logs[i].Session != nil {
				substituteMap[*logs[i].Session] = &logs[i]
			}
		}
	}

	// Session time info
	canBypass := canBypassTimeRestriction(user)
	var sessionTimeInfos []fiber.Map
	for _, sess := range sessions {
		times := sessionTimes[sess]
		sessionTimeInfos = append(sessionTimeInfos, fiber.Map{
			"session":    sess,
			"start":      times[0],
			"end":        times[1],
			"is_active":  isWithinSessionTime(sess),
			"can_bypass": canBypass,
		})
	}

	return c.JSON(fiber.Map{
		"assignments":    allAssignments,
		"attendance_map": attendanceMap,
		"substitute_map": substituteMap,
		"teacher":        assignment.Teacher,
		"sessions":       sessions,
		"session_times":  sessionTimeInfos,
		"date":           dateStr,
	})
}

// SubmitSessionAttendance saves student attendance for one session.
func (h *HalaqohHandler) SubmitSessionAttendance(c *fiber.Ctx) error {
	session := c.Params("session")
	validSessions := map[string]bool{"Shubuh": true, "Ashar": true, "Isya": true}
	if !validSessions[session] {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid session"})
	}

	user, err := h.getUserFromContext(c)
	if err != nil {
		return err
	}

	// ── Time restriction check ──
	if !canBypassTimeRestriction(user) && !isWithinSessionTime(session) {
		times := sessionTimes[session]
		return c.Status(403).JSON(fiber.Map{
			"error": fmt.Sprintf("Sesi %s hanya dapat diakses pada jam %s - %s", session, times[0], times[1]),
		})
	}

	type AttendanceRecord struct {
		HalaqohAssignmentID uint   `json:"halaqoh_assignment_id"`
		StudentID           uint   `json:"student_id"`
		Status              string `json:"status"`
		Notes               string `json:"notes"`
	}
	type Req struct {
		Date    string             `json:"date"`
		Records []AttendanceRecord `json:"records"`
	}
	var req Req
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}
	if req.Date == "" {
		req.Date = todayString()
	}

	tx := h.db.Begin()
	for _, record := range req.Records {
		var assignment models.HalaqohAssignment
		if err := tx.First(&assignment, record.HalaqohAssignmentID).Error; err != nil {
			tx.Rollback()
			return c.Status(404).JSON(fiber.Map{"error": "Assignment not found"})
		}
		if !h.canAccessAssignment(user, &assignment, req.Date) {
			tx.Rollback()
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Anda tidak memiliki akses untuk mengisi absensi santri halaqoh"})
		}

		var existing models.HalaqohAttendance
		err := tx.Where("halaqoh_assignment_id = ? AND student_id = ? AND date = ? AND session = ?",
			record.HalaqohAssignmentID, record.StudentID, req.Date, session).
			First(&existing).Error

		if err == gorm.ErrRecordNotFound {
			notes := record.Notes
			var notesPtr *string
			if notes != "" {
				notesPtr = &notes
			}
			sid := record.StudentID
			tx.Create(&models.HalaqohAttendance{
				HalaqohAssignmentID: record.HalaqohAssignmentID,
				StudentID:           &sid,
				Date:                parseDate(req.Date),
				Session:             session,
				Status:              record.Status,
				Notes:               notesPtr,
				SubmittedBy:         &user.ID,
			})
		} else if err == nil {
			updates := map[string]interface{}{
				"status":       record.Status,
				"submitted_by": user.ID,
			}
			if record.Notes != "" {
				updates["notes"] = record.Notes
			} else {
				updates["notes"] = nil
			}
			tx.Model(&existing).Updates(updates)
		}
	}
	tx.Commit()

	return c.JSON(fiber.Map{"message": fmt.Sprintf("Absensi Santri Halaqoh untuk sesi %s berhasil disimpan!", session)})
}

// ──────────────────────────────────────────────────────────────
// Teacher Attendance
// ──────────────────────────────────────────────────────────────

func (h *HalaqohHandler) GetTeacherAttendance(c *fiber.Ctx) error {
	assignmentID, _ := strconv.ParseUint(c.Params("assignment_id"), 10, 64)
	dateStr := c.Query("date")
	if dateStr == "" {
		dateStr = todayString()
	}

	user, err := h.getUserFromContext(c)
	if err != nil {
		return err
	}

	var assignment models.HalaqohAssignment
	if err := h.db.Preload("Teacher").First(&assignment, assignmentID).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Assignment not found"})
	}
	if !h.canAccessTeacherAttendance(user, &assignment, dateStr) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Anda tidak memiliki akses ke absensi guru halaqoh ini"})
	}

	sessions := []string{"Shubuh", "Ashar", "Isya"}

	var subLogs []models.HalaqohSubstituteLog
	h.db.Where("halaqoh_assignment_id = ? AND date = ? AND is_active = ?",
		assignmentID, dateStr, true).
		Preload("SubstituteTeacher").
		Find(&subLogs)
	subLogMap := make(map[string]*models.HalaqohSubstituteLog)
	for i := range subLogs {
		if subLogs[i].Session != nil {
			subLogMap[*subLogs[i].Session] = &subLogs[i]
		}
	}

	var teacherAtts []models.HalaqohTeacherAttendance
	h.db.Where("(teacher_id = ? OR user_id = ?) AND date = ?",
		assignment.UserID, assignment.UserID, dateStr).
		Find(&teacherAtts)
	teacherAttMap := make(map[string]*models.HalaqohTeacherAttendance)
	for i := range teacherAtts {
		teacherAttMap[teacherAtts[i].Session] = &teacherAtts[i]
	}

	canBypass := canBypassTimeRestriction(user)

	type SessionInfo struct {
		Session          string      `json:"session"`
		EffectiveTeacher models.User `json:"effective_teacher"`
		IsSubstitute     bool        `json:"is_substitute"`
		SubstituteReason string      `json:"substitute_reason"`
		Status           string      `json:"status"`
		Notes            string      `json:"notes"`
		PhotoPath        string      `json:"photo_path"`
		HasAttendance    bool        `json:"has_attendance"`
		IsActive         bool        `json:"is_active"`
		CanBypass        bool        `json:"can_bypass"`
		TimeStart        string      `json:"time_start"`
		TimeEnd          string      `json:"time_end"`
	}

	var sessionInfos []SessionInfo
	for _, sess := range sessions {
		times := sessionTimes[sess]
		info := SessionInfo{
			Session:   sess,
			IsActive:  isWithinSessionTime(sess),
			CanBypass: canBypass,
			TimeStart: times[0],
			TimeEnd:   times[1],
		}

		if log, ok := subLogMap[sess]; ok {
			info.EffectiveTeacher = log.SubstituteTeacher
			info.IsSubstitute = true
			if log.Reason != nil {
				info.SubstituteReason = *log.Reason
			}
		} else {
			info.EffectiveTeacher = assignment.Teacher
		}

		if att, ok := teacherAttMap[sess]; ok {
			info.HasAttendance = true
			info.Status = att.Status
			if att.Notes != nil {
				info.Notes = *att.Notes
			}
			if att.PhotoPath != nil {
				info.PhotoPath = *att.PhotoPath
			}
		}

		sessionInfos = append(sessionInfos, info)
	}

	return c.JSON(fiber.Map{
		"assignment":    assignment,
		"session_infos": sessionInfos,
		"date":          dateStr,
	})
}

// SubmitTeacherAttendance saves teacher attendance for one session.
func (h *HalaqohHandler) SubmitTeacherAttendance(c *fiber.Ctx) error {
	assignmentID, _ := strconv.ParseUint(c.Params("assignment_id"), 10, 64)
	session := c.Params("session")

	validSessions := map[string]bool{"Shubuh": true, "Ashar": true, "Isya": true}
	if !validSessions[session] {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid session"})
	}

	user, err := h.getUserFromContext(c)
	if err != nil {
		return err
	}
	status := c.FormValue("status")
	notes := c.FormValue("notes")
	dateStr := c.FormValue("date")
	if dateStr == "" {
		dateStr = todayString()
	}

	var assignment models.HalaqohAssignment
	if err := h.db.First(&assignment, assignmentID).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Assignment not found"})
	}

	var log models.HalaqohSubstituteLog
	subErr := h.db.Where("halaqoh_assignment_id = ? AND date = ? AND session = ? AND is_active = ?",
		assignmentID, dateStr, session, true).First(&log).Error

	if !canManageHalaqohTeacherAttendance(user) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Absensi guru halaqoh hanya dapat diisi oleh tim presensi/admin"})
	}

	// ── Time restriction check ──
	if !canBypassTimeRestriction(user) && !isWithinSessionTime(session) {
		times := sessionTimes[session]
		return c.Status(403).JSON(fiber.Map{
			"error": fmt.Sprintf("Sesi %s hanya dapat diakses pada jam %s - %s", session, times[0], times[1]),
		})
	}

	teacherID := assignment.UserID
	if subErr == nil {
		teacherID = log.SubstituteTeacherID
	}

	var attendance models.HalaqohTeacherAttendance
	err = h.db.Where("user_id = ? AND date = ? AND session = ?", teacherID, dateStr, session).
		First(&attendance).Error

	if err == gorm.ErrRecordNotFound {
		attendance = models.HalaqohTeacherAttendance{
			UserID:      teacherID,
			TeacherID:   &assignment.UserID,
			Date:        parseDate(dateStr),
			Session:     session,
			Status:      status,
			SubmittedBy: &user.ID,
		}
		if notes != "" {
			attendance.Notes = &notes
		}
		h.db.Create(&attendance)
	} else {
		updates := map[string]interface{}{
			"status":       status,
			"submitted_by": user.ID,
			"teacher_id":   assignment.UserID,
		}
		if notes != "" {
			updates["notes"] = notes
		} else {
			updates["notes"] = nil
		}
		h.db.Model(&attendance).Updates(updates)
	}

	file, fileErr := c.FormFile("photo")
	if fileErr == nil && file != nil {
		path := fmt.Sprintf("./uploads/halaqoh_teacher/%d_%s_%s_%s", teacherID, dateStr, session, file.Filename)
		if err := c.SaveFile(file, path); err == nil {
			h.db.Model(&attendance).Update("photo_path", path)
		}
	}

	return c.JSON(fiber.Map{"message": fmt.Sprintf("Absensi Guru Halaqoh untuk sesi %s berhasil disimpan!", session)})
}

// ──────────────────────────────────────────────────────────────
// History & Statistics
// ──────────────────────────────────────────────────────────────

func (h *HalaqohHandler) StudentHistory(c *fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPage, _ := strconv.Atoi(c.Query("per_page", "15"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 15
	}

	query := h.db.Model(&models.HalaqohAttendance{}).
		Preload("HalaqohAssignment.Student").
		Preload("HalaqohAssignment.Teacher").
		Preload("Submitter")

	if sid := c.Query("student_id"); sid != "" {
		query = query.Where("student_id = ?", sid)
	}
	if start := c.Query("start_date"); start != "" {
		query = query.Where("date >= ?", start)
	}
	if end := c.Query("end_date"); end != "" {
		query = query.Where("date <= ?", end)
	}
	if sess := c.Query("session"); sess != "" {
		query = query.Where("session = ?", sess)
	}
	if st := c.Query("status"); st != "" {
		query = query.Where("status = ?", st)
	}

	var total int64
	query.Count(&total)

	var records []models.HalaqohAttendance
	query.Order("date DESC, session").
		Offset((page - 1) * perPage).
		Limit(perPage).
		Find(&records)

	totalPages := int(total) / perPage
	if int(total)%perPage != 0 {
		totalPages++
	}

	return c.JSON(models.PaginatedResponse{
		Data:       records,
		Total:      total,
		Page:       page,
		PerPage:    perPage,
		TotalPages: totalPages,
	})
}

func (h *HalaqohHandler) TeacherHistory(c *fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPage, _ := strconv.Atoi(c.Query("per_page", "15"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 15
	}

	query := h.db.Model(&models.HalaqohTeacherAttendance{}).
		Preload("Teacher")

	if tid := c.Query("teacher_id"); tid != "" {
		query = query.Where("user_id = ?", tid)
	}
	if start := c.Query("start_date"); start != "" {
		query = query.Where("date >= ?", start)
	}
	if end := c.Query("end_date"); end != "" {
		query = query.Where("date <= ?", end)
	}
	if sess := c.Query("session"); sess != "" {
		query = query.Where("session = ?", sess)
	}
	if st := c.Query("status"); st != "" {
		query = query.Where("status = ?", st)
	}

	var total int64
	query.Count(&total)

	var records []models.HalaqohTeacherAttendance
	query.Order("date DESC, session").
		Offset((page - 1) * perPage).
		Limit(perPage).
		Find(&records)

	totalPages := int(total) / perPage
	if int(total)%perPage != 0 {
		totalPages++
	}

	return c.JSON(models.PaginatedResponse{
		Data:       records,
		Total:      total,
		Page:       page,
		PerPage:    perPage,
		TotalPages: totalPages,
	})
}

func (h *HalaqohHandler) StudentStatistics(c *fiber.Ctx) error {
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	teacherID := c.Query("teacher_id")
	gender := c.Query("gender")

	query := h.db.Model(&models.HalaqohAttendance{})

	if startDate != "" && endDate != "" {
		query = query.Where("date BETWEEN ? AND ?", startDate, endDate)
	}
	if teacherID != "" {
		query = query.Joins("JOIN halaqoh_assignments ON halaqoh_assignments.id = halaqoh_attendances.halaqoh_assignment_id").
			Where("halaqoh_assignments.user_id = ?", teacherID)
	}
	if gender != "" {
		query = query.Joins("JOIN students ON students.id = halaqoh_attendances.student_id").
			Where("students.jenis_kelamin = ?", gender)
	}

	type StatusCount struct {
		Status string `json:"status"`
		Count  int64  `json:"count"`
	}
	var counts []StatusCount
	query.Select("status, COUNT(*) as count").
		Group("status").
		Scan(&counts)

	chartData := make(map[string]int64)
	for _, sc := range counts {
		chartData[sc.Status] = sc.Count
	}

	var teachers []models.User
	h.db.Joins("JOIN halaqoh_assignments ON halaqoh_assignments.user_id = users.id AND halaqoh_assignments.active = ?", true).
		Distinct().
		Select("users.id, users.name").
		Find(&teachers)

	return c.JSON(fiber.Map{
		"chart_data": chartData,
		"teachers":   teachers,
	})
}

func (h *HalaqohHandler) TeacherStatistics(c *fiber.Ctx) error {
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	query := h.db.Model(&models.HalaqohTeacherAttendance{})

	if startDate != "" && endDate != "" {
		query = query.Where("date BETWEEN ? AND ?", startDate, endDate)
	}

	type StatusCount struct {
		Status string `json:"status"`
		Count  int64  `json:"count"`
	}
	var counts []StatusCount
	query.Select("status, COUNT(*) as count").
		Group("status").
		Scan(&counts)

	chartData := make(map[string]int64)
	for _, sc := range counts {
		chartData[sc.Status] = sc.Count
	}

	return c.JSON(fiber.Map{
		"chart_data": chartData,
	})
}

// ── Helper ──

func jakartaLocation() *time.Location {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return time.Local
	}
	return loc
}

func todayString() string {
	return time.Now().In(jakartaLocation()).Format("2006-01-02")
}

func parseDate(s string) time.Time {
	loc := jakartaLocation()
	t, err := time.ParseInLocation("2006-01-02", s, loc)
	if err != nil {
		return time.Now().In(loc)
	}
	return t
}
