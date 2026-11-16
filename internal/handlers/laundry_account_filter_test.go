package handlers

import (
	"encoding/json"
	"github.com/glebarez/sqlite"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"net/http/httptest"
	"testing"
)

func TestLaundryAccountsVendorFilter(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	schema := `
 CREATE TABLE laundry_vendors (id INTEGER PRIMARY KEY, name TEXT, gender_type TEXT, deleted_at DATETIME);
 CREATE TABLE students (id INTEGER PRIMARY KEY, nama_lengkap TEXT, deleted_at DATETIME);
 CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, deleted_at DATETIME);
 CREATE TABLE laundry_accounts (id INTEGER PRIMARY KEY, vendor_id INTEGER, student_id INTEGER, user_id INTEGER, nomor_laundry TEXT, active BOOLEAN, created_at DATETIME, updated_at DATETIME, deleted_at DATETIME);
 CREATE TABLE laundry_transactions (id INTEGER PRIMARY KEY, laundry_account_id INTEGER, tanggal DATETIME, berat_kg REAL, deleted_at DATETIME);
 INSERT INTO laundry_vendors(id,name,gender_type) VALUES (1,'Vendor A','banin'),(2,'Vendor B','banat');
 INSERT INTO laundry_accounts(id,vendor_id,nomor_laundry,active) VALUES (1,1,'A-001',1),(2,1,'A-002',1),(3,2,'B-001',1),(4,2,'B-inactive',0);
 `
	if err = db.Exec(schema).Error; err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	app.Get("/accounts", NewLaundryAccountHandler(db).List)
	for _, tc := range []struct {
		query  string
		total  int
		count  int
		vendor uint
	}{
		{"?vendor_id=1&per_page=1&page=2", 2, 1, 1},
		{"?vendor_id=2", 1, 1, 2},
		{"?vendor_id=1&gender_type=banat", 0, 0, 0},
		{"?vendor_id=1&search=A-002", 1, 1, 1},
		{"?vendor_id=all", 3, 3, 0},
		{"?vendor_id=999", 0, 0, 0},
	} {
		t.Run(tc.query, func(t *testing.T) {
			response, err := app.Test(httptest.NewRequest("GET", "/accounts"+tc.query, nil))
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != 200 {
				t.Fatalf("status %d", response.StatusCode)
			}
			var result struct {
				Total int `json:"total"`
				Data  []struct {
					VendorID uint `json:"vendor_id"`
				} `json:"data"`
			}
			if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
				t.Fatal(err)
			}
			if result.Total != tc.total || len(result.Data) != tc.count {
				t.Fatalf("got total=%d count=%d", result.Total, len(result.Data))
			}
			for _, account := range result.Data {
				if tc.vendor > 0 && account.VendorID != tc.vendor {
					t.Fatalf("wrong vendor %d", account.VendorID)
				}
			}
		})
	}
}
