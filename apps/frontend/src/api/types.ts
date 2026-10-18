// Types mirroring the schemas in apps/backend/api/openapi.yaml (the API
// contract). Keep them in sync when the contract changes.

export type ComponentStatus =
  | "operational"
  | "degraded_performance"
  | "partial_outage"
  | "major_outage"
  | "maintenance";

export type OverallStatus = ComponentStatus | "unknown";
export type IncidentStatus = "investigating" | "identified" | "monitoring" | "resolved";
export type IncidentImpact = "minor" | "major" | "critical";
export type MaintenanceStatus = "scheduled" | "in_progress" | "completed" | "cancelled";
export type ExpectedImpact = "none" | "degraded_performance" | "partial_outage" | "major_outage";
export type MaintenanceUpdateKind = "created" | "message" | "started" | "completed" | "cancelled";

export interface Pagination {
  limit: number;
  offset: number;
  total: number;
}

export interface Page<T> {
  data: T[];
  pagination: Pagination;
}

export interface ComponentRef {
  id: string;
  name: string;
  slug: string;
}

export interface Component {
  id: string;
  name: string;
  slug: string;
  description: string;
  status: ComponentStatus;
  is_public: boolean;
  is_active: boolean;
  display_order: number;
  created_at: string;
  updated_at: string;
}

export interface PublicComponent {
  id: string;
  name: string;
  slug: string;
  description: string;
  status: ComponentStatus;
  updated_at: string;
}

export interface IncidentUpdate {
  id: string;
  status: IncidentStatus;
  message: string;
  components: ComponentRef[] | null;
  created_at: string;
}

export interface PublicIncident {
  id: string;
  title: string;
  impact: IncidentImpact;
  status: IncidentStatus;
  started_at: string;
  resolved_at: string | null;
  components: ComponentRef[];
  latest_update?: IncidentUpdate;
  updates?: IncidentUpdate[];
  created_at: string;
  updated_at: string;
}

export interface Incident extends PublicIncident {
  internal_notes: string;
  template_id: string | null;
}

export interface MaintenanceUpdate {
  id: string;
  kind: MaintenanceUpdateKind;
  status: MaintenanceStatus;
  message: string | null;
  created_at: string;
}

export interface PublicMaintenance {
  id: string;
  title: string;
  description: string;
  expected_impact: ExpectedImpact;
  status: MaintenanceStatus;
  timezone: string;
  scheduled_start: string;
  scheduled_end: string;
  started_at: string | null;
  completed_at: string | null;
  cancelled_at: string | null;
  start_overdue: boolean;
  end_overdue: boolean;
  components: ComponentRef[];
  latest_update?: MaintenanceUpdate;
  updates?: MaintenanceUpdate[];
  created_at: string;
  updated_at: string;
}

export interface Maintenance extends PublicMaintenance {
  internal_notes: string;
  template_id: string | null;
}

export interface IncidentTemplate {
  id: string;
  name: string;
  description: string;
  title: string;
  message: string;
  impact: IncidentImpact;
  status: IncidentStatus;
  guidance: string;
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

export interface MaintenanceTemplate {
  id: string;
  name: string;
  description: string;
  title: string;
  message: string;
  expected_impact: ExpectedImpact;
  guidance: string;
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

export interface AuditEvent {
  id: number;
  occurred_at: string;
  resource_type: "component" | "incident" | "maintenance" | "incident_template" | "maintenance_template";
  resource_id: string;
  action: string;
  source: string;
  request_id: string | null;
  changes: { before?: Record<string, unknown>; after?: Record<string, unknown> };
}

export interface StatusSummary {
  overall: {
    status: OverallStatus;
    basis: "declared";
    affected_components: number;
    components_under_maintenance: number;
  };
  components: PublicComponent[];
  active_incidents: PublicIncident[];
  maintenances: {
    in_progress: PublicMaintenance[];
    upcoming: PublicMaintenance[];
  };
  generated_at: string;
}

export interface FieldError {
  field: string;
  message: string;
}

export interface ErrorBody {
  error: {
    code: string;
    message: string;
    details?: FieldError[];
    request_id?: string;
  };
}
