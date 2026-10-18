// Package statuspolicy is the single, documented place where the effective
// status of components and the overall platform status are derived.
//
// All inputs are declared by operators (component status, incidents,
// maintenances). Nothing here represents automated health checks, so results
// must be presented as declared status, never as verified availability.
//
// Rules:
//  1. A component's effective status is the most severe of: its declared
//     status; the status implied by each active incident affecting it
//     (minor → degraded_performance, major → partial_outage,
//     critical → major_outage); and "maintenance" when an in-progress
//     maintenance affects it.
//  2. Severity order: major_outage > partial_outage > degraded_performance >
//     maintenance > operational. Maintenance therefore never hides an outage.
//  3. The overall status is the most severe effective status among public,
//     active components, considering also active incidents that affect no
//     public component (their impact still applies to the platform).
//  4. Maintenance does not degrade the overall status: if the most severe
//     status is "maintenance", the overall status is "maintenance", which
//     clients present as "maintenance in N components", not an outage.
//  5. With no public components and no active incidents the overall status
//     is "unknown": the platform has nothing to declare.
//  6. A component group's status is the most severe effective status of its
//     public components (same order); it is presentation only and does not
//     add anything to the overall status.
package statuspolicy

// Component statuses.
const (
	Operational         = "operational"
	DegradedPerformance = "degraded_performance"
	PartialOutage       = "partial_outage"
	MajorOutage         = "major_outage"
	Maintenance         = "maintenance"
	// Unknown is only used for the overall status.
	Unknown = "unknown"
)

// ComponentStatuses are the values an operator may declare.
var ComponentStatuses = []string{Operational, DegradedPerformance, PartialOutage, MajorOutage, Maintenance}

// Incident impacts.
const (
	ImpactMinor    = "minor"
	ImpactMajor    = "major"
	ImpactCritical = "critical"
)

// Impacts are the valid incident impacts.
var Impacts = []string{ImpactMinor, ImpactMajor, ImpactCritical}

var severity = map[string]int{
	Operational:         0,
	Maintenance:         1,
	DegradedPerformance: 2,
	PartialOutage:       3,
	MajorOutage:         4,
}

// Severity ranks a component status; unknown values rank lowest.
func Severity(status string) int {
	return severity[status]
}

// Worse returns the more severe of two statuses.
func Worse(a, b string) string {
	if Severity(b) > Severity(a) {
		return b
	}
	return a
}

// StatusForImpact maps an incident impact to the component status it implies.
func StatusForImpact(impact string) string {
	switch impact {
	case ImpactCritical:
		return MajorOutage
	case ImpactMajor:
		return PartialOutage
	default:
		return DegradedPerformance
	}
}

// ComponentState is the policy's view of a component.
type ComponentState struct {
	ID             string
	DeclaredStatus string
	Public         bool
}

// ActiveIncident is the policy's view of an active incident.
type ActiveIncident struct {
	Impact       string
	ComponentIDs []string
}

// ActiveMaintenance is the policy's view of an in-progress maintenance.
type ActiveMaintenance struct {
	ComponentIDs []string
}

// Overall summarises the platform.
type Overall struct {
	Status                     string
	AffectedComponents         int
	ComponentsUnderMaintenance int
}

// Evaluate returns each component's effective status (keyed by ID) and the
// overall status. Only active components should be passed in.
func Evaluate(components []ComponentState, incidents []ActiveIncident, maintenances []ActiveMaintenance) (map[string]string, Overall) {
	effective := make(map[string]string, len(components))
	public := make(map[string]bool, len(components))
	for _, c := range components {
		effective[c.ID] = c.DeclaredStatus
		public[c.ID] = c.Public
	}
	for _, m := range maintenances {
		for _, id := range m.ComponentIDs {
			if cur, ok := effective[id]; ok {
				effective[id] = Worse(cur, Maintenance)
			}
		}
	}

	overall := Overall{Status: Unknown}
	worst := Operational
	seen := false
	for _, inc := range incidents {
		implied := StatusForImpact(inc.Impact)
		touchesPublic := false
		for _, id := range inc.ComponentIDs {
			if cur, ok := effective[id]; ok {
				effective[id] = Worse(cur, implied)
				if public[id] {
					touchesPublic = true
				}
			}
		}
		if !touchesPublic {
			worst = Worse(worst, implied)
			seen = true
		}
	}

	for _, c := range components {
		if !c.Public {
			continue
		}
		seen = true
		st := effective[c.ID]
		worst = Worse(worst, st)
		switch st {
		case Operational:
		case Maintenance:
			overall.ComponentsUnderMaintenance++
		default:
			overall.AffectedComponents++
		}
	}
	if seen {
		overall.Status = worst
	}
	return effective, overall
}

// GroupSummary is the derived status of a component group.
type GroupSummary struct {
	Status                     string
	AffectedComponents         int
	ComponentsUnderMaintenance int
}

// Group derives a group's status from its components' effective statuses
// using the same severity order: the most severe status wins, maintenance
// never hides an outage, and an empty group is unknown.
func Group(effective []string) GroupSummary {
	if len(effective) == 0 {
		return GroupSummary{Status: Unknown}
	}
	g := GroupSummary{Status: Operational}
	for _, st := range effective {
		g.Status = Worse(g.Status, st)
		switch st {
		case Operational:
		case Maintenance:
			g.ComponentsUnderMaintenance++
		default:
			g.AffectedComponents++
		}
	}
	return g
}
