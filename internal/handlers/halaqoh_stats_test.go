package handlers

import "testing"

func TestNormalizeHalaqohGenderFilter(t *testing.T) {
	cases := map[string]string{
		"banin":     "banin",
		"Banin":     "banin",
		"putra":     "banin",
		"L":         "banin",
		"lakilaki":  "banin",
		"banat":     "banat",
		"Putri":     "banat",
		"P":         "banat",
		"perempuan": "banat",
		"female":    "banat",
		"":          "",
		"unknown":   "",
	}

	for input, want := range cases {
		if got := normalizeHalaqohGenderFilter(input); got != want {
			t.Fatalf("normalizeHalaqohGenderFilter(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestStudentGenderFilterValues(t *testing.T) {
	if got := studentGenderFilterValues("banin"); len(got) == 0 {
		t.Fatal("studentGenderFilterValues('banin') should not be empty")
	}

	if got := studentGenderFilterValues("banat"); len(got) == 0 {
		t.Fatal("studentGenderFilterValues('banat') should not be empty")
	}

	if got := studentGenderFilterValues("unknown"); len(got) != 0 {
		t.Fatalf("studentGenderFilterValues('unknown') should be empty, got %#v", got)
	}
}
