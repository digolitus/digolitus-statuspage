package clock

import (
	"testing"
	"time"
)

func TestParseInstant(t *testing.T) {
	sp, err := LoadZone("America/Sao_Paulo")
	if err != nil {
		t.Fatal(err)
	}
	ny, _ := LoadZone("America/New_York")
	tests := []struct {
		in   string
		loc  *time.Location
		want string
		err  bool
	}{
		{in: "2026-10-20T02:00:00-03:00", want: "2026-10-20T05:00:00Z"},
		{in: "2026-10-20T05:00:00.123456789Z", want: "2026-10-20T05:00:00.123456Z"},
		{in: "2026-10-20T02:00", loc: sp, want: "2026-10-20T05:00:00Z"},
		{in: "2026-10-20T02:00:30", loc: sp, want: "2026-10-20T05:00:30Z"},
		// An explicit offset wins over the timezone.
		{in: "2026-10-20T02:00:00Z", loc: sp, want: "2026-10-20T02:00:00Z"},
		// DST in New York: same local time, different UTC offsets.
		{in: "2026-01-15T09:00", loc: ny, want: "2026-01-15T14:00:00Z"},
		{in: "2026-07-15T09:00", loc: ny, want: "2026-07-15T13:00:00Z"},
		{in: "2026-10-20T02:00", err: true},
		{in: "20/10/2026 02:00", loc: sp, err: true},
		{in: "", loc: sp, err: true},
	}
	for _, tt := range tests {
		got, err := ParseInstant(tt.in, tt.loc)
		if tt.err {
			if err == nil {
				t.Errorf("ParseInstant(%q) = %v, want error", tt.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseInstant(%q): %v", tt.in, err)
			continue
		}
		if s := got.Format(time.RFC3339Nano); s != tt.want {
			t.Errorf("ParseInstant(%q) = %s, want %s", tt.in, s, tt.want)
		}
	}
}

func TestLoadZoneRejectsMachineDependentZones(t *testing.T) {
	for _, name := range []string{"", "Local", "Not/AZone"} {
		if _, err := LoadZone(name); err == nil {
			t.Errorf("LoadZone(%q) should fail", name)
		}
	}
	if _, err := LoadZone("UTC"); err != nil {
		t.Errorf("UTC: %v", err)
	}
}
