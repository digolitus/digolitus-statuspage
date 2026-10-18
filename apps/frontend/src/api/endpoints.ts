import { request } from "./client";
import type {
  AuditEvent,
  Component,
  ComponentCreate,
  ComponentGroup,
  ComponentGroupCreate,
  ComponentGroupPatch,
  ComponentPatch,
  ExternalComponent,
  ExternalSource,
  ExternalSourceCreate,
  ExternalSourcePatch,
  Incident,
  IncidentCreate,
  IncidentPatch,
  IncidentStatus,
  IncidentTemplate,
  IncidentTemplateCreate,
  IncidentTemplatePatch,
  IncidentUpdate,
  IncidentUpdateCreate,
  Maintenance,
  MaintenanceCreate,
  MaintenancePatch,
  MaintenanceTemplate,
  MaintenanceTemplateCreate,
  MaintenanceTemplatePatch,
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

export const adminApi = {
  components: {
    list: (query: ListQuery & { is_active?: boolean; is_public?: boolean; group_id?: string }, o: Signal = {}) =>
      request<Page<Component>>("GET", `${A}/components`, { ...o, query }),
    create: (body: ComponentCreate) => request<Component>("POST", `${A}/components`, { body }),
    update: (cid: string, body: ComponentPatch) => request<Component>("PATCH", `${A}/components/${id(cid)}`, { body }),
  },
  groups: {
    list: (query: ListQuery, o: Signal = {}) => request<Page<ComponentGroup>>("GET", `${A}/component-groups`, { ...o, query }),
    create: (body: ComponentGroupCreate) => request<ComponentGroup>("POST", `${A}/component-groups`, { body }),
    update: (gid: string, body: ComponentGroupPatch) => request<ComponentGroup>("PATCH", `${A}/component-groups/${id(gid)}`, { body }),
    remove: (gid: string) => request<void>("DELETE", `${A}/component-groups/${id(gid)}`),
  },
  incidents: {
    list: (query: ListQuery & { state?: "active" | "resolved"; status?: IncidentStatus }, o: Signal = {}) =>
      request<Page<Incident>>("GET", `${A}/incidents`, { ...o, query }),
    get: (iid: string, o: Signal = {}) => request<Incident>("GET", `${A}/incidents/${id(iid)}`, o),
    create: (body: IncidentCreate) => request<Incident>("POST", `${A}/incidents`, { body }),
    patch: (iid: string, body: IncidentPatch) => request<Incident>("PATCH", `${A}/incidents/${id(iid)}`, { body }),
    publishUpdate: (iid: string, body: IncidentUpdateCreate) =>
      request<Incident>("POST", `${A}/incidents/${id(iid)}/updates`, { body }),
    updates: (iid: string, query: ListQuery, o: Signal = {}) =>
      request<Page<IncidentUpdate>>("GET", `${A}/incidents/${id(iid)}/updates`, { ...o, query }),
    resolve: (iid: string, message: string) => request<Incident>("POST", `${A}/incidents/${id(iid)}/resolve`, { body: { message } }),
  },
  maintenances: {
    list: (query: ListQuery & { state?: string }, o: Signal = {}) =>
      request<Page<Maintenance>>("GET", `${A}/maintenances`, { ...o, query }),
    get: (mid: string, o: Signal = {}) => request<Maintenance>("GET", `${A}/maintenances/${id(mid)}`, o),
    create: (body: MaintenanceCreate) => request<Maintenance>("POST", `${A}/maintenances`, { body }),
    patch: (mid: string, body: MaintenancePatch) => request<Maintenance>("PATCH", `${A}/maintenances/${id(mid)}`, { body }),
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
    create: (body: IncidentTemplateCreate) => request<IncidentTemplate>("POST", `${A}/incident-templates`, { body }),
    update: (tid: string, body: IncidentTemplatePatch) =>
      request<IncidentTemplate>("PATCH", `${A}/incident-templates/${id(tid)}`, { body }),
  },
  maintenanceTemplates: {
    list: (query: ListQuery & { is_active?: boolean }, o: Signal = {}) =>
      request<Page<MaintenanceTemplate>>("GET", `${A}/maintenance-templates`, { ...o, query }),
    create: (body: MaintenanceTemplateCreate) => request<MaintenanceTemplate>("POST", `${A}/maintenance-templates`, { body }),
    update: (tid: string, body: MaintenanceTemplatePatch) =>
      request<MaintenanceTemplate>("PATCH", `${A}/maintenance-templates/${id(tid)}`, { body }),
  },
  external: {
    list: (o: Signal = {}) => request<{ data: ExternalSource[] }>("GET", `${A}/external-sources`, o),
    create: (body: ExternalSourceCreate) => request<ExternalSource>("POST", `${A}/external-sources`, { body, timeoutMs: 25_000 }),
    update: (sid: string, body: ExternalSourcePatch) => request<ExternalSource>("PATCH", `${A}/external-sources/${id(sid)}`, { body }),
    remove: (sid: string) => request<void>("DELETE", `${A}/external-sources/${id(sid)}`),
    sync: (sid: string) => request<ExternalSource>("POST", `${A}/external-sources/${id(sid)}/sync`, { timeoutMs: 25_000 }),
    components: (sid: string, o: Signal = {}) =>
      request<{ data: ExternalComponent[] }>("GET", `${A}/external-sources/${id(sid)}/components`, o),
    setSelection: (sid: string, componentIds: string[]) =>
      request<{ data: ExternalComponent[] }>("PUT", `${A}/external-sources/${id(sid)}/selection`, { body: { component_ids: componentIds } }),
  },
  audit: {
    list: (
      query: ListQuery & { resource_type?: string; resource_id?: string; action?: string },
      o: Signal = {},
    ) => request<Page<AuditEvent>>("GET", `${A}/audit-events`, { ...o, query }),
  },
};
