package handlers

import (
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func isHalaqohAdmin(user *models.User) bool {
	return user != nil && (user.HasRole("admin") || user.HasRole("super_admin"))
}

func halaqohTeacherQuery(db *gorm.DB) *gorm.DB {
	return db.Model(&models.User{}).Where(`EXISTS (SELECT 1 FROM role_user ru JOIN roles r ON r.id = ru.role_id WHERE ru.user_id = users.id AND LOWER(r.name) IN ('teacher','guru','musyrif')) OR EXISTS (SELECT 1 FROM halaqoh_assignments ha WHERE ha.user_id = users.id AND ha.active = ?)`, true)
}

func (h *HalaqohHandler) canRequestMembership(user *models.User) bool {
	if isHalaqohAdmin(user) {
		return true
	}
	if user == nil {
		return false
	}
	var count int64
	return halaqohTeacherQuery(h.db).Where("users.id = ?", user.ID).Count(&count).Error == nil && count > 0
}

// StudentOptions is a limited membership picker, never the master student API.
func (h *HalaqohHandler) StudentOptions(c *fiber.Ctx) error {
	user, err := h.getUserFromContext(c)
	if err != nil {
		return err
	}
	if !h.canRequestMembership(user) {
		return fiber.NewError(403, "Hanya guru pengampu yang dapat memilih santri halaqoh")
	}
	page, perPage := c.QueryInt("page", 1), c.QueryInt("per_page", 20)
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 20 {
		perPage = 20
	}
	query := h.db.Model(&models.Student{})
	search := strings.TrimSpace(c.Query("search"))
	if len([]rune(search)) >= 2 {
		escaped := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(search)
		query = query.Where("nama_lengkap LIKE ? ESCAPE '!'", "%"+escaped+"%")
	} else if !isHalaqohAdmin(user) {
		// Without a meaningful search, expose only the teacher's own roster.
		query = query.Where("EXISTS (SELECT 1 FROM halaqoh_assignments ha WHERE ha.student_id = students.id AND ha.user_id = ? AND ha.active = ?)", user.ID, true)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return fiber.NewError(500, "Gagal memuat pilihan santri")
	}
	var students []struct {
		ID                 uint    `json:"id"`
		NamaLengkap        string  `json:"nama_lengkap"`
		JenisKelamin       *string `json:"jenis_kelamin,omitempty"`
		CurrentTeacherID   uint    `json:"current_teacher_id,omitempty"`
		CurrentTeacherName string  `json:"current_teacher_name,omitempty"`
	}
	if err := query.Select("students.id, students.nama_lengkap, students.jenis_kelamin").Order("students.nama_lengkap").Limit(perPage).Offset((page - 1) * perPage).Scan(&students).Error; err != nil {
		return fiber.NewError(500, "Gagal memuat pilihan santri")
	}
	ids := make([]uint, 0, len(students))
	for _, student := range students {
		ids = append(ids, student.ID)
	}
	owners, err := halaqohStudentOwners(h.db, ids, false)
	if err != nil {
		return err
	}
	teacherIDs := []uint{}
	for _, id := range owners {
		if id != 0 {
			teacherIDs = append(teacherIDs, id)
		}
	}
	teacherNames := map[uint]string{}
	if len(teacherIDs) > 0 {
		var teachers []models.User
		if err := h.db.Select("id", "name").Where("id IN ?", teacherIDs).Find(&teachers).Error; err != nil {
			return err
		}
		for _, teacher := range teachers {
			teacherNames[teacher.ID] = teacher.Name
		}
	}
	for i := range students {
		students[i].CurrentTeacherID = owners[students[i].ID]
		students[i].CurrentTeacherName = teacherNames[owners[students[i].ID]]
	}
	return c.JSON(BuildPaginatedResponse(students, total, page, perPage))
}

func (h *HalaqohHandler) TeacherOptions(c *fiber.Ctx) error {
	user, err := h.getUserFromContext(c)
	if err != nil {
		return err
	}
	if !h.canRequestMembership(user) {
		return fiber.NewError(403, "Tidak berhak mengelola halaqoh")
	}
	var teachers []struct {
		ID   uint   `json:"id"`
		Name string `json:"name"`
	}
	if err := halaqohTeacherQuery(h.db).Select("users.id, users.name").Order("users.name").Scan(&teachers).Error; err != nil {
		return fiber.NewError(500, "Gagal memuat pengajar")
	}
	return c.JSON(fiber.Map{"data": teachers})
}

func uniqueStudentIDs(ids []uint) ([]uint, error) {
	result := make([]uint, 0, len(ids))
	seen := map[uint]bool{}
	if len(ids) > 1000 {
		return nil, fiber.NewError(400, "Terlalu banyak santri")
	}
	for _, id := range ids {
		if id == 0 {
			return nil, fiber.NewError(400, "ID santri tidak valid")
		}
		if !seen[id] {
			result = append(result, id)
			seen[id] = true
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result, nil
}

func sameStudentIDs(a, b []uint) bool {
	left, leftErr := uniqueStudentIDs(a)
	right, rightErr := uniqueStudentIDs(b)
	if leftErr != nil || rightErr != nil {
		return false
	}
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func validateHalaqohSelection(tx *gorm.DB, teacherID uint, ids []uint, helper *uint) error {
	var teacherCount int64
	if err := halaqohTeacherQuery(tx).Where("users.id = ?", teacherID).Count(&teacherCount).Error; err != nil {
		return err
	}
	if teacherCount != 1 {
		return fiber.NewError(400, "Guru pengampu tidak valid")
	}
	if helper != nil {
		if *helper == teacherID {
			return fiber.NewError(400, "Guru pembantu harus berbeda dari guru pengampu")
		}
		var count int64
		if err := halaqohTeacherQuery(tx).Where("users.id = ?", *helper).Count(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			return fiber.NewError(400, "Guru pembantu tidak valid")
		}
	}
	if len(ids) == 0 {
		return nil
	}
	var existing []models.Student
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").Where("id IN ?", ids).Find(&existing).Error; err != nil {
		return err
	}
	if len(existing) != len(ids) {
		return fiber.NewError(400, "Santri tidak ditemukan")
	}

	return nil
}

func activeHalaqohRoster(tx *gorm.DB, teacherID uint) ([]uint, *uint, error) {
	var rows []models.HalaqohAssignment
	if err := tx.Where("user_id = ? AND active = ?", teacherID, true).Order("id").Find(&rows).Error; err != nil {
		return nil, nil, err
	}
	ids := make([]uint, 0, len(rows))
	var helper *uint
	for _, row := range rows {
		ids = append(ids, row.StudentID)
		if helper == nil {
			helper = row.HelperTeacherID
		}
	}
	return ids, helper, nil
}

func lockHalaqohTeacher(tx *gorm.DB, teacherID uint) error {
	var user models.User
	return tx.Unscoped().Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, teacherID).Error
}

func halaqohStudentOwners(tx *gorm.DB, ids []uint, lock bool) (map[uint]uint, error) {
	owners := make(map[uint]uint, len(ids))
	for _, id := range ids {
		owners[id] = 0
	}
	if len(ids) == 0 {
		return owners, nil
	}
	var assignments []models.HalaqohAssignment
	query := tx.Where("student_id IN ? AND active = ?", ids, true).Order("id")
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := query.Find(&assignments).Error; err != nil {
		return nil, err
	}
	for _, assignment := range assignments {
		if previous := owners[assignment.StudentID]; previous != 0 && previous != assignment.UserID {
			return nil, fiber.NewError(409, "Santri memiliki beberapa halaqoh aktif. Admin perlu memperbaikinya terlebih dahulu.")
		}
		owners[assignment.StudentID] = assignment.UserID
	}
	return owners, nil
}

// Transfers coordinate both the destination and all source teachers. Re-read
// with row locks after taking teacher locks to detect a concurrent ownership move.
func lockHalaqohParticipants(tx *gorm.DB, teacherIDs, studentIDs []uint) error {
	owners, err := halaqohStudentOwners(tx, studentIDs, false)
	if err != nil {
		return err
	}
	locked := map[uint]bool{}
	for _, id := range teacherIDs {
		if id != 0 {
			locked[id] = true
		}
	}
	for _, owner := range owners {
		if owner != 0 {
			locked[owner] = true
		}
	}
	ordered := make([]uint, 0, len(locked))
	for id := range locked {
		ordered = append(ordered, id)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	for _, id := range ordered {
		if err := lockHalaqohTeacher(tx, id); err != nil {
			return err
		}
	}
	current, err := halaqohStudentOwners(tx, studentIDs, true)
	if err != nil {
		return err
	}
	for _, owner := range current {
		if owner != 0 && !locked[owner] {
			return fiber.NewError(409, "Halaqoh santri sedang berubah. Muat ulang permintaan.")
		}
	}
	return nil
}

func (h *HalaqohHandler) submitMembershipChange(c *fiber.Ctx, user *models.User, teacherID uint, kind string, ids []uint, helper *uint) error {
	if user.ID != teacherID || !h.canRequestMembership(user) {
		return fiber.NewError(403, "Guru hanya dapat mengajukan perubahan halaqoh milik sendiri")
	}
	ids, err := uniqueStudentIDs(ids)
	if err != nil {
		return err
	}
	var request models.HalaqohAssignmentChangeRequest
	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := lockHalaqohTeacher(tx, teacherID); err != nil {
			return err
		}
		var pending int64
		if err := tx.Model(&models.HalaqohAssignmentChangeRequest{}).Where("teacher_id = ? AND status = ?", teacherID, "pending").Count(&pending).Error; err != nil {
			return err
		}
		if pending > 0 {
			return fiber.NewError(409, "Masih ada permintaan menunggu persetujuan admin")
		}
		previous, previousHelper, err := activeHalaqohRoster(tx, teacherID)
		if err != nil {
			return err
		}
		switch kind {
		case "create":
			ids, err = uniqueStudentIDs(append(append([]uint{}, previous...), ids...))
			if err != nil {
				return err
			}
			if helper == nil {
				helper = previousHelper
			}
		case "delete_all":
			ids = previous
		case "delete_single":
			if len(ids) != 1 {
				return fiber.NewError(400, "Pilih satu santri")
			}
			found := false
			for _, id := range previous {
				if id == ids[0] {
					found = true
				}
			}
			if !found {
				return fiber.NewError(403, "Santri bukan anggota halaqoh Anda")
			}
		case "update":
		default:
			return fiber.NewError(400, "Jenis perubahan tidak valid")
		}
		if kind == "create" || kind == "update" {
			if err := validateHalaqohSelection(tx, teacherID, ids, helper); err != nil {
				return err
			}
		}
		owners, err := halaqohStudentOwners(tx, ids, false)
		if err != nil {
			return err
		}
		request = models.HalaqohAssignmentChangeRequest{TeacherID: teacherID, RequestedByUserID: user.ID, TargetTeacherID: teacherID, RequestType: kind, StudentIDs: ids, StudentOwnerSnapshot: owners, PreviousStudentIDs: previous, PreviousHelperTeacher: previousHelper, HelperTeacherID: helper, Status: "pending"}
		return tx.Create(&request).Error
	})
	if err != nil {
		return err
	}
	return c.Status(202).JSON(fiber.Map{"message": "Permintaan dikirim. Perubahan berlaku setelah disetujui admin.", "request_id": request.ID})
}

func applyHalaqohRoster(tx *gorm.DB, teacherID uint, ids []uint, helper *uint) error {
	var current []models.HalaqohAssignment
	if err := tx.Where("user_id = ? AND active = ?", teacherID, true).Find(&current).Error; err != nil {
		return err
	}
	retained := map[uint]models.HalaqohAssignment{}
	for _, assignment := range current {
		retained[assignment.StudentID] = assignment
	}
	if err := tx.Model(&models.HalaqohAssignment{}).Where("user_id = ? AND active = ?", teacherID, true).Update("active", false).Error; err != nil {
		return err
	}
	for _, id := range ids {
		if existing, ok := retained[id]; ok {
			if err := tx.Model(&existing).Updates(map[string]interface{}{"helper_teacher_id": helper, "active": true}).Error; err != nil {
				return err
			}
			continue
		}
		// Keep the former assignment and its attendance history attached to its
		// original teacher. A new membership period gets a new assignment ID.
		if err := tx.Model(&models.HalaqohAssignment{}).Where("student_id = ? AND active = ?", id, true).Update("active", false).Error; err != nil {
			return err
		}
		if err := tx.Create(&models.HalaqohAssignment{UserID: teacherID, StudentID: id, HelperTeacherID: helper, Active: true}).Error; err != nil {
			return err
		}
	}
	return nil
}

func (h *HalaqohHandler) applyAdminMembership(c *fiber.Ctx, source, target uint, ids []uint, helper *uint, add bool) error {
	if source == 0 || target == 0 {
		return fiber.NewError(400, "Pilih guru pengampu")
	}
	ids, err := uniqueStudentIDs(ids)
	if err != nil {
		return err
	}
	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := lockHalaqohParticipants(tx, []uint{source, target}, ids); err != nil {
			return err
		}

		if add {
			previous, previousHelper, err := activeHalaqohRoster(tx, target)
			if err != nil {
				return err
			}
			ids, err = uniqueStudentIDs(append(append([]uint{}, previous...), ids...))
			if err != nil {
				return err
			}
			if helper == nil {
				helper = previousHelper
			}
		}
		if err := validateHalaqohSelection(tx, target, ids, helper); err != nil {
			return err
		}
		if source != target {
			if err := tx.Model(&models.HalaqohAssignment{}).Where("user_id = ? AND active = ?", source, true).Update("active", false).Error; err != nil {
				return err
			}
			if err := tx.Model(&models.HalaqohSubstituteLog{}).Where("original_teacher_id = ? AND date >= ? AND is_active = ?", source, todayString(), true).Update("original_teacher_id", target).Error; err != nil {
				return err
			}
		}
		return applyHalaqohRoster(tx, target, ids, helper)
	})
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"message": "Penugasan Halaqoh berhasil disimpan"})
}

func reviewPendingHalaqoh(tx *gorm.DB, request *models.HalaqohAssignmentChangeRequest, userID uint, status, reason string) error {
	now := time.Now()
	result := tx.Model(request).Where("status = ?", "pending").Updates(map[string]interface{}{"status": status, "reason": reason, "reviewed_by": userID, "reviewed_at": now})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fiber.NewError(409, "Permintaan sudah ditinjau")
	}
	return nil
}

func (h *HalaqohHandler) reviewMembershipChange(c *fiber.Ctx, approve bool) error {
	user, err := h.getUserFromContext(c)
	if err != nil {
		return err
	}
	if !isHalaqohAdmin(user) {
		return fiber.NewError(403, "Hanya admin/super_admin yang dapat memverifikasi perubahan halaqoh")
	}
	var request models.HalaqohAssignmentChangeRequest
	var acknowledgement struct {
		ConfirmTransfers bool `json:"confirm_transfers"`
	}
	if approve && len(c.Body()) > 0 {
		if err := c.BodyParser(&acknowledgement); err != nil {
			return fiber.NewError(400, "Format persetujuan tidak valid")
		}
	}
	reason := ""
	if !approve {
		var payload struct {
			Reason string `json:"reason"`
		}
		if len(c.Body()) > 0 {
			if err := c.BodyParser(&payload); err != nil {
				return fiber.NewError(400, "Format alasan tidak valid")
			}
		}
		reason = strings.TrimSpace(payload.Reason)
		if reason == "" {
			reason = "Permintaan ditolak oleh admin"
		}
	}
	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&request, primaryKey(c.Params("id"))).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fiber.NewError(404, "Permintaan tidak ditemukan")
			}
			return err
		}
		if request.Status != "pending" {
			return fiber.NewError(409, "Permintaan sudah ditinjau")
		}
		if !approve {
			return reviewPendingHalaqoh(tx, &request, user.ID, "rejected", reason)
		}
		if request.TargetTeacherID != request.TeacherID || request.RequestedByUserID != request.TeacherID {
			return fiber.NewError(409, "Pemilik permintaan tidak valid")
		}
		if err := lockHalaqohParticipants(tx, []uint{request.TargetTeacherID}, append(append([]uint{}, request.StudentIDs...), request.PreviousStudentIDs...)); err != nil {
			return err
		}
		current, helper, err := activeHalaqohRoster(tx, request.TargetTeacherID)
		if err != nil {
			return err
		}
		if !sameStudentIDs(current, request.PreviousStudentIDs) {
			return fiber.NewError(409, "Daftar santri sudah berubah. Tolak permintaan ini dan minta guru mengajukan ulang.")
		}
		currentHelper, previousHelper := uint(0), uint(0)
		if helper != nil {
			currentHelper = *helper
		}
		if request.PreviousHelperTeacher != nil {
			previousHelper = *request.PreviousHelperTeacher
		}
		if currentHelper != previousHelper {
			return fiber.NewError(409, "Guru pembantu sudah berubah. Minta guru mengajukan ulang.")
		}
		ids, err := uniqueStudentIDs(request.StudentIDs)
		if err != nil {
			return err
		}
		if request.RequestType == "create" {
			ids, err = uniqueStudentIDs(append(append([]uint{}, request.PreviousStudentIDs...), ids...))
			if err != nil {
				return err
			}
		}
		if request.RequestType == "create" || request.RequestType == "update" {
			owners, err := halaqohStudentOwners(tx, ids, true)
			if err != nil {
				return err
			}
			transfers := false
			for _, id := range ids {
				owner := owners[id]
				if request.StudentOwnerSnapshot != nil {
					if owner != request.StudentOwnerSnapshot[id] {
						return fiber.NewError(409, "Halaqoh asal santri sudah berubah. Minta guru mengajukan ulang.")
					}
				} else if owner != 0 && owner != request.TargetTeacherID {
					return fiber.NewError(409, "Permintaan lama perlu diajukan ulang sebelum perpindahan santri")
				}
				if owner != 0 && owner != request.TargetTeacherID {
					transfers = true
				}
			}
			if transfers && !acknowledgement.ConfirmTransfers {
				return fiber.NewError(409, "Konfirmasi perpindahan santri dari halaqoh guru lain diperlukan")
			}
		}
		switch request.RequestType {
		case "create", "update":
			if err := validateHalaqohSelection(tx, request.TargetTeacherID, ids, request.HelperTeacherID); err != nil {
				return err
			}
		case "delete_single":
			if len(ids) != 1 {
				return fiber.NewError(409, "Permintaan penghapusan tidak valid")
			}
			found := false
			for _, id := range current {
				if id == ids[0] {
					found = true
				}
			}
			if !found {
				return fiber.NewError(409, "Santri tidak lagi berada di halaqoh ini")
			}
		case "delete_all":
		default:
			return fiber.NewError(409, "Jenis permintaan tidak valid")
		}
		if err := reviewPendingHalaqoh(tx, &request, user.ID, "approved", ""); err != nil {
			return err
		}
		switch request.RequestType {
		case "create", "update":
			return applyHalaqohRoster(tx, request.TargetTeacherID, ids, request.HelperTeacherID)
		case "delete_single":
			return tx.Model(&models.HalaqohAssignment{}).Where("user_id = ? AND student_id = ? AND active = ?", request.TargetTeacherID, ids[0], true).Update("active", false).Error
		default:
			return tx.Model(&models.HalaqohAssignment{}).Where("user_id = ? AND active = ?", request.TargetTeacherID, true).Update("active", false).Error
		}
	})
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"message": "Permintaan perubahan halaqoh berhasil ditinjau"})
}
