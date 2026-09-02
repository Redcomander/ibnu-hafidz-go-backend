package handlers

import (
	"strconv"
	"strings"
)

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

func substituteSessionCount(startTime, endTime string) int {
	start := normalizeSubstituteTime(startTime)
	end := normalizeSubstituteTime(endTime)
	if start == "" || end == "" {
		return 1
	}

	startMinutes, err := timeStringToMinutes(start)
	if err != nil {
		return 1
	}
	endMinutes, err := timeStringToMinutes(end)
	if err != nil || endMinutes <= startMinutes {
		return 1
	}

	if startMinutes < 10*60 && endMinutes >= 9*60+30 {
		return 2
	}
	if startMinutes >= 10*60 && endMinutes >= 11*60+30 {
		return 2
	}
	return 1
}

func normalizeSubstituteTime(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed == "-" {
		return ""
	}
	if strings.Contains(trimmed, "T") {
		trimmed = strings.SplitN(trimmed, "T", 2)[1]
	}
	if len(trimmed) > 5 {
		trimmed = trimmed[:5]
	}
	return trimmed
}

func timeStringToMinutes(value string) (int, error) {
	parts := strings.SplitN(value, ":", 2)
	if len(parts) != 2 {
		return 0, strconv.ErrSyntax
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, err
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute >= 60 {
		return 0, err
	}
	return hour*60 + minute, nil
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
