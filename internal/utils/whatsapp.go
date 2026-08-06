package utils

import "strings"

// NormalizeWhatsAppNumber converts various local formats into Indonesian E.164-like digits without plus sign.
func NormalizeWhatsAppNumber(input string) string {
	n := strings.TrimSpace(input)
	n = strings.ReplaceAll(n, " ", "")
	n = strings.ReplaceAll(n, "-", "")
	n = strings.ReplaceAll(n, "(", "")
	n = strings.ReplaceAll(n, ")", "")

	if strings.HasPrefix(n, "+") {
		n = strings.TrimPrefix(n, "+")
	}

	switch {
	case strings.HasPrefix(n, "62"):
		return n
	case strings.HasPrefix(n, "08"):
		return "62" + n[1:]
	case strings.HasPrefix(n, "8"):
		return "62" + n
	default:
		return n
	}
}
