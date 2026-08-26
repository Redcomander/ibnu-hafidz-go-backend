package handlers

import "strings"

// TeacherSummaryEntry is the normalized per-teacher aggregate used in attendance exports.
type TeacherSummaryEntry struct {
	ID         uint   `json:"id"`
	Name       string `json:"name"`
	Avatar     string `json:"avatar"`
	Hadir      int    `json:"hadir"`
	Izin       int    `json:"izin"`
	Sakit      int    `json:"sakit"`
	Alpha      int    `json:"alpha"`
	Substitute int    `json:"substitute"`
}

func ensureTeacherSummaryEntry(summary []TeacherSummaryEntry, teacherID uint, name, avatar string) ([]TeacherSummaryEntry, int) {
	for i, entry := range summary {
		if entry.ID == teacherID {
			if entry.Name == "" {
				summary[i].Name = name
			}
			if entry.Avatar == "" {
				summary[i].Avatar = avatar
			}
			return summary, i
		}
	}

	entry := TeacherSummaryEntry{ID: teacherID, Name: name, Avatar: avatar}
	summary = append(summary, entry)
	return summary, len(summary) - 1
}

func applySubstituteTeacherCounts(summary []TeacherSummaryEntry, teacherID uint, teacherName, avatar string, count int) []TeacherSummaryEntry {
	if count <= 0 {
		return summary
	}
	var idx int
	var found bool
	for i, entry := range summary {
		if entry.ID == teacherID {
			idx = i
			found = true
			break
		}
	}
	if !found {
		summary = append(summary, TeacherSummaryEntry{ID: teacherID, Name: teacherName, Avatar: avatar})
		idx = len(summary) - 1
	}
	summary[idx].Substitute += count
	if summary[idx].Name == "" {
		summary[idx].Name = teacherName
	}
	if summary[idx].Avatar == "" {
		summary[idx].Avatar = avatar
	}
	return summary
}

func applyOriginalTeacherStatus(summary []TeacherSummaryEntry, teacherID uint, teacherName, avatar, status string, count int) []TeacherSummaryEntry {
	if count <= 0 {
		return summary
	}
	status = strings.TrimSpace(strings.ToLower(status))
	if status == "" || status == "hadir" {
		return summary
	}
	var idx int
	var found bool
	for i, entry := range summary {
		if entry.ID == teacherID {
			idx = i
			found = true
			break
		}
	}
	if !found {
		summary = append(summary, TeacherSummaryEntry{ID: teacherID, Name: teacherName, Avatar: avatar})
		idx = len(summary) - 1
	}
	summary[idx].Name = teacherName
	summary[idx].Avatar = avatar
	switch status {
	case "izin":
		summary[idx].Izin += count
	case "sakit":
		summary[idx].Sakit += count
	case "alpha":
		summary[idx].Alpha += count
	}
	return summary
}
