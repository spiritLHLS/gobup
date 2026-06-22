package models

import "time"

// LiveSessionDayRange returns the local calendar-day range for a recording.
// Same-title fallback matching must stay inside this range so daily streams are
// not merged just because the live title is reused.
func LiveSessionDayRange(start time.Time) (time.Time, time.Time, bool) {
	if start.IsZero() {
		return time.Time{}, time.Time{}, false
	}
	year, month, day := start.Date()
	dayStart := time.Date(year, month, day, 0, 0, 0, 0, start.Location())
	return dayStart, dayStart.AddDate(0, 0, 1), true
}

func LiveSessionDayKey(start time.Time) string {
	dayStart, _, ok := LiveSessionDayRange(start)
	if !ok {
		return ""
	}
	return dayStart.Format("2006-01-02")
}
