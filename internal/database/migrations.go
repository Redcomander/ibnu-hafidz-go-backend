package database

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

//go:embed migrations/001_existing_schema.sql
var migrationFiles embed.FS

const migrationTable = "app_schema_migrations"
const baselineVersion int64 = 1
const baselineName = "existing_schema"

type migrationRecord struct {
	Version        int64 `gorm:"primaryKey"`
	Name           string
	Checksum       string
	Dirty          bool
	SchemaSnapshot string
	AppliedAt      time.Time
}

func (migrationRecord) TableName() string { return migrationTable }

func baselineSQL() string {
	data, err := migrationFiles.ReadFile("migrations/001_existing_schema.sql")
	if err != nil {
		panic(err)
	} // Embedded at build time.
	return string(data)
}

func baselineChecksum() string {
	sum := sha256.Sum256([]byte(baselineSQL()))
	return hex.EncodeToString(sum[:])
}

// CheckMigrations performs reads only, including when the metadata table is absent.
// It intentionally does not run AutoMigrate, fix legacy columns, or seed permissions.
func CheckMigrations(db *gorm.DB) error {
	db = db.Session(&gorm.Session{NewDB: true})
	if !db.Migrator().HasTable(migrationTable) {
		return errors.New("migration baseline missing; follow DATABASE_MIGRATIONS.md before starting this release")
	}
	var rows []migrationRecord
	if err := db.Order("version").Find(&rows).Error; err != nil {
		return fmt.Errorf("read migration history: %w", err)
	}
	if len(rows) != 1 {
		return fmt.Errorf("unexpected migration history (%d entries); refusing to change it", len(rows))
	}
	row := rows[0]
	if row.Version != baselineVersion || row.Name != baselineName || row.Checksum != baselineChecksum() || row.Dirty {
		return errors.New("migration version/checksum mismatch or unfinished migration; restore compatible code or investigate; no automatic repair performed")
	}
	// A marker alone must not permit an empty/missing application schema to start.
	_, err := captureSchema(db)
	return err
}

type tableDefinition struct {
	Name string `json:"name"`
	SQL  string `json:"sql"`
}

// captureSchema reads definitions, never application rows or credentials.
func captureSchema(db *gorm.DB) ([]tableDefinition, error) {
	db = db.Session(&gorm.Session{NewDB: true})
	var tables []tableDefinition
	switch db.Dialector.Name() {
	case "mysql":
		var names []string
		err := db.Raw("SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_TYPE = 'BASE TABLE' AND TABLE_NAME <> ? ORDER BY TABLE_NAME", migrationTable).Scan(&names).Error
		if err != nil {
			return nil, err
		}
		for _, name := range names {
			rows, err := db.Raw("SHOW CREATE TABLE `" + strings.ReplaceAll(name, "`", "``") + "`").Rows()
			if err != nil {
				return nil, err
			}
			var tableName, definition string
			if !rows.Next() {
				rows.Close()
				return nil, fmt.Errorf("missing definition for %s", name)
			}
			err = rows.Scan(&tableName, &definition)
			rows.Close()
			if err != nil {
				return nil, err
			}
			tables = append(tables, tableDefinition{Name: tableName, SQL: definition})
		}
	case "sqlite": // Isolated regression tests; production Connect uses MySQL.
		if err := db.Raw("SELECT name, sql FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' AND name <> ? ORDER BY name", migrationTable).Scan(&tables).Error; err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported migration dialect: %s", db.Dialector.Name())
	}
	found := map[string]bool{}
	for _, table := range tables {
		found[table.Name] = true
	}
	if !found["users"] || !found["students"] {
		return nil, errors.New("expected an existing application database with users and students tables; empty databases require a separately reviewed schema import")
	}
	return tables, nil
}

// RegisterBaseline adopts an existing schema. The only writes are creation of
// the dedicated metadata table and insertion of its baseline record. Existing
// table definitions and business rows are never altered, copied, seeded or deleted.
func RegisterBaseline(db *gorm.DB) error {
	return db.Connection(func(conn *gorm.DB) error {
		conn = conn.Session(&gorm.Session{NewDB: true})
		if conn.Dialector.Name() == "mysql" {
			// Advisory locks are connection-scoped, so all work stays on this connection.
			var databaseName string
			if err := conn.Raw("SELECT DATABASE()").Scan(&databaseName).Error; err != nil {
				return err
			}
			sum := sha256.Sum256([]byte(databaseName))
			lockName := "ibnu-schema-" + hex.EncodeToString(sum[:20])
			var acquired int
			if err := conn.Raw("SELECT GET_LOCK(?, 0)", lockName).Scan(&acquired).Error; err != nil {
				return err
			}
			if acquired != 1 {
				return errors.New("another migration command holds the database lock")
			}
			defer conn.Exec("SELECT RELEASE_LOCK(?)", lockName)
		}
		if conn.Migrator().HasTable(migrationTable) {
			// Never silently adopt an unrelated table, dirty entry, or altered history.
			return CheckMigrations(conn)
		}
		tables, err := captureSchema(conn)
		if err != nil {
			return err
		}
		snapshot, err := json.Marshal(tables)
		if err != nil {
			return err
		}
		if err := conn.Exec(baselineSQL()).Error; err != nil {
			return err
		}
		// MySQL DDL commits implicitly. If this insert fails, the empty metadata table
		// deliberately blocks startup; do not auto-delete or overwrite it on retry.
		row := migrationRecord{Version: baselineVersion, Name: baselineName, Checksum: baselineChecksum(), SchemaSnapshot: string(snapshot), AppliedAt: time.Now().UTC()}
		if err := conn.Create(&row).Error; err != nil {
			return fmt.Errorf("record baseline (metadata table may exist; investigate before retry): %w", err)
		}
		return CheckMigrations(conn)
	})
}
