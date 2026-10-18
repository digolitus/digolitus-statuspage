package clock

import (
	"errors"
	"strings"
	"time"
)

var localLayouts = []string{"2006-01-02T15:04:05", "2006-01-02T15:04"}

// ErrNoZone reports a local date-time given without a timezone to interpret it.
var ErrNoZone = errors.New("local date-time requires a timezone")

// ParseInstant parses an RFC 3339 timestamp with an explicit offset. When
// loc is not nil it also accepts a local date-time without offset
// ("2006-01-02T15:04[:05]") interpreted in loc. The machine and browser
// timezones never take part in the interpretation. The result is normalized
// to UTC.
//
// For a local time that does not exist (DST gap) or is ambiguous (DST
// overlap), Go's time.Date rules apply: the result is shifted forward by the
// gap, or the first occurrence is used.
func ParseInstant(s string, loc *time.Location) (time.Time, error) {
	s = strings.TrimSpace(s)
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return Normalize(t), nil
	}
	for _, layout := range localLayouts {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			if loc == nil {
				return time.Time{}, ErrNoZone
			}
			local := time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, loc)
			return Normalize(local), nil
		}
	}
	return time.Time{}, errors.New("invalid timestamp")
}

// LoadZone validates an IANA timezone name. "Local" is rejected because it
// would depend on the server's configuration.
func LoadZone(name string) (*time.Location, error) {
	if name == "" || name == "Local" || len(name) > 64 {
		return nil, errors.New("invalid timezone")
	}
	return time.LoadLocation(name)
}
