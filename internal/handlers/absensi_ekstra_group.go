package handlers

import (
	"fmt"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"gorm.io/gorm"
)

type AbsensiEkstraGroupHandler struct {
	db *gorm.DB
}

func NewAbsensiEkstraGroupHandler(db *gorm.DB) *AbsensiEkstraGroupHandler {
	return &AbsensiEkstraGroupHandler{db: db}
}

// List returns all groups with pagination
func (h *AbsensiEkstraGroupHandler) List(c *fiber.Ctx) error {
	userID := c.Locals("userID").(uint)

	page := c.QueryInt("page", 1)
	perPage := c.QueryInt("per_page", 10)
	search := c.Query("search")
	jenisKelamin := c.Query("jenis_kelamin")
	isActive := c.Query("is_active")

	query := h.db.Model(&models.AbsensiEkstraGroup{}).
		Preload("CreatedBy").
		Preload("Supervisors")

	// Filter by gender
	if jenisKelamin != "" && jenisKelamin != "all" {
		query = query.Where("jenis_kelamin = ?", jenisKelamin)
	}

	// Filter by active status
	if isActive != "" {
		query = query.Where("is_active = ?", isActive == "true" || isActive == "1")
	}

	// Search
	if search != "" {
		query = query.Where("nama_grup LIKE ?", "%"+search+"%")
	}

	// If user does not have view_all permission, only show groups where they're supervisor
	// We need to load user with roles/permissions to check
	var fullUser models.User
	h.db.Preload("Roles.Permissions").First(&fullUser, userID)
	hasViewAll := fullUser.HasPermission("absensi_ekstra.view_all")
	if !hasViewAll {
		query = query.Where("id IN (?)",
			h.db.Model(&models.AbsensiEkstraGroupSupervisor{}).
				Select("absensi_ekstra_group_id").
				Where("user_id = ?", userID),
		)
	}

	var total int64
	query.Count(&total)

	var groups []models.AbsensiEkstraGroup
	offset := (page - 1) * perPage
	query.Order("created_at desc").Limit(perPage).Offset(offset).Find(&groups)

	// Compute extra stats for each group
	type GroupResponse struct {
		models.AbsensiEkstraGroup
		StudentsCount int64 `json:"students_count"`
		SessionsCount int64 `json:"sessions_count"`
	}
	var response []GroupResponse
	for _, g := range groups {
		var sc int64
		h.db.Model(&models.AbsensiEkstraGroupStudent{}).
			Where("absensi_ekstra_group_id = ? AND tanggal_keluar IS NULL", g.ID).
			Count(&sc)
		var sesC int64
		h.db.Model(&models.AbsensiEkstraSession{}).
			Where("absensi_ekstra_group_id = ?", g.ID).
			Count(&sesC)
		response = append(response, GroupResponse{
			AbsensiEkstraGroup: g,
			StudentsCount:      sc,
			SessionsCount:      sesC,
		})
	}

	return c.JSON(fiber.Map{
		"data":        response,
		"total":       total,
		"page":        page,
		"per_page":    perPage,
		"total_pages": (total + int64(perPage) - 1) / int64(perPage),
	})
}

// Get returns a single group with details
func (h *AbsensiEkstraGroupHandler) Get(c *fiber.Ctx) error {
	id := c.Params("id")
	var group models.AbsensiEkstraGroup

	if err := h.db.
		Preload("CreatedBy").
		Preload("Supervisors").
		First(&group, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "Group not found",
		})
	}

	// Recent sessions
	var recentSessions []models.AbsensiEkstraSession
	h.db.Where("absensi_ekstra_group_id = ?", group.ID).
		Preload("CreatedBy").
		Order("tanggal desc").Limit(5).Find(&recentSessions)

	var openCount int64
	h.db.Model(&models.AbsensiEkstraSession{}).
		Where("absensi_ekstra_group_id = ? AND status = ?", group.ID, "open").
		Count(&openCount)

	// Top alpha students
	type AlphaStudent struct {
		StudentID  uint           `json:"student_id"`
		TotalAlpha int64          `json:"total_alpha"`
		Student    models.Student `json:"student" gorm:"foreignKey:StudentID"`
	}
	var topAlpha []AlphaStudent
	h.db.Model(&models.AbsensiEkstraSessionRecord{}).
		Select("student_id, count(*) as total_alpha").
		Joins("JOIN absensi_ekstra_sessions ON absensi_ekstra_sessions.id = absensi_ekstra_session_records.absensi_ekstra_session_id").
		Where("absensi_ekstra_sessions.absensi_ekstra_group_id = ? AND kehadiran = ?", group.ID, "alpha").
		Group("student_id").
		Order("total_alpha desc").
		Limit(5).
		Find(&topAlpha)
	// Preload student names
	for i, a := range topAlpha {
		h.db.First(&topAlpha[i].Student, a.StudentID)
	}

	var studentsCount int64
	h.db.Model(&models.AbsensiEkstraGroupStudent{}).
		Where("absensi_ekstra_group_id = ? AND tanggal_keluar IS NULL", group.ID).
		Count(&studentsCount)

	var sessionsCount int64
	h.db.Model(&models.AbsensiEkstraSession{}).
		Where("absensi_ekstra_group_id = ?", group.ID).
		Count(&sessionsCount)

	return c.JSON(fiber.Map{
		"group":           group,
		"recent_sessions": recentSessions,
		"open_sessions":   openCount,
		"top_alpha":       topAlpha,
		"students_count":  studentsCount,
		"sessions_count":  sessionsCount,
	})
}

// Create creates a new group
func (h *AbsensiEkstraGroupHandler) Create(c *fiber.Ctx) error {
	userID := c.Locals("userID").(uint)

	type CreateGroupInput struct {
		NamaGrup     string `json:"nama_grup" validate:"required"`
		Deskripsi    string `json:"deskripsi"`
		JenisKelamin string `json:"jenis_kelamin" validate:"required"`
		IsActive     *bool  `json:"is_active"`
	}

	var input CreateGroupInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "bad_request",
			Message: "Invalid input",
		})
	}

	if input.NamaGrup == "" || input.JenisKelamin == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "validation_error",
			Message: "nama_grup and jenis_kelamin are required",
		})
	}

	isActive := true
	if input.IsActive != nil {
		isActive = *input.IsActive
	}

	group := models.AbsensiEkstraGroup{
		NamaGrup:     input.NamaGrup,
		Deskripsi:    input.Deskripsi,
		JenisKelamin: input.JenisKelamin,
		IsActive:     isActive,
		CreatedByID:  userID,
	}

	if err := h.db.Create(&group).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to create group: " + err.Error(),
		})
	}

	// Add creator as admin supervisor
	h.db.Create(&models.AbsensiEkstraGroupSupervisor{
		AbsensiEkstraGroupID: group.ID,
		UserID:               userID,
		Role:                 "admin",
	})

	h.db.Preload("CreatedBy").Preload("Supervisors").First(&group, group.ID)

	return c.Status(fiber.StatusCreated).JSON(group)
}

// Update updates group details
func (h *AbsensiEkstraGroupHandler) Update(c *fiber.Ctx) error {
	id := c.Params("id")
	var group models.AbsensiEkstraGroup

	if err := h.db.First(&group, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "Group not found",
		})
	}

	type UpdateGroupInput struct {
		NamaGrup     string `json:"nama_grup"`
		Deskripsi    string `json:"deskripsi"`
		JenisKelamin string `json:"jenis_kelamin"`
		IsActive     *bool  `json:"is_active"`
	}

	var input UpdateGroupInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "bad_request",
			Message: "Invalid input",
		})
	}

	if input.NamaGrup != "" {
		group.NamaGrup = input.NamaGrup
	}
	if input.Deskripsi != "" {
		group.Deskripsi = input.Deskripsi
	}
	if input.JenisKelamin != "" {
		group.JenisKelamin = input.JenisKelamin
	}
	if input.IsActive != nil {
		group.IsActive = *input.IsActive
	}

	h.db.Save(&group)

	return c.JSON(group)
}

// Delete deletes a group
func (h *AbsensiEkstraGroupHandler) Delete(c *fiber.Ctx) error {
	id := c.Params("id")
	var group models.AbsensiEkstraGroup

	if err := h.db.First(&group, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "Group not found",
		})
	}

	// Delete related data
	h.db.Where("absensi_ekstra_group_id = ?", group.ID).Delete(&models.AbsensiEkstraGroupStudent{})
	h.db.Where("absensi_ekstra_group_id = ?", group.ID).Delete(&models.AbsensiEkstraGroupSupervisor{})
	// Delete sessions and records
	var sessions []models.AbsensiEkstraSession
	h.db.Where("absensi_ekstra_group_id = ?", group.ID).Find(&sessions)
	for _, s := range sessions {
		h.db.Where("absensi_ekstra_session_id = ?", s.ID).Delete(&models.AbsensiEkstraSessionRecord{})
	}
	h.db.Where("absensi_ekstra_group_id = ?", group.ID).Delete(&models.AbsensiEkstraSession{})

	h.db.Delete(&group)

	return c.JSON(fiber.Map{"message": "Group deleted successfully"})
}

// GetStudents returns students in a group (with available students list)
func (h *AbsensiEkstraGroupHandler) GetStudents(c *fiber.Ctx) error {
	id := c.Params("id")
	var group models.AbsensiEkstraGroup

	if err := h.db.First(&group, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "Group not found",
		})
	}

	// Get current members via pivot table
	type MemberInfo struct {
		StudentID     uint       `json:"student_id"`
		NamaLengkap   string     `json:"nama_lengkap"`
		JenisKelamin  string     `json:"jenis_kelamin"`
		TanggalMasuk  time.Time  `json:"tanggal_masuk"`
		TanggalKeluar *time.Time `json:"tanggal_keluar"`
	}
	var members []MemberInfo
	h.db.Raw(`
		SELECT s.id as student_id, s.nama_lengkap, s.jenis_kelamin, 
				gs.tanggal_masuk, gs.tanggal_keluar
		FROM absensi_ekstra_group_students gs
		JOIN students s ON s.id = gs.student_id
		WHERE gs.absensi_ekstra_group_id = ? AND gs.tanggal_keluar IS NULL
		ORDER BY s.nama_lengkap
	`, group.ID).Scan(&members)

	// Get available students (not in this group)
	var existingIDs []uint
	h.db.Model(&models.AbsensiEkstraGroupStudent{}).
		Where("absensi_ekstra_group_id = ? AND tanggal_keluar IS NULL", group.ID).
		Pluck("student_id", &existingIDs)

	var availableStudents []models.Student
	availQuery := h.db.Model(&models.Student{})

	// Filter by gender
	if group.JenisKelamin == "putra" {
		availQuery = availQuery.Where("jenis_kelamin IN ?", []string{"Laki-Laki", "Laki - Laki", "putra"})
	} else if group.JenisKelamin == "putri" {
		availQuery = availQuery.Where("jenis_kelamin IN ?", []string{"Perempuan", "putri"})
	}

	if len(existingIDs) > 0 {
		availQuery = availQuery.Where("id NOT IN ?", existingIDs)
	}
	availQuery.Order("nama_lengkap").Find(&availableStudents)

	return c.JSON(fiber.Map{
		"members":            members,
		"available_students": availableStudents,
	})
}

// AddStudents add students to a group
func (h *AbsensiEkstraGroupHandler) AddStudents(c *fiber.Ctx) error {
	id := c.Params("id")
	var group models.AbsensiEkstraGroup

	if err := h.db.First(&group, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "Group not found",
		})
	}

	type AddStudentsInput struct {
		StudentIDs   []uint `json:"student_ids"`
		TanggalMasuk string `json:"tanggal_masuk"`
	}

	var input AddStudentsInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "bad_request",
			Message: "Invalid input",
		})
	}

	tanggalMasuk := time.Now()
	if input.TanggalMasuk != "" {
		if t, err := time.Parse("2006-01-02", input.TanggalMasuk); err == nil {
			tanggalMasuk = t
		}
	}

	added := 0
	for _, sid := range input.StudentIDs {
		var existing models.AbsensiEkstraGroupStudent
		result := h.db.Where("absensi_ekstra_group_id = ? AND student_id = ? AND tanggal_keluar IS NULL",
			group.ID, sid).First(&existing)
		if result.RowsAffected == 0 {
			h.db.Create(&models.AbsensiEkstraGroupStudent{
				AbsensiEkstraGroupID: group.ID,
				StudentID:            sid,
				TanggalMasuk:         tanggalMasuk,
			})
			added++
		}
	}

	return c.JSON(fiber.Map{
		"message": fmt.Sprintf("%d santri berhasil ditambahkan", added),
	})
}

// RemoveStudent removes a student from a group (soft remove via tanggal_keluar)
func (h *AbsensiEkstraGroupHandler) RemoveStudent(c *fiber.Ctx) error {
	groupID := c.Params("id")
	studentID := c.Params("student_id")

	now := time.Now()
	result := h.db.Model(&models.AbsensiEkstraGroupStudent{}).
		Where("absensi_ekstra_group_id = ? AND student_id = ? AND tanggal_keluar IS NULL", groupID, studentID).
		Update("tanggal_keluar", now)

	if result.RowsAffected == 0 {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "Student not found in this group",
		})
	}

	return c.JSON(fiber.Map{"message": "Santri berhasil dikeluarkan dari grup"})
}

// GetSupervisors returns supervisors for a group
func (h *AbsensiEkstraGroupHandler) GetSupervisors(c *fiber.Ctx) error {
	id := c.Params("id")
	var group models.AbsensiEkstraGroup

	if err := h.db.First(&group, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "Group not found",
		})
	}

	type SupervisorInfo struct {
		UserID uint   `json:"user_id"`
		Name   string `json:"name"`
		Role   string `json:"role"`
	}
	var supervisors []SupervisorInfo
	h.db.Raw(`
		SELECT u.id as user_id, u.name, gs.role
		FROM absensi_ekstra_group_supervisors gs
		JOIN users u ON u.id = gs.user_id
		WHERE gs.absensi_ekstra_group_id = ?
		ORDER BY gs.role, u.name
	`, group.ID).Scan(&supervisors)

	// Get available users
	var existingIDs []uint
	h.db.Model(&models.AbsensiEkstraGroupSupervisor{}).
		Where("absensi_ekstra_group_id = ?", group.ID).
		Pluck("user_id", &existingIDs)

	var availableUsers []models.User
	userQuery := h.db.Model(&models.User{})
	if len(existingIDs) > 0 {
		userQuery = userQuery.Where("id NOT IN ?", existingIDs)
	}
	userQuery.Order("name").Find(&availableUsers)

	return c.JSON(fiber.Map{
		"supervisors":     supervisors,
		"available_users": availableUsers,
	})
}

// AddSupervisor adds a supervisor to a group
func (h *AbsensiEkstraGroupHandler) AddSupervisor(c *fiber.Ctx) error {
	id := c.Params("id")
	var group models.AbsensiEkstraGroup

	if err := h.db.First(&group, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "Group not found",
		})
	}

	type AddSupervisorInput struct {
		UserID uint   `json:"user_id"`
		Role   string `json:"role"`
	}

	var input AddSupervisorInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "bad_request",
			Message: "Invalid input",
		})
	}

	if input.Role != "admin" && input.Role != "pengawas" {
		input.Role = "pengawas"
	}

	// Check not already
	var existing models.AbsensiEkstraGroupSupervisor
	result := h.db.Where("absensi_ekstra_group_id = ? AND user_id = ?", group.ID, input.UserID).First(&existing)
	if result.RowsAffected > 0 {
		return c.Status(fiber.StatusConflict).JSON(models.ErrorResponse{
			Error:   "conflict",
			Message: "User is already a supervisor in this group",
		})
	}

	h.db.Create(&models.AbsensiEkstraGroupSupervisor{
		AbsensiEkstraGroupID: group.ID,
		UserID:               input.UserID,
		Role:                 input.Role,
	})

	return c.JSON(fiber.Map{"message": "Supervisor berhasil ditambahkan"})
}

// RemoveSupervisor removes a supervisor from a group
func (h *AbsensiEkstraGroupHandler) RemoveSupervisor(c *fiber.Ctx) error {
	groupID := c.Params("id")
	userIDParam := c.Params("user_id")
	userIDInt, _ := strconv.Atoi(userIDParam)

	// Don't allow removing the last admin
	var adminCount int64
	h.db.Model(&models.AbsensiEkstraGroupSupervisor{}).
		Where("absensi_ekstra_group_id = ? AND role = ?", groupID, "admin").
		Count(&adminCount)

	var isAdmin int64
	h.db.Model(&models.AbsensiEkstraGroupSupervisor{}).
		Where("absensi_ekstra_group_id = ? AND user_id = ? AND role = ?", groupID, userIDInt, "admin").
		Count(&isAdmin)

	if isAdmin > 0 && adminCount <= 1 {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "validation_error",
			Message: "Tidak bisa menghapus admin terakhir dari grup",
		})
	}

	result := h.db.Where("absensi_ekstra_group_id = ? AND user_id = ?", groupID, userIDInt).
		Delete(&models.AbsensiEkstraGroupSupervisor{})

	if result.RowsAffected == 0 {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "Supervisor not found in this group",
		})
	}

	return c.JSON(fiber.Map{"message": "Supervisor berhasil dihapus"})
}
