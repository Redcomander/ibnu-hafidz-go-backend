package middleware

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"reflect"
	"runtime"
	"strings"
	"time"

	drivermysql "github.com/go-sql-driver/mysql"
	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/diagnostics"
)

// Diagnostics deliberately captures no bodies, query strings, headers, cookies,
// raw error messages, SQL, IP addresses or panic values. Stack frames have no locals.
func Diagnostics(store *diagnostics.Store) fiber.Handler {
	return func(c *fiber.Ctx) (returned error) {
		id := diagnostics.NewID()
		c.Set("X-Request-ID", id)
		c.Locals("requestID", id)
		started := time.Now()
		var frames []string
		panicked := false
		defer func() {
			c.Set("X-Request-ID", id)
			if value := recover(); value != nil {
				panicked = true
				returned = fiber.NewError(500, "Internal Server Error")
				pcs := make([]uintptr, 24)
				count := runtime.Callers(3, pcs)
				stack := runtime.CallersFrames(pcs[:count])
				for {
					frame, more := stack.Next()
					frames = append(frames, fmt.Sprintf("%s (%s:%d)", frame.Function, frame.File, frame.Line))
					if !more {
						break
					}
				}
			}
			status := c.Response().StatusCode()
			if returned != nil {
				status = 500
				if err, ok := returned.(*fiber.Error); ok {
					status = err.Code
				}
			}
			duration := time.Since(started).Milliseconds()
			if status >= 500 && returned == nil {
				var body struct {
					Error string `json:"error"`
				}
				_ = json.Unmarshal(c.Response().Body(), &body)
				// Maintenance has its own deliberate public message. Other direct 5xx
				// responses get the same safe envelope as errors returned to ErrorHandler.
				if status != 503 || body.Error != "maintenance" {
					_ = c.Status(status).JSON(fiber.Map{"error": "server_error", "message": "Internal Server Error", "request_id": id})
				}
			}
			route := c.Route().Path
			// Only registered route templates are safe; never fallback to the raw URL.
			if route == "" || route == "/" {
				route = "(unmatched)"
			}
			if strings.HasPrefix(route, "/api/system-logs") || route == "/health" {
				return
			}
			if status < 500 && !(store.DebugEnabled() && (status >= 400 || duration >= 1000)) {
				return
			}
			severity, kind, summary := "error", "http_5xx", "Server request failed; inspect handler and infrastructure using this request ID."
			if status >= 500 && returned != nil {
				kind, summary = diagnosticErrorClass(returned)
			}
			if status < 500 {
				severity, kind, summary = "warning", "http_4xx", "Request rejected during debug capture."
			}
			if status < 400 {
				severity, kind, summary = "warning", "slow_request", "Request exceeded 1000 ms during debug capture."
			}
			if panicked {
				kind, summary = "panic", "Recovered a server panic; inspect the captured code frames."
			}
			handler := ""
			handlers := c.Route().Handlers
			if len(handlers) > 0 {
				if fn := runtime.FuncForPC(reflect.ValueOf(handlers[len(handlers)-1]).Pointer()); fn != nil {
					handler = fn.Name()
				}
			}
			sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%d|%s|%s", c.Method(), route, status, kind, handler)))
			activityID, _ := c.Locals("activityLogID").(uint)
			event := diagnostics.Event{ID: id, RequestID: id, CreatedAt: time.Now().UTC(), Method: c.Method(), Route: route,
				Handler: handler, Status: status, DurationMS: duration, Kind: kind, Severity: severity, Summary: summary,
				Fingerprint: hex.EncodeToString(sum[:8]), ActivityLogID: activityID, Frames: frames}
			if err := store.Add(event); err != nil {
				log.Printf("diagnostic_storage_unavailable request_id=%s status=%d", id, status)
			}
			log.Printf("request_failure request_id=%s status=%d fingerprint=%s", id, status, event.Fingerprint)
		}()
		return c.Next()
	}
}

// Classify using types/codes, never arbitrary exception text containing SQL or PII.
func diagnosticErrorClass(err error) (string, string) {
	var databaseError *drivermysql.MySQLError
	if errors.As(err, &databaseError) {
		summary := "Database request failed; inspect schema and database health."
		switch databaseError.Number {
		case 1062:
			summary = "Database rejected a duplicate unique value."
		case 1054:
			summary = "Database column is missing or incompatible with this release."
		case 1146:
			summary = "Required database table is missing."
		case 1040:
			summary = "Database connection limit reached."
		case 1205:
			summary = "Database lock wait timed out."
		case 1213:
			summary = "Database deadlock detected."
		case 1451, 1452:
			summary = "Database relationship constraint rejected the operation."
		}
		return fmt.Sprintf("mysql_%d", databaseError.Number), summary
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout", "Request or dependency timed out."
	}
	if errors.Is(err, sql.ErrConnDone) {
		return "database_connection", "Database connection is no longer available."
	}
	return "http_5xx", "Server request failed; inspect handler and infrastructure using this request ID."
}
