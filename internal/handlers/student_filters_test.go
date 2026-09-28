package handlers

import "testing"

func TestNormalizeStudentRelationStatusValues(t *testing.T) {
	cases := map[string][]string{
		"":                       {},
		"ada":                    {"ada"},
		"kosong":                 {"kosong"},
		"ada&kosong":             {"ada", "kosong"},
		"yes,none":               {"ada", "kosong"},
		"exists;tidak_ada":       {"ada", "kosong"},
		"unknown":                {},
		"ada|kosong|unknown|yes": {"ada", "kosong"},
		"tidak_ada|kosong|ada":   {"ada", "kosong"},
		"punya,tidak_punya":      {"ada", "kosong"},
		"has,missing":            {"ada", "kosong"},
		"ada,ada,kosong,kosong":  {"ada", "kosong"},
		"  ada  ,  kosong  ":     {"ada", "kosong"},
	}

	for raw, want := range cases {
		got := normalizeStudentRelationStatusValues(raw)
		if len(got) != len(want) {
			t.Fatalf("normalizeStudentRelationStatusValues(%q) = %#v, want %#v", raw, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("normalizeStudentRelationStatusValues(%q) = %#v, want %#v", raw, got, want)
			}
		}
	}
}

func TestHasStudentRelationStatus(t *testing.T) {
	if !hasStudentRelationStatus("ada", "ada") {
		t.Fatal("should treat ada as ada")
	}
	if !hasStudentRelationStatus("kosong", "kosong") {
		t.Fatal("should treat kosong as kosong")
	}
	if hasStudentRelationStatus("ada", "kosong") {
		t.Fatal("should not treat ada as kosong")
	}
	if !hasStudentRelationStatus("yes", "ada") {
		t.Fatal("should accept yes alias")
	}
	if !hasStudentRelationStatus("tidak_ada", "kosong") {
		t.Fatal("should accept tidak_ada alias")
	}
	if hasStudentRelationStatus("unknown", "ada") {
		t.Fatal("should reject unknown alias")
	}
}
