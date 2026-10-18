package statuspolicy

import "testing"

func TestEvaluate(t *testing.T) {
	api := ComponentState{ID: "api", DeclaredStatus: Operational, Public: true}
	web := ComponentState{ID: "web", DeclaredStatus: Operational, Public: true}
	internal := ComponentState{ID: "internal", DeclaredStatus: Operational, Public: false}

	tests := []struct {
		name          string
		components    []ComponentState
		incidents     []ActiveIncident
		maintenances  []ActiveMaintenance
		wantOverall   Overall
		wantEffective map[string]string
	}{
		{
			name:        "no components means unknown",
			wantOverall: Overall{Status: Unknown},
		},
		{
			name:        "only hidden components means unknown",
			components:  []ComponentState{internal},
			wantOverall: Overall{Status: Unknown},
		},
		{
			name:          "all operational",
			components:    []ComponentState{api, web},
			wantOverall:   Overall{Status: Operational},
			wantEffective: map[string]string{"api": Operational, "web": Operational},
		},
		{
			name:          "declared status is respected",
			components:    []ComponentState{{ID: "api", DeclaredStatus: PartialOutage, Public: true}, web},
			wantOverall:   Overall{Status: PartialOutage, AffectedComponents: 1},
			wantEffective: map[string]string{"api": PartialOutage, "web": Operational},
		},
		{
			name:          "incident impact raises component status",
			components:    []ComponentState{api, web},
			incidents:     []ActiveIncident{{Impact: ImpactCritical, ComponentIDs: []string{"api"}}},
			wantOverall:   Overall{Status: MajorOutage, AffectedComponents: 1},
			wantEffective: map[string]string{"api": MajorOutage, "web": Operational},
		},
		{
			name:          "incident never lowers a worse declared status",
			components:    []ComponentState{{ID: "api", DeclaredStatus: MajorOutage, Public: true}},
			incidents:     []ActiveIncident{{Impact: ImpactMinor, ComponentIDs: []string{"api"}}},
			wantOverall:   Overall{Status: MajorOutage, AffectedComponents: 1},
			wantEffective: map[string]string{"api": MajorOutage},
		},
		{
			name:          "maintenance on one component does not mark the platform as down",
			components:    []ComponentState{api, web},
			maintenances:  []ActiveMaintenance{{ComponentIDs: []string{"api"}}},
			wantOverall:   Overall{Status: Maintenance, ComponentsUnderMaintenance: 1},
			wantEffective: map[string]string{"api": Maintenance, "web": Operational},
		},
		{
			name:          "maintenance does not hide an outage",
			components:    []ComponentState{api},
			incidents:     []ActiveIncident{{Impact: ImpactMajor, ComponentIDs: []string{"api"}}},
			maintenances:  []ActiveMaintenance{{ComponentIDs: []string{"api"}}},
			wantOverall:   Overall{Status: PartialOutage, AffectedComponents: 1},
			wantEffective: map[string]string{"api": PartialOutage},
		},
		{
			name:          "incident without public components still affects overall",
			components:    []ComponentState{api, internal},
			incidents:     []ActiveIncident{{Impact: ImpactMajor, ComponentIDs: []string{"internal"}}},
			wantOverall:   Overall{Status: PartialOutage},
			wantEffective: map[string]string{"api": Operational, "internal": PartialOutage},
		},
		{
			name:        "incident without components and no public components",
			incidents:   []ActiveIncident{{Impact: ImpactMinor}},
			wantOverall: Overall{Status: DegradedPerformance},
		},
		{
			name:          "unknown component references are ignored",
			components:    []ComponentState{api},
			incidents:     []ActiveIncident{{Impact: ImpactMinor, ComponentIDs: []string{"api", "gone"}}},
			wantOverall:   Overall{Status: DegradedPerformance, AffectedComponents: 1},
			wantEffective: map[string]string{"api": DegradedPerformance},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			effective, overall := Evaluate(tt.components, tt.incidents, tt.maintenances)
			if overall != tt.wantOverall {
				t.Fatalf("overall = %+v, want %+v", overall, tt.wantOverall)
			}
			for id, want := range tt.wantEffective {
				if effective[id] != want {
					t.Errorf("effective[%s] = %q, want %q", id, effective[id], want)
				}
			}
		})
	}
}

func TestSeverityOrder(t *testing.T) {
	order := []string{Operational, Maintenance, DegradedPerformance, PartialOutage, MajorOutage}
	for i := 1; i < len(order); i++ {
		if Severity(order[i]) <= Severity(order[i-1]) {
			t.Fatalf("%s must be more severe than %s", order[i], order[i-1])
		}
	}
}
