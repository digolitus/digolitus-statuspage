import { request } from "./client";
import type {
  AuditEvent,
  Component,
  ComponentStatus,
  ExpectedImpact,
  Incident,
  IncidentImpact,
  IncidentStatus,
  IncidentTemplate,
  IncidentUpdate,
  Maintenance,
  MaintenanceTemplate,
  MaintenanceUpdate,
  Page,
  PublicComponent,
  PublicIncident,
  PublicMaintenance,
  StatusSummary,
} from "./types";

type Signal = { signal?: AbortSignal };
type ListQuery = { limit?: number; offset?: number; sort?: string };

export const publicApi = {
  status: (o: Signal = {}) => request<StatusSummary>("GET", "/api/v1/status", o),
  components: (o: Signal = {}) => request<Page<PublicComponent>>("GET", "/api/v1/components", { ...o, query: { limit: 100 } }),
  incidents: (query: ListQuery & { state?: "active" | "resolved" }, o: Signal = {}) =>
    request<Page<PublicIncident>>("GET", "/api/v1/incidents", { ...o, query }),
  incident: (id: string, o: Signal = {}) => request<PublicIncident>("GET", `/api/v1/incidents/${encodeURIComponent(id)}`, o),
  maintenances: (query: ListQuery & { state?: string }, o: Signal = {}) =>
    request<Page<PublicMaintenance>>("GET", "/api/v1/maintenances", { ...o, query }),
  maintenance: (id: string, o: Signal = {}) =>
    request<PublicMaintenance>("GET", `/api/v1/maintenances/${encodeURIComponent(id)}`, o),
};

const A = "/api/v1/admin";
const id = (v: string) => encodeURIComponent(v);

export interface ComponentInput {
  name?: string;
  slug?: string;
  description?: string;
  status?: ComponentStatus;
  is_public?: boolean;
  is_active?: boolean;
  display_order?: number;
}

export interface IncidentCreateInput {
  title: string;
  message: string;
  impact: IncidentImpact;
  status?: Exclude<IncidentStatus, "resolved">;
  component_ids?: string[];
  started_at?: string;
  internal_notes?: string;
  template_id?: string;
}

export interface IncidentPatchInput {
  title?: string;
  impact?: IncidentImpact;
  internal_notes?: string;
  started_at?: string;
}

export interface IncidentUpdateInput {
  status: IncidentStatus;
  message: string;
  component_ids?: string[];
}

export interface MaintenanceCreateInput {
  title: string;
  description?: string;
  message?: string;
  expected_impact?: ExpectedImpact;
  timezone: string;
  scheduled_start: string;
  scheduled_end: string;
  component_ids?: string[];
  internal_notes?: string;
  template_id?: string;
}

export type MaintenancePatchInput = Partial<Omit<MaintenanceCreateInput, "message" | "template_id">>;

export interface IncidentTemplateInput {
  name?: string;
  description?: string;
  title?: string;
  message?: string;
  impact?: IncidentImpact;
  status?: IncidentStatus;
  guidance?: string;
  is_active?: boolean;
}

export interface MaintenanceTemplateInput {
  name?: string;
  description?: string;
  title?: string;
  message?: string;
  expected_impact?: ExpectedImpact;
  guidance?: string;
  is_active?: boolean;
}

export const adminApi = {
  components: {
    list: (query: ListQuery & { is_active?: boolean; is_public?: boolean }, o: Signal = {}) =>
      request<Page<Component>>("GET", `${A}/components`, { ...o, query }),
    create: (body: ComponentInput) => request<Component>("POST", `${A}/components`, { body }),
    update: (cid: string, body: ComponentInput) => request<Component>("PATCH", `${A}/components/${id(cid)}`, { body }),
  },
  incidents: {
    list: (query: ListQuery & { state?: "active" | "resolved"; status?: IncidentStatus }, o: Signal = {}) =>
      request<Page<Incident>>("GET", `${A}/incidents`, { ...o, query }),
    get: (iid: string, o: Signal = {}) => request<Incident>("GET", `${A}/incidents/${id(iid)}`, o),
    create: (body: IncidentCreateInput) => request<Incident>("POST", `${A}/incidents`, { body }),
    patch: (iid: string, body: IncidentPatchInput) => request<Incident>("PATCH", `${A}/incidents/${id(iid)}`, { body }),
    publishUpdate: (iid: string, body: IncidentUpdateInput) =>
      request<Incident>("POST", `${A}/incidents/${id(iid)}/updates`, { body }),
    updates: (iid: string, query: ListQuery, o: Signal = {}) =>
      request<Page<IncidentUpdate>>("GET", `${A}/incidents/${id(iid)}/updates`, { ...o, query }),
    resolve: (iid: string, message: string) => request<Incident>("POST", `${A}/incidents/${id(iid)}/resolve`, { body: { message } }),
  },
  maintenances: {
    list: (query: ListQuery & { state?: string }, o: Signal = {}) =>
      request<Page<Maintenance>>("GET", `${A}/maintenances`, { ...o, query }),
    get: (mid: string, o: Signal = {}) => request<Maintenance>("GET", `${A}/maintenances/${id(mid)}`, o),
    create: (body: MaintenanceCreateInput) => request<Maintenance>("POST", `${A}/maintenances`, { body }),
    patch: (mid: string, body: MaintenancePatchInput) => request<Maintenance>("PATCH", `${A}/maintenances/${id(mid)}`, { body }),
    postMessage: (mid: string, message: string) =>
      request<Maintenance>("POST", `${A}/maintenances/${id(mid)}/updates`, { body: { message } }),
    updates: (mid: string, query: ListQuery, o: Signal = {}) =>
      request<Page<MaintenanceUpdate>>("GET", `${A}/maintenances/${id(mid)}/updates`, { ...o, query }),
    transition: (mid: string, action: "start" | "complete" | "cancel", message?: string) =>
      request<Maintenance>("POST", `${A}/maintenances/${id(mid)}/${action}`, { body: message ? { message } : {} }),
  },
  incidentTemplates: {
    list: (query: ListQuery & { is_active?: boolean }, o: Signal = {}) =>
      request<Page<IncidentTemplate>>("GET", `${A}/incident-templates`, { ...o, query }),
    create: (body: IncidentTemplateInput) => request<IncidentTemplate>("POST", `${A}/incident-templates`, { body }),
    update: (tid: string, body: IncidentTemplateInput) =>
      request<IncidentTemplate>("PATCH", `${A}/incident-templates/${id(tid)}`, { body }),
  },
  maintenanceTemplates: {
    list: (query: ListQuery & { is_active?: boolean }, o: Signal = {}) =>
      request<Page<MaintenanceTemplate>>("GET", `${A}/maintenance-templates`, { ...o, query }),
    create: (body: MaintenanceTemplateInput) => request<MaintenanceTemplate>("POST", `${A}/maintenance-templates`, { body }),
    update: (tid: string, body: MaintenanceTemplateInput) =>
      request<MaintenanceTemplate>("PATCH", `${A}/maintenance-templates/${id(tid)}`, { body }),
  },
  audit: {
    list: (
      query: ListQuery & { resource_type?: string; resource_id?: string; action?: string },
      o: Signal = {},
    ) => request<Page<AuditEvent>>("GET", `${A}/audit-events`, { ...o, query }),
  },
};
