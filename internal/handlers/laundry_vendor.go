package handlers

import (
	"fmt"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"gorm.io/gorm"
)

type LaundryVendorHandler struct {
	db *gorm.DB
}

func NewLaundryVendorHandler(db *gorm.DB) *LaundryVendorHandler {
	return &LaundryVendorHandler{db: db}
}

// List returns all vendors with optional filters
func (h *LaundryVendorHandler) List(c *fiber.Ctx) error {
	var vendors []models.LaundryVendor
	var total int64

	page := c.QueryInt("page", 1)
	perPage := c.QueryInt("per_page", 20)
	search := c.Query("search")
	genderType := c.Query("gender_type")

	query := h.db.Model(&models.LaundryVendor{})

	if search != "" {
		query = query.Where("name LIKE ? OR phone LIKE ?", "%"+search+"%", "%"+search+"%")
	}

	if genderType != "" && genderType != "all" {
		query = query.Where("gender_type = ?", genderType)
	}

	query.Count(&total)

	// In Laravel, the response included `accounts_count`
	// We can preload Accounts, but since we just need the count, we'll keep it simple
	// or we can select a raw count. For now, Preloading Accounts may be heavy if they have many.
	// But let's retrieve to get accurate counts if needed.
	offset := (page - 1) * perPage
	query.Order("created_at desc").Limit(perPage).Offset(offset).Preload("Accounts").Find(&vendors)

	// Calculate custom stats as in old app
	type VendorWithStats struct {
		models.LaundryVendor
		AccountsCount int `json:"accounts_count"`
	}

	var results []VendorWithStats
	for _, v := range vendors {
		results = append(results, VendorWithStats{
			LaundryVendor: v,
			AccountsCount: len(v.Accounts),
		})
	}

	return c.JSON(fiber.Map{
		"data":        results,
		"total":       total,
		"page":        page,
		"per_page":    perPage,
		"total_pages": (total + int64(perPage) - 1) / int64(perPage),
	})
}

// Create adds a new vendor
func (h *LaundryVendorHandler) Create(c *fiber.Ctx) error {
	type CreateRequest struct {
		Name       string `json:"name"`
		Phone      string `json:"phone"`
		Address    string `json:"address"`
		GenderType string `json:"gender_type"`
		Active     bool   `json:"active"`
	}

	var req CreateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Message: "Invalid input"})
	}

	vendor := models.LaundryVendor{
		Name:       req.Name,
		Phone:      req.Phone,
		Address:    req.Address,
		GenderType: req.GenderType,
		Active:     req.Active,
	}

	if err := h.db.Create(&vendor).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Message: "Failed to create vendor"})
	}

	// Log Action
	logUserAction(h.db, c, "Tambah Vendor Laundry", "Menambahkan vendor "+vendor.Name)

	return c.Status(fiber.StatusCreated).JSON(vendor)
}

// Update modifies a vendor
func (h *LaundryVendorHandler) Update(c *fiber.Ctx) error {
	id := c.Params("id")
	var vendor models.LaundryVendor

	if err := h.db.First(&vendor, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Message: "Vendor not found"})
	}

	type UpdateRequest struct {
		Name       string `json:"name"`
		Phone      string `json:"phone"`
		Address    string `json:"address"`
		GenderType string `json:"gender_type"`
		Active     *bool  `json:"active"`
	}

	var req UpdateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Message: "Invalid input"})
	}

	if req.Name != "" {
		vendor.Name = req.Name
	}
	if req.Phone != "" {
		vendor.Phone = req.Phone
	}
	if req.Address != "" {
		vendor.Address = req.Address
	}
	if req.GenderType != "" {
		vendor.GenderType = req.GenderType
	}
	if req.Active != nil {
		vendor.Active = *req.Active
	}

	if err := h.db.Save(&vendor).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Message: "Failed to update vendor"})
	}

	// Log Action
	logUserAction(h.db, c, "Ubah Vendor Laundry", "Memperbarui vendor "+vendor.Name)

	return c.JSON(vendor)
}

// Delete removes a vendor
func (h *LaundryVendorHandler) Delete(c *fiber.Ctx) error {
	id := c.Params("id")
	var vendor models.LaundryVendor

	if err := h.db.First(&vendor, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Message: "Vendor not found"})
	}

	name := vendor.Name
	if err := h.db.Delete(&vendor).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Message: "Failed to delete vendor"})
	}

	// Log Action
	logUserAction(h.db, c, "Hapus Vendor Laundry", "Menghapus vendor "+name)

	return c.JSON(fiber.Map{"message": "Vendor deleted successfully"})
}

// TransferAccountsRandom moves a random number of active accounts from source vendors to a target vendor.
// The target vendor is provided via path param :id.
func (h *LaundryVendorHandler) TransferAccountsRandom(c *fiber.Ctx) error {
	targetID, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil || targetID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Message: "Target vendor tidak valid"})
	}

	var req struct {
		AmountPerVendor int    `json:"amount_per_vendor"`
		SourceVendorIDs []uint `json:"source_vendor_ids"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Message: "Body request tidak valid"})
	}
	if req.AmountPerVendor <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Message: "Jumlah akun per vendor harus lebih dari 0"})
	}

	var target models.LaundryVendor
	if err := h.db.First(&target, uint(targetID)).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Message: "Vendor tujuan tidak ditemukan"})
	}

	var sources []models.LaundryVendor
	if len(req.SourceVendorIDs) > 0 {
		if err := h.db.
			Where("id IN ? AND id <> ? AND gender_type = ?", req.SourceVendorIDs, uint(targetID), target.GenderType).
			Find(&sources).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Message: "Gagal memuat vendor sumber"})
		}
	} else {
		if err := h.db.
			Where("id <> ? AND gender_type = ?", uint(targetID), target.GenderType).
			Find(&sources).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Message: "Gagal memuat vendor sumber"})
		}
	}

	if len(sources) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Message: "Tidak ada vendor sumber yang cocok untuk transfer"})
	}

	type transferDetail struct {
		SourceVendorID uint   `json:"source_vendor_id"`
		SourceName     string `json:"source_name"`
		Requested      int    `json:"requested"`
		Moved          int    `json:"moved"`
	}

	details := make([]transferDetail, 0, len(sources))
	totalMoved := 0

	err = h.db.Transaction(func(tx *gorm.DB) error {
		for _, source := range sources {
			var accountIDs []uint
			if err := tx.Model(&models.LaundryAccount{}).
				Where("vendor_id = ? AND active = ?", source.ID, true).
				Where("deleted_at IS NULL").
				Order("RAND()").
				Limit(req.AmountPerVendor).
				Pluck("id", &accountIDs).Error; err != nil {
				return err
			}

			moved := 0
			if len(accountIDs) > 0 {
				result := tx.Model(&models.LaundryAccount{}).
					Where("id IN ?", accountIDs).
					Update("vendor_id", target.ID)
				if result.Error != nil {
					return result.Error
				}
				moved = int(result.RowsAffected)
				totalMoved += moved
			}

			details = append(details, transferDetail{
				SourceVendorID: source.ID,
				SourceName:     source.Name,
				Requested:      req.AmountPerVendor,
				Moved:          moved,
			})
		}
		return nil
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Message: "Gagal melakukan transfer akun"})
	}

	logUserAction(h.db, c, "Transfer Akun Laundry Acak", fmt.Sprintf("Memindahkan %d akun ke vendor %s dari %d vendor sumber", totalMoved, target.Name, len(sources)))

	return c.JSON(fiber.Map{
		"message":            "Transfer akun selesai",
		"target_vendor_id":   target.ID,
		"target_vendor_name": target.Name,
		"amount_per_vendor":  req.AmountPerVendor,
		"total_moved":        totalMoved,
		"details":            details,
	})
}

// EqualizeAccounts redistributes active accounts across selected vendors so counts are as even as possible.
func (h *LaundryVendorHandler) EqualizeAccounts(c *fiber.Ctx) error {
	var req struct {
		VendorIDs []uint `json:"vendor_ids"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Message: "Body request tidak valid"})
	}
	if len(req.VendorIDs) < 2 {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Message: "Pilih minimal 2 vendor untuk pemerataan"})
	}

	var vendors []models.LaundryVendor
	if err := h.db.Where("id IN ?", req.VendorIDs).Find(&vendors).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Message: "Gagal memuat vendor"})
	}
	if len(vendors) < 2 {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Message: "Vendor yang dipilih tidak valid"})
	}

	gender := vendors[0].GenderType
	for _, v := range vendors {
		if v.GenderType != gender {
			return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Message: "Pemerataan hanya dapat dilakukan pada vendor dengan kategori gender yang sama"})
		}
	}

	type countRow struct {
		VendorID uint
		Count    int64
	}

	countMap := make(map[uint]int)
	for _, v := range vendors {
		countMap[v.ID] = 0
	}

	var countRows []countRow
	if err := h.db.Model(&models.LaundryAccount{}).
		Select("vendor_id, COUNT(*) as count").
		Where("vendor_id IN ? AND active = ?", req.VendorIDs, true).
		Where("deleted_at IS NULL").
		Group("vendor_id").
		Scan(&countRows).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Message: "Gagal menghitung akun vendor"})
	}

	totalAccounts := 0
	for _, row := range countRows {
		countMap[row.VendorID] = int(row.Count)
		totalAccounts += int(row.Count)
	}

	targetBase := 0
	remainder := 0
	if len(vendors) > 0 {
		targetBase = totalAccounts / len(vendors)
		remainder = totalAccounts % len(vendors)
	}

	targetMap := make(map[uint]int)
	for _, v := range vendors {
		targetMap[v.ID] = targetBase
	}
	for i := 0; i < remainder; i++ {
		targetMap[vendors[i].ID]++
	}

	type vendorState struct {
		VendorID uint
		Name     string
		Current  int
		Target   int
		Diff     int
	}

	states := make(map[uint]*vendorState)
	for _, v := range vendors {
		states[v.ID] = &vendorState{
			VendorID: v.ID,
			Name:     v.Name,
			Current:  countMap[v.ID],
			Target:   targetMap[v.ID],
			Diff:     countMap[v.ID] - targetMap[v.ID],
		}
	}

	type movement struct {
		FromVendorID uint   `json:"from_vendor_id"`
		FromName     string `json:"from_name"`
		ToVendorID   uint   `json:"to_vendor_id"`
		ToName       string `json:"to_name"`
		Moved        int    `json:"moved"`
	}
	movements := make([]movement, 0)
	totalMoved := 0

	err := h.db.Transaction(func(tx *gorm.DB) error {
		for {
			var donor *vendorState
			var receiver *vendorState

			for _, st := range states {
				if st.Diff > 0 {
					if donor == nil || st.Diff > donor.Diff {
						donor = st
					}
				}
				if st.Diff < 0 {
					if receiver == nil || st.Diff < receiver.Diff {
						receiver = st
					}
				}
			}

			if donor == nil || receiver == nil {
				break
			}

			moveCount := donor.Diff
			if -receiver.Diff < moveCount {
				moveCount = -receiver.Diff
			}
			if moveCount <= 0 {
				break
			}

			var accountIDs []uint
			if err := tx.Model(&models.LaundryAccount{}).
				Where("vendor_id = ? AND active = ?", donor.VendorID, true).
				Where("deleted_at IS NULL").
				Order("RAND()").
				Limit(moveCount).
				Pluck("id", &accountIDs).Error; err != nil {
				return err
			}
			if len(accountIDs) == 0 {
				donor.Diff = 0
				continue
			}

			result := tx.Model(&models.LaundryAccount{}).
				Where("id IN ?", accountIDs).
				Update("vendor_id", receiver.VendorID)
			if result.Error != nil {
				return result.Error
			}

			moved := int(result.RowsAffected)
			if moved == 0 {
				break
			}

			donor.Diff -= moved
			receiver.Diff += moved
			totalMoved += moved

			movements = append(movements, movement{
				FromVendorID: donor.VendorID,
				FromName:     donor.Name,
				ToVendorID:   receiver.VendorID,
				ToName:       receiver.Name,
				Moved:        moved,
			})
		}

		return nil
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Message: "Gagal melakukan pemerataan akun"})
	}

	finalCounts := make(map[uint]int)
	for _, st := range states {
		finalCounts[st.VendorID] = st.Target + st.Diff
	}

	logUserAction(h.db, c, "Pemerataan Akun Laundry", fmt.Sprintf("Pemerataan %d akun antar %d vendor", totalMoved, len(vendors)))

	return c.JSON(fiber.Map{
		"message":        "Pemerataan akun selesai",
		"total_moved":    totalMoved,
		"target_base":    targetBase,
		"total_accounts": totalAccounts,
		"movements":      movements,
		"final_counts":   finalCounts,
	})
}

// Statistics returns vendor statistics for a given period
func (h *LaundryVendorHandler) Statistics(c *fiber.Ctx) error {
	period := c.Query("period", "this_week")
	startDateStr := c.Query("start_date")
	endDateStr := c.Query("end_date")
	genderType := c.Query("gender_type", "all")

	// Calculate date range based on period exactly like Laravel
	var start, end time.Time
	now := time.Now()

	if startDateStr != "" && endDateStr != "" {
		start, _ = time.Parse("2006-01-02", startDateStr)
		end, _ = time.Parse("2006-01-02", endDateStr)
	} else if period == "last_week" {
		start = getStartOfWeek(now.AddDate(0, 0, -7))
		end = getEndOfWeek(now.AddDate(0, 0, -7))
	} else {
		// this_week
		start = getStartOfWeek(now)
		end = getEndOfWeek(now)
	}

	start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
	end = time.Date(end.Year(), end.Month(), end.Day(), 23, 59, 59, 0, end.Location())

	// If a specific vendor ID is requested
	vendorID := c.Params("id")

	if vendorID != "" && vendorID != "all" {
		var vendor models.LaundryVendor
		if err := h.db.First(&vendor, vendorID).Error; err != nil {
			return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Message: "Vendor not found"})
		}

		// Calculate total kg and rupiah for this vendor in the date range
		var totalKg, totalRupiah float64
		// Join through LaundryAccount and LaundryTransaction
		err := h.db.Model(&models.LaundryTransaction{}).
			Joins("JOIN laundry_accounts ON laundry_accounts.id = laundry_transactions.laundry_account_id").
			Where("laundry_accounts.vendor_id = ?", vendor.ID).
			Where("laundry_transactions.tanggal BETWEEN ? AND ?", start, end).
			Select("COALESCE(SUM(berat_kg), 0) as total_kg, COALESCE(SUM(total_harga), 0) as total_rupiah").
			Row().Scan(&totalKg, &totalRupiah)

		if err != nil {
			// Log error
		}

		return c.JSON(fiber.Map{
			"vendor":       vendor,
			"total_kg":     totalKg,
			"total_rupiah": totalRupiah,
			"start_date":   start.Format("2006-01-02"),
			"end_date":     end.Format("2006-01-02"),
		})
	}

	// All vendors
	query := h.db.Model(&models.LaundryVendor{})
	if genderType != "all" {
		query = query.Where("gender_type = ?", genderType)
	}

	var allVendors []models.LaundryVendor
	query.Find(&allVendors)

	type VendorStat struct {
		ID          uint    `json:"id"`
		Name        string  `json:"name"`
		TotalKg     float64 `json:"total_kg"`
		TotalRupiah float64 `json:"total_rupiah"`
	}

	var stats []VendorStat
	var grandTotalKg, grandTotalRupiah float64

	for _, v := range allVendors {
		var totalKg, totalRupiah float64
		h.db.Model(&models.LaundryTransaction{}).
			Joins("JOIN laundry_accounts ON laundry_accounts.id = laundry_transactions.laundry_account_id").
			Where("laundry_accounts.vendor_id = ?", v.ID).
			Where("laundry_transactions.tanggal BETWEEN ? AND ?", start, end).
			Select("COALESCE(SUM(berat_kg), 0) as total_kg, COALESCE(SUM(total_harga), 0) as total_rupiah").
			Row().Scan(&totalKg, &totalRupiah)

		stats = append(stats, VendorStat{
			ID:          v.ID,
			Name:        v.Name,
			TotalKg:     totalKg,
			TotalRupiah: totalRupiah,
		})

		grandTotalKg += totalKg
		grandTotalRupiah += totalRupiah
	}

	// Actually count properly:
	var activeCount int64
	queryActive := h.db.Model(&models.LaundryVendor{}).Where("active = ?", true)
	if genderType != "all" {
		queryActive = queryActive.Where("gender_type = ?", genderType)
	}
	queryActive.Count(&activeCount)

	return c.JSON(fiber.Map{
		"vendors":              stats,
		"grand_total_kg":       grandTotalKg,
		"grand_total_rupiah":   grandTotalRupiah,
		"active_vendors_count": activeCount,
		"start_date":           start.Format("2006-01-02"),
		"end_date":             end.Format("2006-01-02"),
	})
}

// Helper to log actions
func logUserAction(db *gorm.DB, c *fiber.Ctx, action string, description string) {
	userIDStr := c.Locals("user_id")
	if userIDStr != nil {
		if uidStr, ok := userIDStr.(string); ok {
			userID, _ := strconv.Atoi(uidStr)
			uid := uint(userID)
			db.Create(&models.LaundryLog{
				UserID:      &uid,
				Action:      action,
				Description: description,
			})
		} else if uidFloat, ok := userIDStr.(float64); ok {
			uid := uint(uidFloat)
			db.Create(&models.LaundryLog{
				UserID:      &uid,
				Action:      action,
				Description: description,
			})
		}
	}
}

// Date helpers
func getStartOfWeek(t time.Time) time.Time {
	weekday := int(t.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	return t.AddDate(0, 0, -weekday+1)
}

func getEndOfWeek(t time.Time) time.Time {
	start := getStartOfWeek(t)
	return start.AddDate(0, 0, 6)
}
