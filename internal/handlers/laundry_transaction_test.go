package handlers

import (
	"encoding/json"
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

func newLaundryTransactionTestDB(t *testing.T) *gorm.DB {
	t.Helper()
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

	return db
}

func TestLaundryTransactionListSearchByStudentName(t *testing.T) {
	db := newLaundryTransactionTestDB(t)
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

func TestLaundryTransactionVendorSimilarityAndAttribution(t *testing.T) {
	db := newLaundryTransactionTestDB(t)
	if err := db.Exec(`
 UPDATE laundry_transactions SET tanggal = '2026-11-16 08:00:00';
 INSERT INTO users (id,name) VALUES (9,'Petugas Laundry');
 INSERT INTO students(id,nama_lengkap) VALUES (2,'Ahmad Fajar');
 INSERT INTO laundry_vendors(id,name,gender_type) VALUES (2,'Vendor B','banat');
 INSERT INTO laundry_accounts(id,student_id,vendor_id,nomor_laundry,active) VALUES (2,1,2,'LA-002',1),(3,2,2,'LA-003',1);
 INSERT INTO laundry_transactions(id,laundry_account_id,vendor_id,tanggal,berat_kg,status) VALUES
 (2,2,2,'2026-11-16 09:00:00',1,'picked_up'),
 (3,3,2,'2026-11-16 10:00:00',1,'pending'),
 (4,1,NULL,'2026-11-17 08:00:00',1,'pending'),
 (5,1,1,'2026-11-16 10:00:00',1,'pending');
 UPDATE laundry_transactions SET deleted_at = '2026-11-16 12:00:00' WHERE id=5;
 CREATE TABLE laundry_transaction_inputs (transaction_id BIGINT PRIMARY KEY, actor_id BIGINT NOT NULL, actor_name TEXT NOT NULL, created_at DATETIME NOT NULL);
 INSERT INTO laundry_transaction_inputs VALUES (1,9,'Petugas Laundry','2026-11-16 08:00:00');
 `).Error; err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	app.Get("/transactions", NewLaundryTransactionHandler(db).List)
	for _, tc := range []struct {
		query string
		total int
		count int64
		actor string
	}{
		{"&vendor_id=1&status=pending&search=LA-001&per_page=1", 2, 2, "Petugas Laundry"},
		{"&vendor_id=2&search=LA-002", 1, 2, ""},
		{"&vendor_id=2&search=LA-003", 1, 1, ""},
		{"&vendor_id=999", 0, 0, ""},
	} {
		t.Run(tc.query, func(t *testing.T) {
			response, err := app.Test(httptest.NewRequest("GET", "/transactions?date_from=2026-11-16&date_to=2026-11-17"+tc.query, nil))
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			var result struct {
				Total int                      `json:"total"`
				Data  []laundryTransactionView `json:"data"`
			}
			if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != 200 || result.Total != tc.total {
				t.Fatalf("status %d, total %d", response.StatusCode, result.Total)
			}
			if tc.total > 0 {
				// Search isolates the requested account; sorting selects transaction 1 before the legacy row.
				for _, row := range result.Data {
					if row.ID == 4 {
						if row.SameDayCount != 1 {
							t.Fatal("different date highlighted")
						}
						continue
					}
					if row.SameDayCount != tc.count {
						t.Fatalf("count=%d want=%d", row.SameDayCount, tc.count)
					}
					if tc.actor != "" && (row.InputBy == nil || row.InputBy.Name != tc.actor) {
						t.Fatalf("input_by=%+v", row.InputBy)
					}
				}
			}
		})
	}
}

func TestLaundryTransactionCreateRecordsAuthenticatedActorAtomically(t *testing.T) {
	db := newLaundryTransactionTestDB(t)
	for _, sql := range []string{
		`ALTER TABLE laundry_accounts ADD COLUMN total_kg REAL DEFAULT 0`,
		`ALTER TABLE laundry_accounts ADD COLUMN total_transactions INTEGER DEFAULT 0`,
		`ALTER TABLE laundry_accounts ADD COLUMN blocked BOOLEAN DEFAULT FALSE`,
		`INSERT INTO users(id,name) VALUES (9,'Petugas asli')`,
		`CREATE TABLE laundry_transaction_inputs (transaction_id BIGINT PRIMARY KEY, actor_id BIGINT NOT NULL, actor_name TEXT NOT NULL, created_at DATETIME NOT NULL)`,
	} {
		if err := db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	handler := NewLaundryTransactionHandler(db)
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error { c.Locals("userID", uint(9)); return c.Next() })
	app.Post("/transactions", handler.Create)
	request := httptest.NewRequest("POST", "/transactions", strings.NewReader(`{"laundry_account_id":1,"tanggal":"2026-11-16","berat_kg":2,"harga_per_kg":5000,"actor_id":123,"actor_name":"Palsu"}`))
	request.Header.Set("Content-Type", "application/json")
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 201 {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("status %d: %s", response.StatusCode, body)
	}
	var input struct {
		ActorID   uint
		ActorName string
	}
	if err := db.Table("laundry_transaction_inputs").First(&input).Error; err != nil {
		t.Fatal(err)
	}
	if input.ActorID != 9 || input.ActorName != "Petugas asli" {
		t.Fatalf("incorrect actor %+v", input)
	}
	// A failed attribution write must roll back the transaction and account totals.
	if err := db.Exec(`CREATE TRIGGER fail_input BEFORE INSERT ON laundry_transaction_inputs BEGIN SELECT RAISE(ABORT,'test failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	request = httptest.NewRequest("POST", "/transactions", strings.NewReader(`{"laundry_account_id":1,"tanggal":"2026-11-16","berat_kg":3,"harga_per_kg":5000}`))
	request.Header.Set("Content-Type", "application/json")
	response, err = app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 500 {
		t.Fatalf("expected rollback, status=%d", response.StatusCode)
	}
	var count int64
	db.Table("laundry_transactions").Count(&count)
	if count != 2 {
		t.Fatalf("partial transaction saved: %d", count)
	}
	var total float64
	db.Table("laundry_accounts").Where("id=1").Pluck("total_kg", &total)
	if total != 2 {
		t.Fatalf("incorrect account totals %v", total)
	}
}
