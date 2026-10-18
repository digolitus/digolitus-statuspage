// Package notify records what subscribers should hear about. Domain
// services call Record inside the transaction of the change, so an event
// exists if and only if the change was committed (transactional outbox).
// Delivery happens later, in the subscription worker.
//
// Events are public snapshots: only public components, public texts and the
// public page path. Internal notes never enter an event.
package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/database"
)

// Event kinds.
const (
	IncidentCreated      = "incident.created"
	IncidentUpdated      = "incident.updated"
	IncidentResolved     = "incident.resolved"
	MaintenanceScheduled = "maintenance.scheduled"
	MaintenanceUpdated   = "maintenance.updated"
	MaintenanceStarted   = "maintenance.started"
	MaintenanceCompleted = "maintenance.completed"
	MaintenanceCancelled = "maintenance.cancelled"
	AnnouncementPosted   = "announcement.published"
	Test                 = "test"
)

// Component is a public component named in an event.
type Component struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Event is the public snapshot sent to subscribers.
type Event struct {
	Kind         string      `json:"event"`
	ResourceType string      `json:"resource_type"`
	ResourceID   string      `json:"resource_id"`
	Title        string      `json:"title"`
	Status       string      `json:"status,omitempty"`
	Impact       string      `json:"impact,omitempty"`
	Message      string      `json:"message,omitempty"`
	Components   []Component `json:"components"`
	// Path is the public page of the resource (e.g. /incidents/{id}).
	Path           string     `json:"path"`
	ScheduledStart *time.Time `json:"scheduled_start,omitempty"`
	ScheduledEnd   *time.Time `json:"scheduled_end,omitempty"`
	Timezone       string     `json:"timezone,omitempty"`
	OccurredAt     time.Time  `json:"occurred_at"`
}

// Record stores e using q, which must be the transaction of the change.
func Record(ctx context.Context, q database.Querier, e Event) error {
	if e.Components == nil {
		e.Components = []Component{}
	}
	ids := make([]string, len(e.Components))
	for i, c := range e.Components {
		ids[i] = c.ID
	}
	payload, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("encode notification: %w", err)
	}
	_, err = q.Exec(ctx, `INSERT INTO notification_events (kind, resource_type, resource_id, payload, component_ids, created_at)
		VALUES ($1, $2, $3, $4, $5::uuid[], $6)`, e.Kind, e.ResourceType, e.ResourceID, payload, ids, e.OccurredAt)
	if err != nil {
		return fmt.Errorf("record notification: %w", err)
	}
	return nil
}

// Labels in Portuguese for messages (the API keeps the raw values).
var (
	IncidentStatusLabel = map[string]string{
		"investigating": "Investigando", "identified": "Causa identificada", "monitoring": "Monitorando", "resolved": "Resolvido",
	}
	ImpactLabel = map[string]string{
		"operational": "Operacional", "degraded_performance": "Performance degradada",
		"partial_outage": "Interrupção parcial", "major_outage": "Interrupção grave",
	}
	KindLabel = map[string]string{
		IncidentCreated: "Novo incidente", IncidentUpdated: "Atualização de incidente", IncidentResolved: "Incidente resolvido",
		MaintenanceScheduled: "Manutenção programada", MaintenanceUpdated: "Atualização de manutenção",
		MaintenanceStarted: "Manutenção iniciada", MaintenanceCompleted: "Manutenção concluída",
		MaintenanceCancelled: "Manutenção cancelada", AnnouncementPosted: "Comunicado", Test: "Mensagem de teste",
	}
)
