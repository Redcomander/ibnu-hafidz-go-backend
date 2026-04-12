package handlers

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"gorm.io/gorm"
)

type SidebarMenuSettingHandler struct {
	db *gorm.DB
}

type sidebarMenuDefinition struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

var sidebarMenuDefinitions = []sidebarMenuDefinition{
	{Key: "students", Label: "Santri"},
	{Key: "student_arrival", Label: "Absen Kedatangan"},
	{Key: "teachers", Label: "Pengajar"},
	{Key: "kelas", Label: "Kelas"},
	{Key: "lessons", Label: "Pelajaran Formal"},
	{Key: "jadwal_formal", Label: "Jadwal Formal"},
	{Key: "jadwal_formal_ramadhan", Label: "Jadwal Formal Ramadhan"},
	{Key: "absensi_formal_ramadhan", Label: "Absensi Formal Ramadhan"},
	{Key: "absensi_formal_history", Label: "Riwayat Absensi Formal"},
	{Key: "diniyyah_lesson", Label: "Pelajaran Diniyyah"},
	{Key: "jadwal_diniyyah", Label: "Jadwal Diniyyah"},
	{Key: "absensi_diniyyah_history", Label: "Riwayat Absensi Diniyyah"},
	{Key: "halaqoh", Label: "Halaqoh"},
	{Key: "halaqoh_history", Label: "Riwayat Absensi Halaqoh"},
	{Key: "absensi_ekstra", Label: "Absensi Ekstra"},
	{Key: "kamar", Label: "Data Kamar"},
	{Key: "content", Label: "Berita"},
	{Key: "gallery", Label: "Galeri"},
	{Key: "prestasi", Label: "Prestasi"},
	{Key: "laundry_accounts", Label: "Manajemen Akun Laundry"},
	{Key: "laundry_transactions", Label: "Transaksi Laundry"},
}

func NewSidebarMenuSettingHandler(db *gorm.DB) *SidebarMenuSettingHandler {
	return &SidebarMenuSettingHandler{db: db}
}

func (h *SidebarMenuSettingHandler) List(c *fiber.Ctx) error {
	var stored []models.SidebarMenuSetting
	if err := h.db.Find(&stored).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "server_error",
			Message: "Gagal mengambil pengaturan sidebar",
		})
	}

	storedMap := make(map[string]bool, len(stored))
	for _, item := range stored {
		storedMap[strings.TrimSpace(item.MenuKey)] = item.IsActive
	}

	resolved := make([]fiber.Map, 0, len(sidebarMenuDefinitions))
	for _, item := range sidebarMenuDefinitions {
		isActive, ok := storedMap[item.Key]
		if !ok {
			isActive = true
		}
		resolved = append(resolved, fiber.Map{
			"key":       item.Key,
			"label":     item.Label,
			"is_active": isActive,
		})
	}

	return c.JSON(fiber.Map{"menu_settings": resolved})
}

func (h *SidebarMenuSettingHandler) Update(c *fiber.Ctx) error {
	user, err := h.currentUser(c)
	if err != nil {
		return err
	}
	if !user.HasRole("super_admin") {
		return c.Status(fiber.StatusForbidden).JSON(models.ErrorResponse{
			Error:   "forbidden",
			Message: "Hanya super admin yang dapat mengatur menu sidebar.",
		})
	}

	type updateRequest struct {
		Settings map[string]bool `json:"settings"`
	}

	var req updateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "bad_request",
			Message: "Body request tidak valid",
		})
	}

	for _, item := range sidebarMenuDefinitions {
		isActive := false
		if req.Settings != nil {
			isActive = req.Settings[item.Key]
		}

		var setting models.SidebarMenuSetting
		err := h.db.Where("menu_key = ?", item.Key).First(&setting).Error
		if err != nil && err != gorm.ErrRecordNotFound {
			return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
				Error:   "server_error",
				Message: "Gagal menyimpan pengaturan sidebar",
			})
		}

		if err == gorm.ErrRecordNotFound {
			setting = models.SidebarMenuSetting{
				MenuKey:  item.Key,
				IsActive: isActive,
			}
			err = h.db.Create(&setting).Error
		} else {
			err = h.db.Model(&setting).Update("is_active", isActive).Error
		}

		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
				Error:   "server_error",
				Message: "Gagal menyimpan pengaturan sidebar",
			})
		}
	}

	return c.JSON(fiber.Map{"message": "Pengaturan menu sidebar berhasil diperbarui."})
}

func (h *SidebarMenuSettingHandler) currentUser(c *fiber.Ctx) (*models.User, error) {
	if existing, ok := c.Locals("user").(*models.User); ok && existing != nil {
		return existing, nil
	}

	userID, ok := c.Locals("userID").(uint)
	if !ok || userID == 0 {
		return nil, c.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse{
			Error:   "unauthorized",
			Message: "User tidak terautentikasi",
		})
	}

	var user models.User
	if err := h.db.Preload("Roles.Permissions").First(&user, userID).Error; err != nil {
		return nil, c.Status(fiber.StatusForbidden).JSON(models.ErrorResponse{
			Error:   "forbidden",
			Message: "User tidak ditemukan",
		})
	}

	return &user, nil
}
