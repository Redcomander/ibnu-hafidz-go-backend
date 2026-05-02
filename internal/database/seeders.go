package database

import (
	"log"

	"github.com/ibnu-hafidz/web-v2/internal/models"
	"gorm.io/gorm"
)

// SeedPermissions seeds the database with all permissions used in the app.
// Safe to run multiple times — all operations are idempotent.
func SeedPermissions(db *gorm.DB) error {
	permissions := []string{
		// Kelas
		"kelas.view", "kelas.create", "kelas.edit", "kelas.delete",
		// Prestasi
		"prestasi.view", "prestasi.create", "prestasi.edit", "prestasi.delete",
		// Students
		"students.view", "students.create", "students.edit", "students.delete",
		// Users & Roles
		"users.view", "users.create", "users.edit", "users.delete",
		"roles.view", "roles.create", "roles.edit", "roles.delete",
		"permissions.create",
		// Dashboard
		"dashboard.view",
		// Kamar
		"kamar.view", "kamar.create", "kamar.edit", "kamar.delete",
		// Curriculum & Lessons
		"curriculum.create", "curriculum.edit", "curriculum.delete",
		"lessons.view",
		// Jadwal Formal
		"jadwal_formal.create", "jadwal_formal.edit", "jadwal_formal.delete", "jadwal_formal.view_all",
		// Jadwal Diniyyah
		"jadwal_diniyyah.edit", "jadwal_diniyyah.view_all",
		// Halaqoh
		"halaqoh-assignments.create", "halaqoh-assignments.edit", "halaqoh-assignments.delete",
		"halaqoh.view_all",
		// Schedule / Attendance
		"schedule.substitute",
		// Articles
		"article.view", "article.create", "article.edit", "article.delete",
		// Gallery
		"gallery.view", "gallery.create", "gallery.edit", "gallery.delete",
		// Laundry
		"laundry.view", "laundry.create", "laundry.edit", "laundry.delete",
		// Absensi Ekstra
		"absensi_ekstra.view_all", "absensi_ekstra.create", "absensi_ekstra.edit", "absensi_ekstra.delete",
		// Settings
		"settings.edit",
	}

	// Ensure all permissions exist (idempotent via FirstOrCreate)
	for _, permName := range permissions {
		var perm models.Permission
		if err := db.FirstOrCreate(&perm, models.Permission{Name: permName}).Error; err != nil {
			log.Printf("Failed to create permission %s: %v", permName, err)
			return err
		}
	}

	// Load all permissions from DB into a map
	var allPerms []models.Permission
	if err := db.Where("name IN ?", permissions).Find(&allPerms).Error; err != nil {
		return err
	}

	// Assign to both admin and super_admin roles
	roleNames := []string{"admin", "super_admin"}
	for _, roleName := range roleNames {
		var role models.Role
		if err := db.Where("name = ?", roleName).First(&role).Error; err != nil {
			log.Printf("Role '%s' not found, skipping: %v", roleName, err)
			continue
		}

		// Load existing role permissions to diff — avoids duplicate pivot rows
		var existing []models.Permission
		db.Model(&role).Association("Permissions").Find(&existing)
		existingSet := make(map[uint]bool, len(existing))
		for _, p := range existing {
			existingSet[p.ID] = true
		}

		// Only append permissions not already assigned
		var toAdd []models.Permission
		for _, p := range allPerms {
			if !existingSet[p.ID] {
				toAdd = append(toAdd, p)
			}
		}

		if len(toAdd) > 0 {
			if err := db.Model(&role).Association("Permissions").Append(toAdd); err != nil {
				log.Printf("Failed to assign permissions to role '%s': %v", roleName, err)
				return err
			}
			log.Printf("✅ Assigned %d new permissions to role '%s'", len(toAdd), roleName)
		} else {
			log.Printf("✅ Role '%s' already has all permissions, nothing to add", roleName)
		}
	}

	log.Println("✅ Permissions seeded successfully")
	return nil
}
