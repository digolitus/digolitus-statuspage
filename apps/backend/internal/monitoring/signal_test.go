package monitoring

import (
	"net/url"
	"testing"
)

func TestFields(t *testing.T) {
	f, err := Fields(url.Values{"token": {"secret"}, "Extra": {"q"}}, "application/json", []byte(`{"monitorID": 798122451, "alertType": "1", "ok": true, "nested": {"x": 1}}`))
	if err != nil {
		t.Fatal(err)
	}
	if f["monitorid"] != "798122451" || f["alerttype"] != "1" || f["ok"] != "true" || f["extra"] != "q" {
		t.Fatalf("fields = %v", f)
	}
	if _, ok := f["token"]; ok {
		t.Fatal("the token must not be part of the fields")
	}
	if _, ok := f["nested"]; ok {
		t.Fatal("nested values are ignored")
	}

	form, err := Fields(nil, "application/x-www-form-urlencoded", []byte("check_id=abc&status=down"))
	if err != nil || form["check_id"] != "abc" || form["status"] != "down" {
		t.Fatalf("form = %v, %v", form, err)
	}
	// JSON sent as text/plain is accepted.
	if plain, err := Fields(nil, "text/plain", []byte(`{"status":"up"}`)); err != nil || plain["status"] != "up" {
		t.Fatalf("plain = %v, %v", plain, err)
	}
	if _, err := Fields(nil, "application/json", []byte("not json")); err == nil {
		t.Fatal("invalid JSON must fail")
	}
	if empty, err := Fields(nil, "", nil); err != nil || len(empty) != 0 {
		t.Fatalf("empty body = %v, %v", empty, err)
	}
}

func TestParse(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		fields   map[string]string
		want     Signal
		wantErr  bool
	}{
		{"uptimerobot down", ProviderUptimeRobot,
			map[string]string{"monitorid": "1", "monitorfriendlyname": "Site", "alerttype": "1", "alertdetails": "Timeout"},
			Signal{ExternalID: "1", Name: "Site", Kind: KindDown, Detail: "Timeout"}, false},
		{"uptimerobot up by friendly name", ProviderUptimeRobot,
			map[string]string{"monitorid": "1", "alerttypefriendlyname": "Up"},
			Signal{ExternalID: "1", Kind: KindUp}, false},
		{"uptimerobot ssl expiry is ignored", ProviderUptimeRobot,
			map[string]string{"monitorid": "1", "alerttype": "3"},
			Signal{ExternalID: "1", Kind: KindIgnored, Detail: "alerta 3 não altera o status"}, false},
		{"uptimerobot without alert type", ProviderUptimeRobot, map[string]string{"monitorid": "1"}, Signal{}, true},
		{"uptimerobot unfilled template", ProviderUptimeRobot, map[string]string{"monitorid": "*monitorID*", "alerttype": "1"}, Signal{}, true},
		{"healthchecks down", ProviderHealthchecks,
			map[string]string{"check_id": "6f3c", "name": "Backup", "status": "down"},
			Signal{ExternalID: "6f3c", Name: "Backup", Kind: KindDown}, false},
		{"healthchecks code alias", ProviderHealthchecks,
			map[string]string{"code": "6f3c", "name": "$NAME", "status": "UP"},
			Signal{ExternalID: "6f3c", Kind: KindUp}, false},
		{"healthchecks unfilled id", ProviderHealthchecks, map[string]string{"check_id": "$CODE", "status": "down"}, Signal{}, true},
		{"healthchecks without id", ProviderHealthchecks, map[string]string{"status": "down"}, Signal{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.provider, tt.fields)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}
