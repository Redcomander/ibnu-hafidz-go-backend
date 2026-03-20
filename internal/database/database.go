package database

import (
	"fmt"
	"log"

	"github.com/ibnu-hafidz/web-v2/internal/config"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Connect establishes connection to MySQL database
func Connect(cfg *config.Config) (*gorm.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		cfg.DBUser, cfg.DBPassword, cfg.DBHost, cfg.DBPort, cfg.DBName,
	)

	logLevel := logger.Silent
	if cfg.Environment == "development" {
		logLevel = logger.Info
	}

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get sql.DB: %w", err)
	}

	// Connection pool settings
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)

	log.Println("✅ Database connected successfully")
	return db, nil
}

// Migrate runs auto-migrations for all models
func Migrate(db *gorm.DB) error {
	log.Println("Running database migrations...")
	if err := db.AutoMigrate(
		&models.User{},
		&models.Role{},
		&models.Permission{},
		&models.Student{},
		&models.Visitor{},
		&models.Kamar{},
		&models.Kelas{},
	); err != nil {
		return err
	}

	// Migrate legacy class membership from kelas_siswa pivot (old app) to students.kelas_id
	if db.Migrator().HasTable("kelas_siswa") {
		log.Println("Migrating legacy kelas_siswa membership into students.kelas_id...")
		err := db.Exec(`
			UPDATE students s
			JOIN kelas_siswa ks ON ks.student_id = s.id
			SET s.kelas_id = ks.kelas_id
			WHERE s.kelas_id IS NULL
		`).Error
		if err != nil {
			return err
		}
	}

	return nil
}
