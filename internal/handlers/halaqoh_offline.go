package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

// Offline permits are purpose-bound HMAC messages, never API access JWTs.
// A permit expires at Jakarta midnight three days after its attendance date.
type halaqohOfflinePermit struct {
	User        uint   `json:"u"`
	Kind        string `json:"k"`
	Date        string `json:"d"`
	Session     string `json:"s"`
	Assignments []uint `json:"a"`
	Expires     int64  `json:"e"`
}

func signHalaqohPermit(p halaqohOfflinePermit, secret string) string {
	data, _ := json.Marshal(p)
	encoded := base64.RawURLEncoding.EncodeToString(data)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("halaqoh-offline-v1:" + encoded))
	return encoded + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func halaqohOfflinePermits(c *fiber.Ctx, user uint, kind, date string, assignments []uint) map[string]string {
	permits := map[string]string{}
	secret, _ := c.Locals("jwtSecret").(string)
	if secret == "" || date != todayString() {
		return permits
	}
	day := parseDate(date)
	for session := range sessionTimes {
		permits[session] = signHalaqohPermit(halaqohOfflinePermit{User: user, Kind: kind, Date: date, Session: session, Assignments: assignments, Expires: day.Add(72 * time.Hour).Unix()}, secret)
	}
	return permits
}

func validHalaqohOfflinePermit(c *fiber.Ctx, user uint, kind, date, session, raw, captured string, ids []uint) bool {
	secret, _ := c.Locals("jwtSecret").(string)
	parts := strings.Split(raw, ".")
	if secret == "" || len(parts) != 2 {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	var p halaqohOfflinePermit
	if json.Unmarshal(decoded, &p) != nil || p.User != user || p.Kind != kind || p.Date != date || p.Session != session || p.Expires <= time.Now().Unix() {
		return false
	}
	if !hmac.Equal([]byte(raw), []byte(signHalaqohPermit(p, secret))) {
		return false
	}
	allowed := map[uint]bool{}
	for _, id := range p.Assignments {
		allowed[id] = true
	}
	if len(ids) == 0 {
		return false
	}
	for _, id := range ids {
		if !allowed[id] {
			return false
		}
	}
	at, err := time.Parse(time.RFC3339Nano, captured)
	if err != nil || at.After(time.Now().Add(5*time.Minute)) {
		return false
	}
	loc := jakartaLocation()
	at = at.In(loc)
	if at.Format("2006-01-02") != date {
		return false
	}
	times, ok := sessionTimes[session]
	if !ok {
		return false
	}
	start, err := time.ParseInLocation("2006-01-02 15:04", date+" "+times[0], loc)
	if err != nil {
		return false
	}
	end, err := time.ParseInLocation("2006-01-02 15:04", date+" "+times[1], loc)
	return err == nil && !at.Before(start) && !at.After(end)
}
