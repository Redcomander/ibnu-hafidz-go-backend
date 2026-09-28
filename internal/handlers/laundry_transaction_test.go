package handlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

func TestLaundryTransactionListSearchByStudentName(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite in-memory db: %v", err)
	}

	if err := db.Exec(`
		CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT,
			email TEXT,
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME
		);
		CREATE TABLE students (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			nama_lengkap TEXT NOT NULL,
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME
		);
		CREATE TABLE laundry_vendors (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT,
			gender_type TEXT,
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME
		);
		CREATE TABLE laundry_accounts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			student_id INTEGER,
			user_id INTEGER,
			vendor_id INTEGER NOT NULL,
			nomor_laundry TEXT,
			active BOOLEAN DEFAULT TRUE,
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME
		);
		CREATE TABLE laundry_transactions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			laundry_account_id INTEGER NOT NULL,
			vendor_id INTEGER,
			tanggal DATETIME,
			berat_kg REAL,
			harga_per_kg REAL,
			total_harga REAL,
			catatan TEXT,
			status TEXT,
			picked_up_at DATETIME,
			picked_up_by_id INTEGER,
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME
		);
	`).Error; err != nil {
		t.Fatalf("create minimal schema: %v", err)
	}

	if err := db.Exec(`INSERT INTO students (id, nama_lengkap, created_at, updated_at) VALUES (1, 'Ahmad Fajar', ?, ?)`, time.Now(), time.Now()).Error; err != nil {
		t.Fatalf("create student: %v", err)
	}

	if err := db.Exec(`INSERT INTO laundry_vendors (id, name, gender_type, created_at, updated_at) VALUES (1, 'Vendor A', 'banin', ?, ?)`, time.Now(), time.Now()).Error; err != nil {
		t.Fatalf("create vendor: %v", err)
	}

	if err := db.Exec(`INSERT INTO laundry_accounts (id, student_id, user_id, vendor_id, nomor_laundry, active, created_at, updated_at) VALUES (1, 1, NULL, 1, 'LA-001', 1, ?, ?)`, time.Now(), time.Now()).Error; err != nil {
		t.Fatalf("create account: %v", err)
	}

	if err := db.Exec(`INSERT INTO laundry_transactions (id, laundry_account_id, vendor_id, tanggal, berat_kg, harga_per_kg, total_harga, catatan, status, created_at, updated_at) VALUES (1, 1, 1, ?, 2.5, 2500, 6250, 'Cuci cepat', 'pending', ?, ?)`, time.Now(), time.Now(), time.Now()).Error; err != nil {
		t.Fatalf("create transaction: %v", err)
	}

	handler := NewLaundryTransactionHandler(db)
	app := fiber.New()
	app.Get("/laundry/transactions", handler.List)

	req := httptest.NewRequest(http.MethodGet, "/laundry/transactions?search=Ahmad", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app test: %v", err)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d with body: %s", resp.StatusCode, string(body))
	}

	if !strings.Contains(string(body), "Ahmad") {
		t.Fatalf("expected search result to include student name; got: %s", string(body))
	}
}
