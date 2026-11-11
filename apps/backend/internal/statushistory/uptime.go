package statushistory

import (
	"math"
	"time"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/statuspolicy"
)

// NoData marks a day (or the whole range) without any tracked time: before
// the component's history starts, or while it was deactivated.
const NoData = "no_data"

// Weights of each status in the downtime used for uptime percentages, as in
// Atlassian Statuspage: a major outage counts fully, a partial outage 30%;
// degraded performance and maintenance do not count as downtime.
var downtimeWeight = map[string]float64{
	statuspolicy.MajorOutage:   1,
	statuspolicy.PartialOutage: 0.3,
}

// Change is one point of a component's history: Status is in force from At
// until the next change; nil means not tracked (deactivated).
type Change struct {
	Status *string
	At     time.Time
}

// Durations are the seconds spent in each non-operational status.
type Durations struct {
	MajorOutage         int64 `json:"major_outage"`
	PartialOutage       int64 `json:"partial_outage"`
	DegradedPerformance int64 `json:"degraded_performance"`
	Maintenance         int64 `json:"maintenance"`
}

func (d *Durations) add(status string, secs int64) {
	switch status {
	case statuspolicy.MajorOutage:
		d.MajorOutage += secs
	case statuspolicy.PartialOutage:
		d.PartialOutage += secs
	case statuspolicy.DegradedPerformance:
		d.DegradedPerformance += secs
	case statuspolicy.Maintenance:
		d.Maintenance += secs
	}
}

// Day is the summary of one calendar day in the requested timezone.
type Day struct {
	Date string `json:"date"`
	// Status is the most severe status seen during the day, or NoData.
	Status        string    `json:"status"`
	UptimePercent *float64  `json:"uptime_percent"`
	Durations     Durations `json:"durations"`
	// Incidents that affected the component during the day.
	Incidents []IncidentRef `json:"incidents"`

	start, end time.Time
	tracked    int64
	downtime   float64
}

// IncidentRef is an incident shown on a day of the uptime bar.
type IncidentRef struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Impact string `json:"impact"`
}

// DayStarts returns the start of each of the n calendar days ending today
// (in loc), oldest first, plus the end of today.
func DayStarts(now time.Time, n int, loc *time.Location) ([]time.Time, time.Time) {
	local := now.In(loc)
	y, m, d := local.Date()
	starts := make([]time.Time, n)
	for i := range n {
		starts[i] = time.Date(y, m, d-(n-1-i), 0, 0, 0, 0, loc)
	}
	return starts, time.Date(y, m, d+1, 0, 0, 0, 0, loc)
}

// Daily summarises a component's history over the days starting at starts
// (see DayStarts), counting time only up to now. changes must be ordered by
// time and should include the last change before the first day, so the
// status in force at the start is known. It also returns the uptime of the
// whole range (nil when nothing was tracked).
func Daily(changes []Change, starts []time.Time, endOfLast, now time.Time) ([]Day, *float64) {
	days := make([]Day, len(starts))
	for i, s := range starts {
		end := endOfLast
		if i+1 < len(starts) {
			end = starts[i+1]
		}
		days[i] = Day{Date: s.Format("2006-01-02"), Status: NoData, Incidents: []IncidentRef{}, start: s, end: end}
	}
	// Walk each interval [changes[k].At, next) clipped to [day.start, min(day.end, now)).
	for k, c := range changes {
		if c.Status == nil {
			continue
		}
		from, to := c.At, now
		if k+1 < len(changes) && changes[k+1].At.Before(to) {
			to = changes[k+1].At
		}
		if !from.Before(to) {
			continue
		}
		for i := range days {
			d := &days[i]
			lo, hi := later(from, d.start), earlier(to, earlier(d.end, now))
			if !lo.Before(hi) {
				continue
			}
			secs := int64(hi.Sub(lo) / time.Second)
			if secs == 0 {
				secs = 1 // a sub-second interval still shows on the day
			}
			d.tracked += secs
			d.downtime += downtimeWeight[*c.Status] * float64(secs)
			d.Durations.add(*c.Status, secs)
			if d.Status == NoData {
				d.Status = *c.Status
			} else {
				d.Status = statuspolicy.Worse(d.Status, *c.Status)
			}
		}
	}
	var tracked int64
	var downtime float64
	for i := range days {
		d := &days[i]
		d.UptimePercent = percent(d.tracked, d.downtime)
		tracked += d.tracked
		downtime += d.downtime
	}
	return days, percent(tracked, downtime)
}

// percent returns the uptime rounded to two decimals, never rounding a
// period with downtime up to 100%.
func percent(tracked int64, downtime float64) *float64 {
	if tracked == 0 {
		return nil
	}
	// The epsilon absorbs float error (e.g. 0.99989999 for 99.99%).
	p := math.Floor((1-downtime/float64(tracked))*10000+1e-6) / 100
	return &p
}

func later(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func earlier(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
