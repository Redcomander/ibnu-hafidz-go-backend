package handlers

import (
	"strings"
	"testing"
)

func TestNormalizeStatus(t *testing.T) {
	cases := map[string]string{
		"hadir":         "hadir",
		" HADIR ":       "hadir",
		"izin":          "izin",
		"sakit":         "sakit",
		"alpha":         "alpha",
		"tidak hadir":   "alpha",
		"unknown value": "alpha",
	}

	for input, want := range cases {
		if got := normalizeStatus(input); got != want {
			t.Fatalf("normalizeStatus(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestResolveRevitalisasiJenis(t *testing.T) {
	cases := map[string]string{
		"/revitalisasi/tukang":               "sma",
		"/revitalisasi-smp/tukang":           "smp",
		"/revitalisasi/tukang?jenis=smp":     "smp",
		"/revitalisasi/tukang?jenis=unknown": "sma",
	}

	for input, want := range cases {
		path := input
		query := ""
		if idx := strings.Index(input, "?jenis="); idx >= 0 {
			path = input[:idx]
			query = input[idx+7:]
		}
		if got := resolveRevitalisasiJenis(path, query); got != want {
			t.Fatalf("resolveRevitalisasiJenis(%q, %q) = %q, want %q", path, query, got, want)
		}
	}
}

func TestIsAllowedImageExtension(t *testing.T) {
	allowed := []string{".jpg", ".jpeg", ".png", ".webp"}
	for _, ext := range allowed {
		if !isAllowedImageExtension(ext) {
			t.Fatalf("expected %q to be allowed", ext)
		}
	}

	if isAllowedImageExtension(".gif") {
		t.Fatal("gif should not be allowed")
	}
}
