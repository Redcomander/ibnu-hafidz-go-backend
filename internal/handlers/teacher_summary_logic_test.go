package handlers

import (
	"strings"
	"testing"
)

func TestApplyTeacherSubstituteCountsDoesNotInflateHadir(t *testing.T) {
	summary := []TeacherSummaryEntry{{ID: 1, Name: "Guru A", Hadir: 2}}

	summary = applySubstituteTeacherCounts(summary, 1, "Guru A", "", 3)
	if summary[0].Hadir != 2 {
		t.Fatalf("expected Hadir to stay at 2, got %d", summary[0].Hadir)
	}
	if summary[0].Substitute != 3 {
		t.Fatalf("expected Substitute to be 3, got %d", summary[0].Substitute)
	}
}

func TestApplyOriginalTeacherStatusCountsOnlyAbsenceStatuses(t *testing.T) {
	summary := []TeacherSummaryEntry{{ID: 2, Name: "Guru B", Hadir: 1}}

	summary = applyOriginalTeacherStatus(summary, 2, "Guru B", "", "Hadir", 1)
	if summary[0].Hadir != 1 {
		t.Fatalf("expected Hadir to remain 1, got %d", summary[0].Hadir)
	}

	summary = applyOriginalTeacherStatus(summary, 2, "Guru B", "", "Izin", 1)
	if summary[0].Izin != 1 {
		t.Fatalf("expected Izin to be 1, got %d", summary[0].Izin)
	}
}

func TestSessionCountFromTimeRangeCountsAcrossMorningBreak(t *testing.T) {
	cases := map[string]int{
		"08:00-09:30": 2,
		"08:15-09:30": 2,
		"10:00-11:30": 2,
		"08:00-11:30": 2,
		"08:15-10:00": 2,
		"09:30-10:15": 2,
		"08:00-08:30": 1,
		"":            1,
	}

	for rangeText, want := range cases {
		start, end, ok := strings.Cut(rangeText, "-")
		if !ok {
			if got := substituteSessionCount("", ""); got != want {
				t.Fatalf("expected empty range count to be %d, got %d", want, got)
			}
			continue
		}
		if got := substituteSessionCount(start, end); got != want {
			t.Fatalf("range %q => expected %d, got %d", rangeText, want, got)
		}
	}
}

func TestCountFormalSessionUnitsUsesSessionBasedTotals(t *testing.T) {
	rows := []formalSessionCountRow{
		{Status: "Izin", StartTime: "08:00", EndTime: "09:30"},
		{Status: "Sakit", StartTime: "10:00", EndTime: "11:30"},
		{Status: "Alpha", StartTime: "08:15", EndTime: "08:45"},
		{Status: "Hadir", StartTime: "08:00", EndTime: "09:30"},
	}

	got := countFormalSessionUnits(rows)
	if got["Izin"] != 2 {
		t.Fatalf("expected Izin session total to be 2, got %d", got["Izin"])
	}
	if got["Sakit"] != 2 {
		t.Fatalf("expected Sakit session total to be 2, got %d", got["Sakit"])
	}
	if got["Alpha"] != 1 {
		t.Fatalf("expected Alpha session total to be 1, got %d", got["Alpha"])
	}
	if got["Hadir"] != 0 {
		t.Fatalf("expected Hadir to be excluded from session totals, got %d", got["Hadir"])
	}
}

func TestFormalStatusAggregationDoesNotOverwriteMultiRowTeacherTotals(t *testing.T) {
	rows := []formalSessionCountRow{
		{Status: "Izin", StartTime: "08:00", EndTime: "09:30"},
		{Status: "Sakit", StartTime: "10:00", EndTime: "11:30"},
		{Status: "Izin", StartTime: "10:00", EndTime: "11:30"},
	}

	got := countFormalSessionUnits(rows)
	if got["Izin"] != 4 {
		t.Fatalf("expected aggregated Izin total to be 4, got %d", got["Izin"])
	}
	if got["Sakit"] != 2 {
		t.Fatalf("expected aggregated Sakit total to be 2, got %d", got["Sakit"])
	}
}
