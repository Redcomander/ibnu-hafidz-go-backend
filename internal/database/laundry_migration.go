package database

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"gorm.io/gorm"
	"time"
)

func laundryInputsSQL() string {
	b, err := migrationFiles.ReadFile("migrations/003_laundry_transaction_inputs.sql")
	if err != nil {
		panic(err)
	}
	return string(b)
}
func laundryInputsChecksum() string {
	sum := sha256.Sum256([]byte(laundryInputsSQL()))
	return hex.EncodeToString(sum[:])
}

// Explicit additive migration: no existing tables or business rows are changed.
func ApplyLaundryInputs(db *gorm.DB) error {
	if db.Dialector.Name() != "mysql" {
		return errors.New("laundry input migration requires MySQL; rehearse on a restored database")
	}
	return db.Connection(func(conn *gorm.DB) error {
		conn = conn.Session(&gorm.Session{NewDB: true})
		var name string
		if err := conn.Raw("SELECT DATABASE()").Scan(&name).Error; err != nil {
			return err
		}
		sum := sha256.Sum256([]byte(name))
		lock := "ibnu-schema-" + hex.EncodeToString(sum[:20])
		var acquired int
		if err := conn.Raw("SELECT GET_LOCK(?, 0)", lock).Scan(&acquired).Error; err != nil {
			return err
		}
		if acquired != 1 {
			return errors.New("another migration command holds the database lock")
		}
		defer conn.Exec("SELECT RELEASE_LOCK(?)", lock)
		if err := CheckMigrations(conn); err != nil {
			return err
		}
		var rows []migrationRecord
		if err := conn.Order("version").Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 3 {
			return nil
		}
		if len(rows) != 2 {
			return errors.New("apply mobile-releases migration before laundry-inputs")
		}
		if conn.Migrator().HasTable("laundry_transaction_inputs") {
			return errors.New("unregistered laundry input table exists; investigate before retrying")
		}
		record := migrationRecord{Version: 3, Name: "laundry_transaction_inputs", Checksum: laundryInputsChecksum(), Dirty: true, SchemaSnapshot: "{}", AppliedAt: time.Now().UTC()}
		if err := conn.Create(&record).Error; err != nil {
			return err
		}
		if err := conn.Exec(laundryInputsSQL()).Error; err != nil {
			return err
		}
		if err := conn.Model(&migrationRecord{}).Where("version = 3").Update("dirty", false).Error; err != nil {
			return err
		}
		return CheckMigrations(conn)
	})
}
