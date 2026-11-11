package statushistory

import (
	"testing"
	"time"
)

func st(s string) *string { return &s }

func TestDayStartsFollowTimezone(t *testing.T) {
	sp, _ := time.LoadLocation("America/Sao_Paulo")
	now := time.Date(2026, 11, 11, 2, 0, 0, 0, time.UTC) // 23:00 on Nov 10 in São Paulo
	starts, end := DayStarts(now, 3, sp)
	if got := starts[0].Format("2006-01-02"); got != "2026-11-08" || starts[2].Format("2006-01-02") != "2026-11-10" {
		t.Fatalf("days = %v", starts)
	}
	if !end.Equal(time.Date(2026, 11, 11, 3, 0, 0, 0, time.UTC)) {
		t.Fatalf("end = %v", end)
	}
}

func TestDaily(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2026, 11, d, 0, 0, 0, 0, time.UTC) }
	now := day(4).Add(12 * time.Hour)
	starts, end := DayStarts(now, 4, time.UTC) // Nov 1..4, Nov 4 until noon
	changes := []Change{
		{Status: st("operational"), At: day(1).Add(12 * time.Hour)}, // history starts mid Nov 1
		{Status: st("major_outage"), At: day(2).Add(6 * time.Hour)},
		{Status: st("operational"), At: day(2).Add(7 * time.Hour)},     // 1 h major
		{Status: st("partial_outage"), At: day(3).Add(23 * time.Hour)}, // 1 h partial on Nov 3...
		{Status: st("maintenance"), At: day(4).Add(1 * time.Hour)},     // ...and 1 h on Nov 4
		{Status: nil, At: day(4).Add(2 * time.Hour)},                   // deactivated: no data after
	}
	days, total := Daily(changes, starts, end, now)

	if days[0].Status != "operational" || *days[0].UptimePercent != 100 {
		t.Fatalf("day 1: %+v", days[0])
	}
	if days[1].Status != "major_outage" || days[1].Durations.MajorOutage != 3600 || *days[1].UptimePercent != 95.83 {
		t.Fatalf("day 2: %+v (uptime %v)", days[1], *days[1].UptimePercent)
	}
	// 1 h partial counts 30%: 1 - 0.3/24 = 98.75%.
	if days[2].Status != "partial_outage" || days[2].Durations.PartialOutage != 3600 || *days[2].UptimePercent != 98.75 {
		t.Fatalf("day 3: %+v (uptime %v)", days[2], *days[2].UptimePercent)
	}
	// Nov 4: 1 h partial + 1 h maintenance tracked, then deactivated.
	if days[3].Status != "partial_outage" || days[3].Durations.Maintenance != 3600 || *days[3].UptimePercent != 85 {
		t.Fatalf("day 4: %+v (uptime %v)", days[3], *days[3].UptimePercent)
	}
	// Tracked: 12 h + 24 h + 24 h + 2 h = 62 h; downtime 1 h + 0.6 h.
	want := 97.41
	if total == nil || *total != want {
		t.Fatalf("total = %v, want %v", total, want)
	}
}

func TestDailyNoHistory(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	starts, end := DayStarts(now, 30, time.UTC)
	days, total := Daily(nil, starts, end, now)
	if total != nil || len(days) != 30 || days[29].Status != NoData || days[29].UptimePercent != nil {
		t.Fatalf("no history: total=%v last=%+v", total, days[29])
	}
}

func TestDailyStateBeforeRange(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	starts, end := DayStarts(now, 2, time.UTC)
	// A change long before the range defines the status of the whole range.
	days, total := Daily([]Change{{Status: st("degraded_performance"), At: now.AddDate(0, -6, 0)}}, starts, end, now)
	if days[0].Status != "degraded_performance" || *total != 100 {
		t.Fatalf("days=%+v total=%v", days, *total)
	}
}
