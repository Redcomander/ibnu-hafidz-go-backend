package handlers

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"gorm.io/gorm"
)

type LaundryTransactionHandler struct {
	db *gorm.DB
}

func NewLaundryTransactionHandler(db *gorm.DB) *LaundryTransactionHandler {
	return &LaundryTransactionHandler{db: db}
}

// List transactions
func (h *LaundryTransactionHandler) List(c *fiber.Ctx) error {
	var transactions []models.LaundryTransaction
	var total int64

	page := c.QueryInt("page", 1)
	perPage := c.QueryInt("per_page", 20)
	search := c.Query("search")
	status := c.Query("status") // pending or picked_up

	query := h.db.Model(&models.LaundryTransaction{}).
		Preload("Account").
		Preload("Account.Vendor").
		Preload("Account.Student").
		Preload("Account.User").
		Preload("PickedUpBy")

	if status != "" {
		query = query.Where("status = ?", status)
	}

	if search != "" {
		query = query.Joins("JOIN laundry_accounts ON laundry_accounts.id = laundry_transactions.laundry_account_id").
			Where(
				"laundry_accounts.nomor_laundry LIKE ? OR laundry_transactions.catatan LIKE ?",
				"%"+search+"%", "%"+search+"%",
			)
	}

	query.Count(&total)

	offset := (page - 1) * perPage
	query.Order("created_at desc").Limit(perPage).Offset(offset).Find(&transactions)

	return c.JSON(fiber.Map{
		"data":        transactions,
		"total":       total,
		"page":        page,
		"per_page":    perPage,
		"total_pages": (total + int64(perPage) - 1) / int64(perPage),
	})
}

// Get account stats for creating transaction
func (h *LaundryTransactionHandler) GetAccountStats(c *fiber.Ctx) error {
	accountID := c.Query("account_id")
	if accountID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Message: "Account ID required"})
	}

	var account models.LaundryAccount
	if err := h.db.First(&account, accountID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Message: "Account not found"})
	}

	now := time.Now()
	weekStart := getStartOfWeek(now)
	weekEnd := getEndOfWeek(now)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	monthEnd := monthStart.AddDate(0, 1, -1).Add(time.Hour*23 + time.Minute*59 + time.Second*59)

	var weeklyWeight float64
	h.db.Model(&models.LaundryTransaction{}).
		Where("laundry_account_id = ? AND tanggal BETWEEN ? AND ?", account.ID, weekStart, weekEnd).
		Select("COALESCE(SUM(berat_kg), 0)").Row().Scan(&weeklyWeight)

	var monthlyWeight float64
	h.db.Model(&models.LaundryTransaction{}).
		Where("laundry_account_id = ? AND tanggal BETWEEN ? AND ?", account.ID, monthStart, monthEnd).
		Select("COALESCE(SUM(berat_kg), 0)").Row().Scan(&monthlyWeight)

	weeklyLimit := 7.5
	monthlyLimit := 30.0

	return c.JSON(fiber.Map{
		"weekly": fiber.Map{
			"weight":       weeklyWeight,
			"limit":        weeklyLimit,
			"percentage":   (weeklyWeight / weeklyLimit) * 100,
			"exceeded":     weeklyWeight >= weeklyLimit,
			"period_start": weekStart.Format("02 Jan 2006"),
			"period_end":   weekEnd.Format("02 Jan 2006"),
		},
		"monthly": fiber.Map{
			"weight":     monthlyWeight,
			"limit":      monthlyLimit,
			"percentage": (monthlyWeight / monthlyLimit) * 100,
			"exceeded":   monthlyWeight >= monthlyLimit,
		},
	})
}

// Create new transaction
func (h *LaundryTransactionHandler) Create(c *fiber.Ctx) error {
	type CreateRequest struct {
		LaundryAccountID uint    `json:"laundry_account_id"`
		Tanggal          string  `json:"tanggal"`
		BeratKg          float64 `json:"berat_kg"`
		HargaPerKg       float64 `json:"harga_per_kg"`
		Catatan          *string `json:"catatan"`
	}

	var req CreateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Message: "Invalid input"})
	}

	var account models.LaundryAccount
	if err := h.db.Preload("Student").Preload("User").First(&account, req.LaundryAccountID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Message: "Akun laundry tidak ditemukan."})
	}

	if account.Blocked {
		return c.Status(fiber.StatusForbidden).JSON(models.ErrorResponse{Message: "Akun ini diblokir. Tidak dapat melakukan transaksi."})
	}

	tanggal, err := time.Parse("2006-01-02", req.Tanggal)
	if err != nil {
		// Try parsing ISO format if simple date fails
		tanggal, err = time.Parse(time.RFC3339, req.Tanggal)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Message: "Invalid date format, use YYYY-MM-DD or ISO8601"})
		}
	}

	tx := h.db.Begin()

	trans := models.LaundryTransaction{
		LaundryAccountID: req.LaundryAccountID,
		Tanggal:          tanggal,
		BeratKg:          req.BeratKg,
		HargaPerKg:       req.HargaPerKg,
		TotalHarga:       req.BeratKg * req.HargaPerKg,
		Catatan:          req.Catatan,
		Status:           "pending",
	}

	if err := tx.Create(&trans).Error; err != nil {
		tx.Rollback()
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Message: "Failed to create transaction"})
	}

	// Update account totals
	account.TotalTransactions++
	account.TotalKg += trans.BeratKg
	if err := tx.Save(&account).Error; err != nil {
		tx.Rollback()
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Message: "Failed to update account totals"})
	}

	// Update Vendor totals (just week totals, simplified as we query dynamically anyway)
	// Laravel code `updateWeeklyTotals`
	// but basically we can just calculate these dynamically instead.
	// We'll skip updating Vendor's raw columns for now to avoid complexity during insert,
	// or we can just trigger it if strictly needed.

	// Log Action
	logUserAction(tx, c, "Tambah Transaksi", "Menambahkan "+strconv.FormatFloat(trans.BeratKg, 'f', 2, 64)+" KG untuk Nomor Laundry: "+account.NomorLaundry)

	tx.Commit()

	return c.Status(fiber.StatusCreated).JSON(trans)
}

// MarkAsPickedUp
func (h *LaundryTransactionHandler) MarkAsPickedUp(c *fiber.Ctx) error {
	id := c.Params("id")
	var trans models.LaundryTransaction

	if err := h.db.Preload("Account").First(&trans, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Message: "Transaction not found"})
	}

	if trans.Status == "picked_up" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Message: "Laundry ini sudah diambil sebelumnya."})
	}

	userIDStr := c.Locals("user_id")
	var pickedUpByID *uint
	if userIDStr != nil {
		if uidStr, ok := userIDStr.(string); ok {
			userID, _ := strconv.Atoi(uidStr)
			uid := uint(userID)
			pickedUpByID = &uid
		} else if uidFloat, ok := userIDStr.(float64); ok {
			uid := uint(uidFloat)
			pickedUpByID = &uid
		}
	}

	now := time.Now()
	trans.Status = "picked_up"
	trans.PickedUpAt = &now
	trans.PickedUpByID = pickedUpByID

	if err := h.db.Save(&trans).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Message: "Failed to mark as picked up"})
	}

	logUserAction(h.db, c, "Pengambilan Laundry", "Laundry diambil untuk Nomor: "+trans.Account.NomorLaundry)

	return c.JSON(trans)
}

// BulkMarkAsPickedUp
func (h *LaundryTransactionHandler) BulkMarkAsPickedUp(c *fiber.Ctx) error {
	type BulkRequest struct {
		IDs []uint `json:"ids"`
	}

	var req BulkRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Message: "Invalid input"})
	}

	if len(req.IDs) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Message: "No transaction IDs provided"})
	}

	userIDStr := c.Locals("user_id")
	var pickedUpByID *uint
	if userIDStr != nil {
		if uidStr, ok := userIDStr.(string); ok {
			userID, _ := strconv.Atoi(uidStr)
			uid := uint(userID)
			pickedUpByID = &uid
		} else if uidFloat, ok := userIDStr.(float64); ok {
			uid := uint(uidFloat)
			pickedUpByID = &uid
		}
	}
	now := time.Now()

	res := h.db.Model(&models.LaundryTransaction{}).
		Where("id IN ? AND status = ?", req.IDs, "pending").
		Updates(map[string]interface{}{
			"status":          "picked_up",
			"picked_up_at":    now,
			"picked_up_by_id": pickedUpByID,
		})

	if res.Error != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Message: "Failed to update transactions"})
	}

	return c.JSON(fiber.Map{
		"message": "Transactions marked as picked up",
		"count":   res.RowsAffected,
	})
}

// Delete transaction
func (h *LaundryTransactionHandler) Delete(c *fiber.Ctx) error {
	id := c.Params("id")
	var trans models.LaundryTransaction

	if err := h.db.Preload("Account").First(&trans, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Message: "Transaction not found"})
	}

	tx := h.db.Begin()

	if err := tx.Delete(&trans).Error; err != nil {
		tx.Rollback()
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Message: "Failed to delete transaction"})
	}

	var account models.LaundryAccount
	if err := tx.First(&account, trans.LaundryAccountID).Error; err == nil {
		account.TotalTransactions--
		account.TotalKg -= trans.BeratKg
		tx.Save(&account)
	}

	logUserAction(tx, c, "Hapus Transaksi", "Menghapus transaksi "+strconv.FormatFloat(trans.BeratKg, 'f', 2, 64)+" KG untuk Nomor Laundry: "+trans.Account.NomorLaundry)

	tx.Commit()

	return c.JSON(fiber.Map{"message": "Transaction deleted successfully"})
}
