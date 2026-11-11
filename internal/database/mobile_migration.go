package database

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"gorm.io/gorm"
	"strings"
	"time"
)

func mobileReleaseSQL() string {
	b, e := migrationFiles.ReadFile("migrations/002_mobile_releases.sql")
	if e != nil {
		panic(e)
	}
	return string(b)
}
func mobileReleaseChecksum() string {
	sum := sha256.Sum256([]byte(mobileReleaseSQL()))
	return hex.EncodeToString(sum[:])
}

// ApplyMobileReleases is only called by the explicit operator CLI, never at startup.
func ApplyMobileReleases(db *gorm.DB) error {
	if db.Dialector.Name() != "mysql" {
		return errors.New("mobile release migration requires MySQL; rehearse on a restored MySQL database")
	}
	return db.Connection(func(conn *gorm.DB) error {
		var name string
		if e := conn.Raw("SELECT DATABASE()").Scan(&name).Error; e != nil {
			return e
		}
		sum := sha256.Sum256([]byte(name))
		lock := "ibnu-schema-" + hex.EncodeToString(sum[:20])
		var acquired int
		if e := conn.Raw("SELECT GET_LOCK(?, 0)", lock).Scan(&acquired).Error; e != nil {
			return e
		}
		if acquired != 1 {
			return errors.New("another migration command holds the database lock")
		}
		defer conn.Exec("SELECT RELEASE_LOCK(?)", lock)
		if e := CheckMigrations(conn); e != nil {
			return e
		}
		var count int64
		if e := conn.Model(&migrationRecord{}).Where("version = 2").Count(&count).Error; e != nil {
			return e
		}
		if count > 0 {
			return nil
		}
		row := migrationRecord{Version: 2, Name: "mobile_releases", Checksum: mobileReleaseChecksum(), Dirty: true, SchemaSnapshot: "{}", AppliedAt: time.Now().UTC()}
		if e := conn.Create(&row).Error; e != nil {
			return e
		}
		for _, statement := range strings.Split(mobileReleaseSQL(), ";") {
			if strings.TrimSpace(statement) != "" {
				if e := conn.Exec(statement).Error; e != nil {
					return e
				}
			}
		}
		if e := conn.Model(&migrationRecord{}).Where("version = 2").Update("dirty", false).Error; e != nil {
			return e
		}
		return CheckMigrations(conn)
	})
}
