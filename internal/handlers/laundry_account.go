package handlers

import (
	"fmt"
	"math"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"gorm.io/gorm"
)

type LaundryAccountHandler struct {
	db *gorm.DB
}

func NewLaundryAccountHandler(db *gorm.DB) *LaundryAccountHandler {
	return &LaundryAccountHandler{db: db}
}

const (
	WEEKLY_LIMIT_KG   = 7.5
	MONTHLY_LIMIT_KG  = 30.0
	PRICE_PER_KG      = 4500.0
	DEBT_PRICE_PER_KG = 6000.0
)

// List returns all accounts with custom calculated limits and debts
func (h *LaundryAccountHandler) List(c *fiber.Ctx) error {
	var accounts []models.LaundryAccount
	var total int64

	page := c.QueryInt("page", 1)
	perPage := c.QueryInt("per_page", 20)
	search := c.Query("search")
	genderType := c.Query("gender_type")
	dateFromStr := c.Query("date_from")
	dateToStr := c.Query("date_to")

	// Default to current month if no dates
	now := time.Now()
	var dateFrom, dateTo time.Time
	if dateFromStr == "" || dateToStr == "" {
		dateFrom = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		dateTo = dateFrom.AddDate(0, 1, -1).Add(time.Hour*23 + time.Minute*59 + time.Second*59)
	} else {
		dateFrom, _ = time.Parse("2006-01-02", dateFromStr)
		dateTo, _ = time.Parse("2006-01-02", dateToStr)
		dateTo = time.Date(dateTo.Year(), dateTo.Month(), dateTo.Day(), 23, 59, 59, 0, dateTo.Location())
	}

	query := h.db.Model(&models.LaundryAccount{}).Preload("Vendor").Preload("Student").Preload("User").Where("active = ?", true)

	if search != "" {
		query = query.Where(
			"nomor_laundry LIKE ? OR student_id IN (SELECT id FROM students WHERE nama_lengkap LIKE ?) OR user_id IN (SELECT id FROM users WHERE name LIKE ?)",
			"%"+search+"%", "%"+search+"%", "%"+search+"%",
		)
	}

	if genderType != "" && genderType != "all" {
		query = query.Where("vendor_id IN (SELECT id FROM laundry_vendors WHERE gender_type = ?)", genderType)
	}

	query.Count(&total)

	offset := (page - 1) * perPage
	query.Limit(perPage).Offset(offset).Find(&accounts)

	type AccountWithMetrics struct {
		models.LaundryAccount
		PeriodWeight    float64 `json:"period_weight"`
		WeeklyWeight    float64 `json:"weekly_weight"`
		MonthlyWeight   float64 `json:"monthly_weight"`
		MonthlyExceeded bool    `json:"monthly_exceeded"`
		WeeklyExceeded  bool    `json:"weekly_exceeded"`
		ExcessKg        float64 `json:"excess_kg"`
		DebtAmount      float64 `json:"debt_amount"`
		OwnerName       string  `json:"owner_name"`
		OwnerType       string  `json:"owner_type"`
	}

	var results []AccountWithMetrics

	weekStart := getStartOfWeek(now)
	weekEnd := getEndOfWeek(now)

	for _, acc := range accounts {
		var periodWeight float64
		h.db.Model(&models.LaundryTransaction{}).
			Where("laundry_account_id = ? AND tanggal BETWEEN ? AND ?", acc.ID, dateFrom, dateTo).
			Select("COALESCE(SUM(berat_kg), 0)").Row().Scan(&periodWeight)

		var weeklyWeight float64
		h.db.Model(&models.LaundryTransaction{}).
			Where("laundry_account_id = ? AND tanggal BETWEEN ? AND ?", acc.ID, weekStart, weekEnd).
			Select("COALESCE(SUM(berat_kg), 0)").Row().Scan(&weeklyWeight)

		// Replace month-specific fetch with the periodWeight fetch so that custom filtered ranges apply the 30kg limits and debts properly on the UI.
		monthlyExcess := math.Max(0, periodWeight-MONTHLY_LIMIT_KG)
		debtAmount := monthlyExcess * DEBT_PRICE_PER_KG

		ownerName := "Unknown"
		ownerType := "Unknown"
		if acc.Student != nil {
			ownerName = acc.Student.NamaLengkap
			ownerType = "Siswa"
		} else if acc.User != nil {
			ownerName = acc.User.Name
			ownerType = "User"
		}

		results = append(results, AccountWithMetrics{
			LaundryAccount:  acc,
			PeriodWeight:    periodWeight,
			WeeklyWeight:    weeklyWeight,
			MonthlyWeight:   periodWeight, // Frontend looks at this field for table sorting/display
			MonthlyExceeded: periodWeight > MONTHLY_LIMIT_KG,
			WeeklyExceeded:  weeklyWeight > WEEKLY_LIMIT_KG,
			ExcessKg:        monthlyExcess,
			DebtAmount:      debtAmount,
			OwnerName:       ownerName,
			OwnerType:       ownerType,
		})
	}

	// Sort results by period weight descending
	// Note: in memory sort for pagination is not ideal for large datasets but good enough for now
	// To do it in DB requires a complex subquery.

	return c.JSON(fiber.Map{
		"data":        results,
		"total":       total,
		"page":        page,
		"per_page":    perPage,
		"total_pages": (total + int64(perPage) - 1) / int64(perPage),
	})
}

// Generate unique permanent laundry number
func (h *LaundryAccountHandler) generateLaundryNumber() string {
	datePart := time.Now().Format("20060102")
	var count int64
	todayStart := time.Now().Truncate(24 * time.Hour)
	h.db.Unscoped().Model(&models.LaundryAccount{}).Where("created_at >= ?", todayStart).Count(&count)
	return fmt.Sprintf("%s%03d", datePart, count+1)
}

func (h *LaundryAccountHandler) Create(c *fiber.Ctx) error {
	type CreateRequest struct {
		VendorID  uint  `json:"vendor_id"`
		StudentID *uint `json:"student_id"`
		UserID    *uint `json:"user_id"`
	}

	var req CreateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Message: "Invalid input"})
	}

	if req.StudentID == nil && req.UserID == nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Message: "Select a student or user"})
	}

	account := models.LaundryAccount{
		VendorID:     req.VendorID,
		StudentID:    req.StudentID,
		UserID:       req.UserID,
		NomorLaundry: h.generateLaundryNumber(),
		Active:       true,
		Blocked:      false,
	}

	if err := h.db.Create(&account).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Message: "Failed to create account"})
	}

	h.db.Preload("Student").Preload("User").First(&account, account.ID)
	name := "Unknown"
	if account.Student != nil {
		name = account.Student.NamaLengkap
	} else if account.User != nil {
		name = account.User.Name
	}
	logUserAction(h.db, c, "Buat Akun Laundry", "Membuat akun laundry baru dengan nomor "+account.NomorLaundry+" untuk "+name)

	return c.Status(fiber.StatusCreated).JSON(account)
}

func (h *LaundryAccountHandler) Update(c *fiber.Ctx) error {
	id := c.Params("id")
	var account models.LaundryAccount

	if err := h.db.First(&account, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Message: "Account not found"})
	}

	type UpdateRequest struct {
		VendorID  uint  `json:"vendor_id"`
		StudentID *uint `json:"student_id"`
		UserID    *uint `json:"user_id"`
		Active    *bool `json:"active"`
	}

	var req UpdateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Message: "Invalid input"})
	}

	if req.VendorID != 0 {
		account.VendorID = req.VendorID
	}
	account.StudentID = req.StudentID
	account.UserID = req.UserID
	if req.Active != nil {
		account.Active = *req.Active
	}

	if err := h.db.Save(&account).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Message: "Failed to update account"})
	}

	h.db.Preload("Student").Preload("User").First(&account, account.ID)
	name := "Unknown"
	if account.Student != nil {
		name = account.Student.NamaLengkap
	} else if account.User != nil {
		name = account.User.Name
	}
	logUserAction(h.db, c, "Ubah Akun Laundry", "Memperbarui akun laundry "+account.NomorLaundry+" untuk "+name)

	return c.JSON(account)
}

func (h *LaundryAccountHandler) ToggleBlock(c *fiber.Ctx) error {
	id := c.Params("id")
	var account models.LaundryAccount

	if err := h.db.Preload("Student").Preload("User").First(&account, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Message: "Account not found"})
	}

	account.Blocked = !account.Blocked
	if err := h.db.Save(&account).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Message: "Failed to update block status"})
	}

	ownerName := "Unknown"
	if account.Student != nil {
		ownerName = account.Student.NamaLengkap
	} else if account.User != nil {
		ownerName = account.User.Name
	}

	action := "Blokir Akun Laundry"
	desc := "Telah diblokir."
	if !account.Blocked {
		action = "Buka Blokir Akun Laundry"
		desc = "Telah dibuka blokirnya."
	}
	logUserAction(h.db, c, action, "Akun laundry "+account.NomorLaundry+" milik "+ownerName+" "+desc)

	return c.JSON(fiber.Map{
		"message": "Account target updated",
		"blocked": account.Blocked,
	})
}

// BulkCreate based on selected students/users
func (h *LaundryAccountHandler) BulkCreate(c *fiber.Ctx) error {
	type BulkCreateRequest struct {
		VendorID       uint     `json:"vendor_id"`
		SelectedPeople []string `json:"selected_people"` // Format: "student_1", "user_2"
	}

	var req BulkCreateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Message: "Invalid input"})
	}

	if len(req.SelectedPeople) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Message: "No people selected"})
	}

	createdCount := 0
	skippedCount := 0

	tx := h.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	for _, personData := range req.SelectedPeople {
		// Parse "type_id"
		var pType string
		var pId uint
		if _, err := fmt.Sscanf(personData, "%s_%d", &pType, &pId); err != nil {
			// fallback check
			var tmp int
			fmt.Sscanf(personData, "%[a-z]_%d", &pType, &tmp)
			pId = uint(tmp)
		}

		var studentID, userID *uint
		if pType == "student" {
			studentID = &pId
		} else if pType == "user" {
			userID = &pId
		} else {
			continue // skip invalid formats
		}

		// Check if exists
		var existing models.LaundryAccount
		query := tx.Where("active = ?", true)
		if studentID != nil {
			query = query.Where("student_id = ?", *studentID)
		} else {
			query = query.Where("user_id = ?", *userID)
		}

		if err := query.First(&existing).Error; err == nil {
			skippedCount++
			continue
		}

		account := models.LaundryAccount{
			VendorID:     req.VendorID,
			StudentID:    studentID,
			UserID:       userID,
			NomorLaundry: h.generateLaundryNumber(),
			Active:       true,
		}

		if err := tx.Create(&account).Error; err != nil {
			tx.Rollback()
			return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Message: "Failed to create accounts"})
		}
		createdCount++
	}

	var vendor models.LaundryVendor
	tx.First(&vendor, req.VendorID)
	logUserAction(tx, c, "Bulk Create Akun Laundry", fmt.Sprintf("Membuat %d akun laundry baru untuk vendor %s. %d dilewati.", createdCount, vendor.Name, skippedCount))

	tx.Commit()

	return c.JSON(fiber.Map{
		"message":       fmt.Sprintf("%d accounts created, %d skipped", createdCount, skippedCount),
		"created_count": createdCount,
		"skipped_count": skippedCount,
	})
}

// Delete permanently removes an account and its transactions
func (h *LaundryAccountHandler) Delete(c *fiber.Ctx) error {
	id := c.Params("id")
	var account models.LaundryAccount
	if err := h.db.Preload("Student").Preload("User").First(&account, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Message: "Account not found"})
	}

	tx := h.db.Begin()

	var txCount int64
	tx.Model(&models.LaundryTransaction{}).Where("laundry_account_id = ?", account.ID).Count(&txCount)

	// Delete transactions
	tx.Where("laundry_account_id = ?", account.ID).Delete(&models.LaundryTransaction{})

	// Delete account
	tx.Delete(&account)

	ownerName := "Unknown"
	if account.Student != nil {
		ownerName = account.Student.NamaLengkap
	} else if account.User != nil {
		ownerName = account.User.Name
	}

	logUserAction(tx, c, "Hapus Akun Laundry", fmt.Sprintf("Akun %s milik %s dihapus (soft-delete) dengan %d transaksi.", account.NomorLaundry, ownerName, txCount))

	tx.Commit()
	return c.JSON(fiber.Map{"message": "Account deleted successfully"})
}

// ListTrashed returns globally soft-deleted accounts
func (h *LaundryAccountHandler) ListTrashed(c *fiber.Ctx) error {
	var accounts []models.LaundryAccount
	query := h.db.Unscoped().Where("deleted_at IS NOT NULL")

	// Preload basic relations
	query.Preload("Vendor").Preload("Student").Preload("User").
		Order("deleted_at desc").Find(&accounts)

	// Build response with owner_name formatting
	var response []fiber.Map
	for _, acc := range accounts {
		ownerName := "Unknown"
		ownerType := "Unknown"

		if acc.Student != nil {
			ownerName = acc.Student.NamaLengkap
			ownerType = "Siswa"
		} else if acc.User != nil {
			ownerName = acc.User.Name
			ownerType = "User/Guru"
		}

		response = append(response, fiber.Map{
			"id":            acc.ID,
			"nomor_laundry": acc.NomorLaundry,
			"owner_name":    ownerName,
			"owner_type":    ownerType,
			"vendor_name":   acc.Vendor.Name,
			"deleted_at":    acc.DeletedAt.Time,
		})
	}

	if response == nil {
		response = make([]fiber.Map, 0)
	}

	return c.JSON(response)
}

// Restore recovers a soft-deleted account and its transactions
func (h *LaundryAccountHandler) Restore(c *fiber.Ctx) error {
	id := c.Params("id")
	var account models.LaundryAccount

	if err := h.db.Unscoped().Preload("Student").Preload("User").First(&account, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Message: "Account not found in trash"})
	}

	if !account.DeletedAt.Valid {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Message: "Account is not deleted"})
	}

	tx := h.db.Begin()

	// Restore transactions
	if err := tx.Unscoped().Model(&models.LaundryTransaction{}).
		Where("laundry_account_id = ?", account.ID).
		Update("deleted_at", nil).Error; err != nil {
		tx.Rollback()
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Message: "Failed to restore transactions"})
	}

	// Restore account
	if err := tx.Unscoped().Model(&account).Update("deleted_at", nil).Error; err != nil {
		tx.Rollback()
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Message: "Failed to restore account"})
	}

	ownerName := "Unknown"
	if account.Student != nil {
		ownerName = account.Student.NamaLengkap
	} else if account.User != nil {
		ownerName = account.User.Name
	}

	logUserAction(tx, c, "Restore Akun Laundry", fmt.Sprintf("Akun %s milik %s dikembalikan dari tempat sampah.", account.NomorLaundry, ownerName))

	tx.Commit()
	return c.JSON(fiber.Map{"message": "Account restored successfully"})
}

// ForceDelete permanently removes an account from the database
func (h *LaundryAccountHandler) ForceDelete(c *fiber.Ctx) error {
	id := c.Params("id")
	var account models.LaundryAccount

	if err := h.db.Unscoped().Preload("Student").Preload("User").First(&account, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Message: "Account not found"})
	}

	tx := h.db.Begin()

	var txCount int64
	tx.Unscoped().Model(&models.LaundryTransaction{}).Where("laundry_account_id = ?", account.ID).Count(&txCount)

	// Permanently delete transactions
	if err := tx.Unscoped().Where("laundry_account_id = ?", account.ID).Delete(&models.LaundryTransaction{}).Error; err != nil {
		tx.Rollback()
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Message: "Failed to permanently delete transactions"})
	}

	// Permanently delete account
	if err := tx.Unscoped().Delete(&account).Error; err != nil {
		tx.Rollback()
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Message: "Failed to permanently delete account"})
	}

	ownerName := "Unknown"
	if account.Student != nil {
		ownerName = account.Student.NamaLengkap
	} else if account.User != nil {
		ownerName = account.User.Name
	}

	logUserAction(tx, c, "Force Delete Akun Laundry", fmt.Sprintf("Akun %s milik %s dihapus permanen beserta %d transaksi.", account.NomorLaundry, ownerName, txCount))

	tx.Commit()
	return c.JSON(fiber.Map{"message": "Account permanently deleted"})
}
