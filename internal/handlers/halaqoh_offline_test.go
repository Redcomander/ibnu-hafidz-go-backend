package handlers

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestHalaqohTeacherNeverAcknowledgesFailedDatabaseWrite(t *testing.T) {
	for _, operation := range []string{"create", "update", "query"} {
		t.Run(operation, func(t *testing.T) {
			db := securityDB(t)
			db.Config.IgnoreRelationshipsWhenMigrating = true
			require.NoError(t, db.AutoMigrate(&models.HalaqohAssignment{}, &models.HalaqohTeacherAttendance{}))
			require.NoError(t, db.Exec(`CREATE TABLE halaqoh_substitute_logs (id INTEGER PRIMARY KEY, halaqoh_assignment_id INTEGER, date DATE, session TEXT, is_active BOOLEAN)`).Error)
			require.NoError(t, db.Create(&models.HalaqohAssignment{UserID: 1, StudentID: 2, Active: true}).Error)
			if operation == "update" {
				require.NoError(t, db.Exec(`INSERT INTO halaqoh_teacher_attendances (user_id, date, session, status) VALUES (1, '2026-11-01', 'Shubuh', 'Hadir')`).Error)
			}
			fail := func(tx *gorm.DB) {
				if tx.Statement.Table == "halaqoh_teacher_attendances" {
					tx.AddError(errors.New("simulated database failure"))
				}
			}
			switch operation {
			case "create":
				require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:fail", fail))
			case "update":
				require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:fail", fail))
			case "query":
				require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:fail", fail))
			}
			app := fiber.New()
			app.Use(func(c *fiber.Ctx) error {
				c.Locals("user", &models.User{ID: 3, Roles: []models.Role{{Name: "admin"}}})
				return c.Next()
			})
			app.Post("/:assignment_id/:session", NewHalaqohHandler(db).SubmitTeacherAttendance)
			req := httptest.NewRequest("POST", "/1/Shubuh", strings.NewReader("status=Hadir&date=2026-11-01"))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			res, err := app.Test(req)
			require.NoError(t, err)
			defer res.Body.Close()
			require.Equal(t, 500, res.StatusCode, "mobile queue must retain this submission")
		})
	}
}

func TestHalaqohOfflinePermitScopeAndCaptureWindow(t *testing.T) {
	day := time.Now().In(jakartaLocation()).AddDate(0, 0, -1)
	date := day.Format("2006-01-02")
	captured := date + "T05:00:00+07:00"
	p := halaqohOfflinePermit{User: 7, Kind: "students", Date: date, Session: "Shubuh", Assignments: []uint{12, 13}, Expires: time.Now().Add(time.Hour).Unix()}
	permit := signHalaqohPermit(p, "test-secret")
	tests := []struct {
		name, raw, at, kind, date, session string
		user                               uint
		ids                                []uint
		want                               bool
	}{
		{"delayed replay", permit, captured, "students", date, "Shubuh", 7, []uint{12, 13}, true},
		{"another user", permit, captured, "students", date, "Shubuh", 8, []uint{12}, false},
		{"another assignment", permit, captured, "students", date, "Shubuh", 7, []uint{99}, false},
		{"wrong form", permit, captured, "teachers", date, "Shubuh", 7, []uint{12}, false},
		{"wrong date", permit, captured, "students", "2000-01-01", "Shubuh", 7, []uint{12}, false},
		{"wrong session", permit, captured, "students", date, "Isya", 7, []uint{12}, false},
		{"outside capture window", permit, date + "T12:00:00+07:00", "students", date, "Shubuh", 7, []uint{12}, false},
		{"missing capture", permit, "", "students", date, "Shubuh", 7, []uint{12}, false},
		{"tampered signature", permit + "x", captured, "students", date, "Shubuh", 7, []uint{12}, false},
		{"missing permit", "", captured, "students", date, "Shubuh", 7, []uint{12}, false},
		{"empty attendance", permit, captured, "students", date, "Shubuh", 7, nil, false},
	}
	expired := p
	expired.Expires = time.Now().Add(-time.Second).Unix()
	tests = append(tests, struct {
		name, raw, at, kind, date, session string
		user                               uint
		ids                                []uint
		want                               bool
	}{"expired", signHalaqohPermit(expired, "test-secret"), captured, "students", date, "Shubuh", 7, []uint{12}, false})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New()
			app.Get("/", func(c *fiber.Ctx) error {
				c.Locals("jwtSecret", "test-secret")
				got := validHalaqohOfflinePermit(c, tt.user, tt.kind, tt.date, tt.session, tt.raw, tt.at, tt.ids)
				if got != tt.want {
					t.Errorf("accepted=%v want %v", got, tt.want)
				}
				return c.SendStatus(200)
			})
			res, err := app.Test(httptest.NewRequest("GET", "/", nil))
			if err != nil {
				t.Fatal(err)
			}
			res.Body.Close()
		})
	}
}

func TestHalaqohOfflinePermitsOnlyIssueForToday(t *testing.T) {
	app := fiber.New()
	app.Get("/", func(c *fiber.Ctx) error {
		c.Locals("jwtSecret", "test-secret")
		if got := halaqohOfflinePermits(c, 7, "teachers", todayString(), []uint{12}); len(got) != 3 {
			t.Errorf("got %d session permits", len(got))
		}
		if got := halaqohOfflinePermits(c, 7, "teachers", "2000-01-01", []uint{12}); len(got) != 0 {
			t.Error("must not issue historical capture permits")
		}
		c.Locals("jwtSecret", "")
		if got := halaqohOfflinePermits(c, 7, "teachers", todayString(), []uint{12}); len(got) != 0 {
			t.Error("must require configured signing key")
		}
		return c.SendStatus(200)
	})
	res, err := app.Test(httptest.NewRequest("GET", "/", nil))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
}
