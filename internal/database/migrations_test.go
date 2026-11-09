package database

import (
	"fmt"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func migrationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.Close() })
	return db
}

func seedLegacyDatabase(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, statement := range []string{
		`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, password TEXT, deleted_at TEXT)`,
		`INSERT INTO users VALUES (41, 'Guru asli', 'untouched-hash', NULL)`,
		`CREATE TABLE students (id INTEGER PRIMARY KEY, kelas_id INTEGER, notes TEXT)`,
		`INSERT INTO students VALUES (9, NULL, 'Do not backfill my membership')`,
		`CREATE TABLE kelas_siswa (student_id INTEGER, kelas_id INTEGER)`,
		`INSERT INTO kelas_siswa VALUES (9, 3)`,
		`CREATE TABLE lessons (id INTEGER, name TEXT)`,
		`INSERT INTO lessons VALUES (1, 'Legacy name must survive')`,
		`CREATE TABLE teacher_attendances (id INTEGER, status TEXT, date TEXT, payload BLOB)`,
		`INSERT INTO teacher_attendances VALUES (1, 'Dinas Luar', '2026-11-09 08:12:34', X'0001FF')`,
		`CREATE TABLE absensi_ekstra_sessions (id INTEGER, created_by_id INTEGER, waktu_mulai TEXT)`,
		`INSERT INTO absensi_ekstra_sessions VALUES (5, 41, '2026-11-09 13:00:00')`,
		`CREATE TABLE permissions (id INTEGER, name TEXT)`,
		`INSERT INTO permissions VALUES (1, 'custom.permission')`,
	} {
		require.NoError(t, db.Exec(statement).Error)
	}
}

func businessSnapshot(t *testing.T, db *gorm.DB) map[string]any {
	t.Helper()
	tables, err := captureSchema(db)
	require.NoError(t, err)
	snapshot := map[string]any{"definitions": tables}
	for _, table := range tables {
		result, err := db.Raw(fmt.Sprintf("SELECT * FROM `%s` ORDER BY rowid", table.Name)).Rows()
		require.NoError(t, err)
		columns, err := result.Columns()
		require.NoError(t, err)
		var rows [][]any
		for result.Next() {
			values := make([]any, len(columns))
			targets := make([]any, len(columns))
			for i := range values {
				targets[i] = &values[i]
			}
			require.NoError(t, result.Scan(targets...))
			for i, value := range values {
				if data, ok := value.([]byte); ok {
					values[i] = append([]byte(nil), data...)
				}
			}
			rows = append(rows, values)
		}
		require.NoError(t, result.Err())
		require.NoError(t, result.Close())
		snapshot[table.Name] = rows
	}
	return snapshot
}

func TestBaselinePreservesAllLegacyRowsAndDefinitions(t *testing.T) {
	db := migrationTestDB(t)
	seedLegacyDatabase(t, db)
	before := businessSnapshot(t, db)
	require.Error(t, CheckMigrations(db))
	require.False(t, db.Migrator().HasTable(migrationTable), "status must not create metadata")
	require.NoError(t, RegisterBaseline(db))
	require.NoError(t, CheckMigrations(db))
	require.Equal(t, before, businessSnapshot(t, db), "values, legacy columns, memberships, types and permissions must survive unchanged")
	var first migrationRecord
	require.NoError(t, db.First(&first).Error)
	require.Contains(t, first.SchemaSnapshot, "created_by_id")
	require.NoError(t, RegisterBaseline(db))
	var second migrationRecord
	require.NoError(t, db.First(&second).Error)
	require.Equal(t, first, second, "repeating the command must not rewrite metadata")
	require.Equal(t, before, businessSnapshot(t, db))
}

func TestBaselineRefusesEmptyDatabaseWithoutWrites(t *testing.T) {
	db := migrationTestDB(t)
	require.ErrorContains(t, RegisterBaseline(db), "existing application database")
	require.False(t, db.Migrator().HasTable(migrationTable))
}

func TestHistoryFailuresNeverRepairOrModifyBusinessData(t *testing.T) {
	for _, change := range []struct{ name, sql string }{
		{"checksum", `UPDATE app_schema_migrations SET checksum = 'wrong'`},
		{"unfinished", `UPDATE app_schema_migrations SET dirty = 1`},
		{"future-version", `UPDATE app_schema_migrations SET version = 999`},
		{"empty-history", `DELETE FROM app_schema_migrations`},
	} {
		t.Run(change.name, func(t *testing.T) {
			db := migrationTestDB(t)
			seedLegacyDatabase(t, db)
			require.NoError(t, RegisterBaseline(db))
			before := businessSnapshot(t, db)
			require.NoError(t, db.Exec(change.sql).Error)
			require.Error(t, CheckMigrations(db))
			require.Error(t, RegisterBaseline(db))
			require.Equal(t, before, businessSnapshot(t, db))
		})
	}
}

func TestUnrelatedMetadataTableIsNotOverwritten(t *testing.T) {
	db := migrationTestDB(t)
	seedLegacyDatabase(t, db)
	require.NoError(t, db.Exec(`CREATE TABLE app_schema_migrations (note TEXT)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO app_schema_migrations VALUES ('keep me')`).Error)
	require.Error(t, RegisterBaseline(db))
	var note string
	require.NoError(t, db.Raw(`SELECT note FROM app_schema_migrations`).Scan(&note).Error)
	require.Equal(t, "keep me", note)
}
