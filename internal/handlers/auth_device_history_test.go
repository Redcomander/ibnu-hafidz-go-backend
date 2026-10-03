package handlers

import "testing"

func TestBuildLoginDeviceLabel(t *testing.T) {
	t.Run("chrome on windows", func(t *testing.T) {
		ua := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36"
		want := "Chrome di Windows"
		if got := buildLoginDeviceLabel(ua); got != want {
			t.Fatalf("buildLoginDeviceLabel(%q) = %q, want %q", ua, got, want)
		}
	})

	t.Run("safari on ios", func(t *testing.T) {
		ua := "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1"
		want := "Safari di iPhone"
		if got := buildLoginDeviceLabel(ua); got != want {
			t.Fatalf("buildLoginDeviceLabel(%q) = %q, want %q", ua, got, want)
		}
	})
}
