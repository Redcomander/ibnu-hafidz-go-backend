package handlers

import "testing"

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
