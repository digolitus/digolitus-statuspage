import type { Incident, Maintenance, Me, StatusSummary } from "../api/types";

export const emptySummary: StatusSummary = {
  overall: { status: "unknown", basis: "declared", affected_components: 0, components_under_maintenance: 0 },
  components: [],
  groups: [],
  active_incidents: [],
  maintenances: { in_progress: [], upcoming: [] },
  third_party: [],
  generated_at: "2026-03-01T12:00:00Z",
};

export const incident: Incident = {
  id: "11111111-1111-4111-8111-111111111111",
  title: "Lentidão na API",
  impact: "partial_outage",
  status: "investigating",
  internal_notes: "",
  started_at: "2026-03-01T11:00:00Z",
  resolved_at: null,
  template_id: null,
  components: [{ id: "c1", name: "API", slug: "api" }],
  updates: [
    { id: "u1", status: "investigating", message: "Investigando lentidão.", components: [{ id: "c1", name: "API", slug: "api" }], created_at: "2026-03-01T11:00:00Z" },
  ],
  created_at: "2026-03-01T11:00:00Z",
  updated_at: "2026-03-01T11:00:00Z",
};

export const maintenance: Maintenance = {
  id: "22222222-2222-4222-8222-222222222222",
  title: "Atualização do banco",
  description: "",
  expected_impact: "degraded_performance",
  status: "scheduled",
  timezone: "America/Sao_Paulo",
  scheduled_start: "2030-01-10T05:00:00Z",
  scheduled_end: "2030-01-10T07:00:00Z",
  started_at: null,
  completed_at: null,
  cancelled_at: null,
  start_overdue: false,
  end_overdue: false,
  internal_notes: "",
  template_id: null,
  components: [],
  updates: [{ id: "mu1", kind: "created", status: "scheduled", message: "Janela programada.", created_at: "2026-03-01T11:00:00Z" }],
  created_at: "2026-03-01T11:00:00Z",
  updated_at: "2026-03-01T11:00:00Z",
};

/** Signed-in first administrator (system role, every permission). */
export const adminMe: Me = {
  user: {
    id: "u1",
    email: "admin@example.com",
    name: "Admin",
    is_active: true,
    must_change_password: false,
    roles: [{ id: "r1", name: "Administrador", is_system: true }],
    last_login_at: "2026-03-01T12:00:00Z",
    locked_until: null,
    created_at: "2026-03-01T12:00:00Z",
    updated_at: "2026-03-01T12:00:00Z",
  },
  permissions: [
    "components.status", "components.manage", "third_party.manage", "incidents.manage", "maintenances.manage", "templates.manage",
    "announcements.write", "announcements.publish", "announcements.settings", "audit.read", "users.manage", "roles.manage",
  ],
  is_admin: true,
};

/** Route for an active admin session (spread into mockApi routes). */
export const session = (me: Me = adminMe) => ({ "GET /api/v1/auth/me": { body: me } });

/** 401 reply for calls without a valid session. */
export const unauthenticated = { status: 401, body: { error: { code: "unauthenticated", message: "Faça login para continuar." } } };
