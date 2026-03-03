package database

import (
	"log"

	"github.com/ibnu-hafidz/web-v2/internal/models"
	"gorm.io/gorm"
)

// SeedPermissions seeds the database with default permissions
func SeedPermissions(db *gorm.DB) error {
	permissions := []string{
		"kelas.view",
		"kelas.create",
		"kelas.edit",
		"kelas.delete",
		"prestasi.view",
		"prestasi.create",
		"prestasi.edit",
		"prestasi.delete",
	}

	for _, permName := range permissions {
		var perm models.Permission
		if err := db.FirstOrCreate(&perm, models.Permission{Name: permName}).Error; err != nil {
			log.Printf("Failed to create permission %s: %v", permName, err)
			return err
		}
	}

	// Assign to Admin role (assuming ID 1 or name 'admin')
	var adminRole models.Role
	if err := db.Where("name = ?", "admin").First(&adminRole).Error; err == nil {
		var perms []models.Permission
		db.Where("name IN ?", permissions).Find(&perms)
		db.Model(&adminRole).Association("Permissions").Append(perms)
	}

	log.Println("✅ Permissions seeded successfully")
	return nil
}
