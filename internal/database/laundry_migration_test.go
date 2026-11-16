package database

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestLaundryInputsSchemaAndHistoryPreserveLegacyData(t *testing.T) {
	db := migrationTestDB(t)
	seedLegacyDatabase(t, db)
	require.NoError(t, RegisterBaseline(db))
	for _, table := range []string{"mobile_app_releases", "mobile_release_policies", "mobile_release_audits"} {
		require.NoError(t, db.Exec("CREATE TABLE "+table+" (id INTEGER PRIMARY KEY)").Error)
	}
	require.NoError(t, db.Create(&migrationRecord{Version: 2, Name: "mobile_releases", Checksum: mobileReleaseChecksum(), SchemaSnapshot: "{}", AppliedAt: time.Now()}).Error)
	before := businessSnapshot(t, db)
	// The additive SQL is portable; the production advisory-lock runner is tested separately on MySQL.
	require.NoError(t, db.Exec(laundryInputsSQL()).Error)
	require.NoError(t, db.Create(&migrationRecord{Version: 3, Name: "laundry_transaction_inputs", Checksum: laundryInputsChecksum(), SchemaSnapshot: "{}", AppliedAt: time.Now()}).Error)
	require.NoError(t, CheckMigrations(db))
	after := businessSnapshot(t, db)
	for name, rows := range before {
		if name != "definitions" {
			require.Equal(t, rows, after[name], "legacy table changed: %s", name)
		}
	}
	definitions := after["definitions"].([]tableDefinition)
	legacy := []tableDefinition{}
	for _, definition := range definitions {
		if definition.Name != "laundry_transaction_inputs" {
			legacy = append(legacy, definition)
		}
	}
	require.Equal(t, before["definitions"], legacy, "old definitions must not change")
	require.NoError(t, db.Model(&migrationRecord{}).Where("version=3").Update("dirty", true).Error)
	require.Error(t, CheckMigrations(db))
	require.NoError(t, db.Model(&migrationRecord{}).Where("version=3").Update("dirty", false).Error)
	require.NoError(t, db.Model(&migrationRecord{}).Where("version=3").Update("checksum", "changed").Error)
	require.Error(t, CheckMigrations(db))
}

func TestLaundryInputsOperatorRunnerRejectsUnsupportedDatabaseWithoutWrites(t *testing.T) {
	db := migrationTestDB(t)
	seedLegacyDatabase(t, db)
	before := businessSnapshot(t, db)
	require.Error(t, ApplyLaundryInputs(db))
	require.Equal(t, before, businessSnapshot(t, db))
}
