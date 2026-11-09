package handlers

import "strings"

// normalizeTeacherDutyStatus accepts the historical name without rewriting old rows.
func normalizeTeacherDutyStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "dinas", "dinas luar":
		return "Dinas"
	default:
		return status
	}
}
