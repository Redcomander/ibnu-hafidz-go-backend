package database

import (
	"os"
	"strings"
	"testing"

	drivermysql "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Never loads .env or uses the application's connection configuration. The DSN
// must select an EMPTY, explicitly named disposable database, otherwise no writes
// occur. Run this in CI's isolated MySQL service or a disposable local instance.
func TestMySQLBaselinePreservesExistingData(t *testing.T) {
	dsn := os.Getenv("IBNU_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set IBNU_MIGRATION_TEST_DSN to an empty disposable MySQL database")
	}
	cfg, err := drivermysql.ParseDSN(dsn)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(cfg.DBName, "ibnu_migration_test_"), "test database name must start with ibnu_migration_test_")
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })
	var tables int64
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE()").Scan(&tables).Error)
	require.Zero(t, tables, "refusing any writes to a nonempty database; provision a new disposable database")
	for _, statement := range []string{
		`CREATE TABLE users (id BIGINT PRIMARY KEY, name VARCHAR(255), password VARCHAR(255))`,
		`INSERT INTO users VALUES (41, 'Original Teacher', 'untouched-hash')`,
		`CREATE TABLE students (id BIGINT PRIMARY KEY, kelas_id BIGINT NULL, notes TEXT)`,
		`INSERT INTO students VALUES (9, NULL, 'Keep membership null')`,
		`CREATE TABLE kelas_siswa (student_id BIGINT, kelas_id BIGINT)`,
		`INSERT INTO kelas_siswa VALUES (9, 3)`,
		`CREATE TABLE lessons (id BIGINT, name VARCHAR(255))`,
		`INSERT INTO lessons VALUES (1, 'Preserve legacy column')`,
		`CREATE TABLE teacher_attendances (id BIGINT, status ENUM('Hadir','Dinas Luar'), date DATETIME, payload BLOB)`,
		`INSERT INTO teacher_attendances VALUES (1, 'Dinas Luar', '2026-11-09 08:12:34', X'0001FF')`,
		`CREATE TABLE absensi_ekstra_sessions (id BIGINT, created_by_id BIGINT, waktu_mulai DATETIME)`,
		`INSERT INTO absensi_ekstra_sessions VALUES (5, 41, '2026-11-09 13:00:00')`,
		`CREATE TABLE permissions (id BIGINT, name VARCHAR(255))`,
		`INSERT INTO permissions VALUES (1, 'custom.permission')`,
	} {
		require.NoError(t, db.Exec(statement).Error)
	}

	snapshot := func() map[string]any {
		definitions, err := captureSchema(db)
		require.NoError(t, err)
		state := map[string]any{"definitions": definitions}
		for _, definition := range definitions {
			// Each fixture table has one record, so row order cannot affect comparison.
			rows, err := db.Raw("SELECT * FROM `" + definition.Name + "`").Rows()
			require.NoError(t, err)
			columns, err := rows.Columns()
			require.NoError(t, err)
			var records [][]any
			for rows.Next() {
				values := make([]any, len(columns))
				targets := make([]any, len(columns))
				for i := range values {
					targets[i] = &values[i]
				}
				require.NoError(t, rows.Scan(targets...))
				for i, value := range values {
					if bytes, ok := value.([]byte); ok {
						values[i] = append([]byte(nil), bytes...)
					}
				}
				records = append(records, values)
			}
			require.NoError(t, rows.Err())
			require.NoError(t, rows.Close())
			state[definition.Name] = records
		}
		return state
	}
	before := snapshot()
	require.Error(t, CheckMigrations(db))
	require.NoError(t, RegisterBaseline(db))
	require.NoError(t, CheckMigrations(db))
	require.NoError(t, RegisterBaseline(db))
	require.Equal(t, before, snapshot(), "all original MySQL definitions and values must remain identical")
	var count int64
	require.NoError(t, db.Model(&migrationRecord{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
	// Do not drop anything; discard the entire isolated database/container externally.
}
