import { request } from "./client";
import type {
  AdminSuspicionList,
  Feature,
  MySuspicion,
  PublicFeatures,
  PublicSuspicion,
  ReportCreate,
  Reporter,
  ReporterProviders,
  SuspicionDetail,
  SuspicionReporter,
  SuspicionStatus,
  AdminBranding,
  BrandingImageKind,
  BrandingSettings,
  PublicBranding,
  Announcement,
  AnnouncementCategory,
  AnnouncementCategoryCreate,
  AnnouncementCategoryPatch,
  AnnouncementCreate,
  AnnouncementPatch,
  AnnouncementState,
  AuditEvent,
  PresenceArea,
  PresenceSnapshot,
  LoginProvider,
  Me,
  PermissionInfo,
  Role,
  RoleCreate,
  RolePatch,
  SetupInput,
  User,
  UserCreate,
  UserPatch,
  Component,
  ComponentCreate,
  ComponentGroup,
  ComponentGroupCreate,
  ComponentGroupPatch,
  ComponentPatch,
  ExternalComponent,
  ExternalEvent,
  ExternalOutage,
  ExternalSource,
  ExternalUptime,
  ExternalSourceCreate,
  ExternalSourcePatch,
  Monitor,
  MonitorCreate,
  MonitorPatch,
  MonitorState,
  MonitoringEvent,
  MonitoringIntegration,
  MonitoringIntegrationCreate,
  MonitoringIntegrationPatch,
  MonitoringIntegrationWithToken,
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
  AnnouncementTemplate,
  AnnouncementTemplateCreate,
  AnnouncementTemplatePatch,
  MaintenanceUpdate,
  Page,
  PublicAnnouncement,
  PublicAnnouncementCategory,
  PublicComponent,
  PublicIncident,
  PublicMaintenance,
  StatusChange,
  StatusChangeCause,
  StatusSummary,
  Team,
  Uptime,
  TeamCreate,
  TeamPatch,
  Department,
  DepartmentCreate,
  DepartmentPatch,
  DepartmentLevels,
  SubscribeInput,
  Subscription,
  SubscriptionAdminCreate,
  SubscriptionAdminPatch,
  SubscriptionChannel,
  SubscriptionCreated,
  SubscriptionList,
  SubscriptionManageView,
  SubscriptionOptions,
  SubscriptionStatus,
} from "./types";

type Signal = { signal?: AbortSignal };
type ListQuery = { limit?: number; offset?: number; sort?: string };
export type UptimeRange = 30 | 60 | 90;

export const publicApi = {
  status: (o: Signal = {}) => request<StatusSummary>("GET", "/api/v1/status", o),
  uptime: (days: UptimeRange, timezone: string, o: Signal = {}) =>
    request<Uptime>("GET", "/api/v1/status/uptime", { ...o, query: { days, timezone } }),
  components: (o: Signal = {}) => request<Page<PublicComponent>>("GET", "/api/v1/components", { ...o, query: { limit: 100 } }),
  incidents: (query: ListQuery & { state?: "active" | "resolved" }, o: Signal = {}) =>
    request<Page<PublicIncident>>("GET", "/api/v1/incidents", { ...o, query }),
  incident: (id: string, o: Signal = {}) => request<PublicIncident>("GET", `/api/v1/incidents/${encodeURIComponent(id)}`, o),
  maintenances: (query: ListQuery & { state?: string }, o: Signal = {}) =>
    request<Page<PublicMaintenance>>("GET", "/api/v1/maintenances", { ...o, query }),
  maintenance: (id: string, o: Signal = {}) =>
    request<PublicMaintenance>("GET", `/api/v1/maintenances/${encodeURIComponent(id)}`, o),
  announcements: (query: ListQuery & { category_id?: string; component_id?: string; pinned?: boolean }, o: Signal = {}) =>
    request<Page<PublicAnnouncement>>("GET", "/api/v1/announcements", { ...o, query }),
  announcement: (id: string, o: Signal = {}) =>
    request<PublicAnnouncement>("GET", `/api/v1/announcements/${encodeURIComponent(id)}`, o),
  announcementCategories: (o: Signal = {}) =>
    request<{ data: PublicAnnouncementCategory[] }>("GET", "/api/v1/announcement-categories", o),
  branding: (o: Signal = {}) => request<PublicBranding>("GET", "/api/v1/branding", o),
};

/** Heartbeats of "who is here now" (no cookie; tab_id is random per tab). */
export const presenceApi = {
  public: (tabId: string, area: PresenceArea) =>
    request<void>("POST", "/api/v1/presence", { body: { tab_id: tabId, area } }),
  admin: (tabId: string, area: string, hidden: boolean) =>
    request<void>("POST", "/api/v1/admin/presence", { body: { tab_id: tabId, area, hidden } }),
};

const S = "/api/v1/subscriptions";

/** Public self-service subscriptions; tokens come from links sent by e-mail. */
export const subscriptionApi = {
  options: (o: Signal = {}) => request<SubscriptionOptions>("GET", `${S}/options`, o),
  subscribe: (body: SubscribeInput) => request<{ message: string }>("POST", S, { body }),
  confirm: (token: string) => request<SubscriptionManageView>("POST", `${S}/confirm`, { body: { token } }),
  manage: (token: string, o: Signal = {}) => request<SubscriptionManageView>("POST", `${S}/manage`, { ...o, body: { token } }),
  updatePreferences: (token: string, componentIds: string[]) =>
    request<SubscriptionManageView>("PUT", `${S}/manage`, { body: { token, component_ids: componentIds } }),
  unsubscribe: (token: string) => request<void>("POST", `${S}/unsubscribe`, { body: { token } }),
};

export const authApi = {
  setupStatus: (o: Signal = {}) => request<{ setup_required: boolean }>("GET", "/api/v1/auth/setup", o),
  setup: (body: SetupInput) => request<Me>("POST", "/api/v1/auth/setup", { body }),
  login: (email: string, password: string) => request<Me>("POST", "/api/v1/auth/login", { body: { email, password } }),
  logout: () => request<void>("POST", "/api/v1/auth/logout"),
  me: (o: Signal = {}) => request<Me>("GET", "/api/v1/auth/me", o),
  changePassword: (currentPassword: string, newPassword: string) =>
    request<void>("POST", "/api/v1/auth/password", { body: { current_password: currentPassword, new_password: newPassword } }),
  providers: (o: Signal = {}) => request<{ data: LoginProvider[] }>("GET", "/api/v1/auth/providers", o),
};

/** Social sign-in starts with a full-page navigation, not a fetch. */
export const socialLoginUrl = (provider: LoginProvider["id"]) => `/api/v1/auth/oauth/${provider}`;

/** Optional features switched on and off in the admin area. */
export const featuresApi = {
  public: (o: Signal = {}) => request<PublicFeatures>("GET", "/api/v1/features", o),
};

const R = "/api/v1/reporter";

/** "Reportar suspeita": public list and the reporter's own account (cookie dsp_reporter). */
export const suspicionApi = {
  list: (o: Signal = {}) => request<{ data: PublicSuspicion[] }>("GET", "/api/v1/suspicions", o),
  providers: (o: Signal = {}) => request<ReporterProviders>("GET", `${R}/providers`, o),
  sendLink: (email: string, name: string) => request<void>("POST", `${R}/login-link`, { body: { email, name } }),
  redeem: (token: string) => request<Reporter>("POST", `${R}/login`, { body: { token } }),
  logout: () => request<void>("POST", `${R}/logout`),
  me: (o: Signal = {}) => request<Reporter>("GET", `${R}/me`, o),
  mine: (o: Signal = {}) => request<{ data: MySuspicion[] }>("GET", `${R}/suspicions`, o),
  report: (body: ReportCreate) => request<MySuspicion>("POST", `${R}/suspicions`, { body }),
  reply: (sid: string, body: string) => request<MySuspicion>("POST", `${R}/suspicions/${encodeURIComponent(sid)}/messages`, { body: { body } }),
};

export const reporterGoogleUrl = `${R}/oauth/google`;

const A = "/api/v1/admin";
const id = (v: string) => encodeURIComponent(v);

export const adminApi = {
  branding: {
    get: (o: Signal = {}) => request<AdminBranding>("GET", "/api/v1/admin/branding", o),
    update: (body: BrandingSettings) => request<AdminBranding>("PUT", "/api/v1/admin/branding", { body }),
    uploadImage: (kind: BrandingImageKind, file: Blob) =>
      request<AdminBranding>("PUT", `/api/v1/admin/branding/images/${kind}`, { rawBody: file, timeoutMs: 30_000 }),
    deleteImage: (kind: BrandingImageKind) => request<AdminBranding>("DELETE", `/api/v1/admin/branding/images/${kind}`),
  },
  components: {
    list: (query: ListQuery & { is_active?: boolean; is_public?: boolean; group_id?: string; subgroup_id?: string; team_id?: string }, o: Signal = {}) =>
      request<Page<Component>>("GET", `${A}/components`, { ...o, query }),
    create: (body: ComponentCreate) => request<Component>("POST", `${A}/components`, { body }),
    update: (cid: string, body: ComponentPatch) => request<Component>("PATCH", `${A}/components/${id(cid)}`, { body }),
  },
  groups: {
    list: (query: ListQuery, o: Signal = {}) => request<Page<ComponentGroup>>("GET", `${A}/component-groups`, { ...o, query }),
    /** Every group and subgroup, following pagination (pages hold at most 100). */
    all: async (o: Signal = {}): Promise<ComponentGroup[]> => {
      const out: ComponentGroup[] = [];
      for (;;) {
        const p = await request<Page<ComponentGroup>>("GET", `${A}/component-groups`, { ...o, query: { limit: 100, offset: out.length } });
        out.push(...p.data);
        if (p.data.length === 0 || out.length >= p.pagination.total) return out;
      }
    },
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
    resolve: (iid: string, message: string, resolvedAt?: string) =>
      request<Incident>("POST", `${A}/incidents/${id(iid)}/resolve`, { body: resolvedAt ? { message, resolved_at: resolvedAt } : { message } }),
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
  announcementTemplates: {
    list: (query: ListQuery & { is_active?: boolean }, o: Signal = {}) =>
      request<Page<AnnouncementTemplate>>("GET", `${A}/announcement-templates`, { ...o, query }),
    create: (body: AnnouncementTemplateCreate) => request<AnnouncementTemplate>("POST", `${A}/announcement-templates`, { body }),
    update: (tid: string, body: AnnouncementTemplatePatch) =>
      request<AnnouncementTemplate>("PATCH", `${A}/announcement-templates/${id(tid)}`, { body }),
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
    outages: (sid: string, query: ListQuery & { only_selected?: boolean }, o: Signal = {}) =>
      request<Page<ExternalOutage>>("GET", `${A}/external-sources/${id(sid)}/outages`, { ...o, query }),
    events: (sid: string, query: ListQuery & { state?: "resolved" | "open" }, o: Signal = {}) =>
      request<Page<ExternalEvent>>("GET", `${A}/external-sources/${id(sid)}/events`, { ...o, query }),
    uptime: (sid: string, days: UptimeRange, timezone: string, o: Signal = {}) =>
      request<ExternalUptime>("GET", `${A}/external-sources/${id(sid)}/uptime`, { ...o, query: { days, timezone } }),
  },
  monitoring: {
    list: (o: Signal = {}) => request<{ data: MonitoringIntegration[] }>("GET", `${A}/monitoring-integrations`, o),
    create: (body: MonitoringIntegrationCreate) =>
      request<MonitoringIntegrationWithToken>("POST", `${A}/monitoring-integrations`, { body }),
    update: (mid: string, body: MonitoringIntegrationPatch) =>
      request<MonitoringIntegration>("PATCH", `${A}/monitoring-integrations/${id(mid)}`, { body }),
    remove: (mid: string) => request<void>("DELETE", `${A}/monitoring-integrations/${id(mid)}`),
    rotateToken: (mid: string) => request<MonitoringIntegrationWithToken>("POST", `${A}/monitoring-integrations/${id(mid)}/token`),
    events: (mid: string, o: Signal = {}) =>
      request<{ data: MonitoringEvent[] }>("GET", `${A}/monitoring-integrations/${id(mid)}/events`, o),
    addMonitor: (mid: string, body: MonitorCreate) => request<Monitor>("POST", `${A}/monitoring-integrations/${id(mid)}/monitors`, { body }),
    monitors: (query: { integration_id?: string; component_id?: string; state?: MonitorState }, o: Signal = {}) =>
      request<{ data: Monitor[] }>("GET", `${A}/monitors`, { ...o, query }),
    updateMonitor: (mid: string, body: MonitorPatch) => request<Monitor>("PATCH", `${A}/monitors/${id(mid)}`, { body }),
    removeMonitor: (mid: string) => request<void>("DELETE", `${A}/monitors/${id(mid)}`),
  },
  announcements: {
    list: (
      query: ListQuery & { state?: AnnouncementState; q?: string; category_id?: string; team_id?: string },
      o: Signal = {},
    ) => request<Page<Announcement>>("GET", `${A}/announcements`, { ...o, query }),
    get: (aid: string, o: Signal = {}) => request<Announcement>("GET", `${A}/announcements/${id(aid)}`, o),
    create: (body: AnnouncementCreate) => request<Announcement>("POST", `${A}/announcements`, { body }),
    update: (aid: string, body: AnnouncementPatch) => request<Announcement>("PATCH", `${A}/announcements/${id(aid)}`, { body }),
    remove: (aid: string) => request<void>("DELETE", `${A}/announcements/${id(aid)}`),
    publish: (aid: string, publishAt?: string) =>
      request<Announcement>("POST", `${A}/announcements/${id(aid)}/publish`, { body: publishAt ? { publish_at: publishAt } : {} }),
    transition: (aid: string, action: "unpublish" | "archive" | "restore") =>
      request<Announcement>("POST", `${A}/announcements/${id(aid)}/${action}`, { body: {} }),
  },
  announcementCategories: {
    list: (query: { is_active?: boolean } = {}, o: Signal = {}) =>
      request<{ data: AnnouncementCategory[] }>("GET", `${A}/announcement-categories`, { ...o, query }),
    create: (body: AnnouncementCategoryCreate) => request<AnnouncementCategory>("POST", `${A}/announcement-categories`, { body }),
    update: (cid: string, body: AnnouncementCategoryPatch) =>
      request<AnnouncementCategory>("PATCH", `${A}/announcement-categories/${id(cid)}`, { body }),
    remove: (cid: string) => request<void>("DELETE", `${A}/announcement-categories/${id(cid)}`),
  },
  teams: {
    list: (query: { is_active?: boolean } = {}, o: Signal = {}) => request<{ data: Team[] }>("GET", `${A}/teams`, { ...o, query }),
    create: (body: TeamCreate) => request<Team>("POST", `${A}/teams`, { body }),
    update: (tid: string, body: TeamPatch) => request<Team>("PATCH", `${A}/teams/${id(tid)}`, { body }),
    remove: (tid: string) => request<void>("DELETE", `${A}/teams/${id(tid)}`),
  },
  departments: {
    list: (o: Signal = {}) => request<{ data: Department[] }>("GET", `${A}/departments`, o),
    create: (body: DepartmentCreate) => request<Department>("POST", `${A}/departments`, { body }),
    update: (did: string, body: DepartmentPatch) => request<Department>("PATCH", `${A}/departments/${id(did)}`, { body }),
    remove: (did: string) => request<void>("DELETE", `${A}/departments/${id(did)}`),
    levels: (o: Signal = {}) => request<DepartmentLevels>("GET", `${A}/department-levels`, o),
    setLevels: (body: DepartmentLevels) => request<DepartmentLevels>("PUT", `${A}/department-levels`, { body }),
  },
  subscriptions: {
    list: (query: ListQuery & { channel?: SubscriptionChannel; status?: SubscriptionStatus; q?: string }, o: Signal = {}) =>
      request<SubscriptionList>("GET", `${A}/subscriptions`, { ...o, query }),
    create: (body: SubscriptionAdminCreate) => request<SubscriptionCreated>("POST", `${A}/subscriptions`, { body }),
    update: (sid: string, body: SubscriptionAdminPatch) => request<Subscription>("PATCH", `${A}/subscriptions/${id(sid)}`, { body }),
    remove: (sid: string) => request<void>("DELETE", `${A}/subscriptions/${id(sid)}`),
    test: (sid: string) => request<{ message: string }>("POST", `${A}/subscriptions/${id(sid)}/test`),
  },
  permissions: (o: Signal = {}) => request<{ data: PermissionInfo[] }>("GET", `${A}/permissions`, o),
  users: {
    list: (o: Signal = {}) => request<{ data: User[] }>("GET", `${A}/users`, o),
    create: (body: UserCreate) => request<User>("POST", `${A}/users`, { body }),
    update: (uid: string, body: UserPatch) => request<User>("PATCH", `${A}/users/${id(uid)}`, { body }),
    resetPassword: (uid: string, password: string) => request<void>("POST", `${A}/users/${id(uid)}/password`, { body: { password } }),
  },
  roles: {
    list: (o: Signal = {}) => request<{ data: Role[] }>("GET", `${A}/roles`, o),
    create: (body: RoleCreate) => request<Role>("POST", `${A}/roles`, { body }),
    update: (rid: string, body: RolePatch) => request<Role>("PATCH", `${A}/roles/${id(rid)}`, { body }),
    remove: (rid: string) => request<void>("DELETE", `${A}/roles/${id(rid)}`),
  },
  statusHistory: {
    list: (query: ListQuery & { component_id?: string; cause_type?: StatusChangeCause }, o: Signal = {}) =>
      request<Page<StatusChange>>("GET", `${A}/status-history`, { ...o, query }),
  },
  presence: (timezone: string, o: Signal = {}) =>
    request<PresenceSnapshot>("GET", `${A}/presence`, { ...o, query: { timezone } }),
  audit: {
    list: (
      query: ListQuery & { resource_type?: string; resource_id?: string; action?: string },
      o: Signal = {},
    ) => request<Page<AuditEvent>>("GET", `${A}/audit-events`, { ...o, query }),
  },
  features: {
    list: (o: Signal = {}) => request<{ data: Feature[] }>("GET", `${A}/features`, o),
    set: (key: Feature["key"], enabled: boolean) => request<Feature>("PUT", `${A}/features/${key}`, { body: { enabled } }),
  },
  suspicions: {
    list: (query: ListQuery & { status?: SuspicionStatus | "open"; queue?: "mine" | "general" | "all" }, o: Signal = {}) =>
      request<AdminSuspicionList>("GET", `${A}/suspicions`, { ...o, query }),
    get: (sid: string, o: Signal = {}) => request<SuspicionDetail>("GET", `${A}/suspicions/${id(sid)}`, o),
    assign: (sid: string, assigneeId: string) => request<SuspicionDetail>("PATCH", `${A}/suspicions/${id(sid)}`, { body: { assignee_id: assigneeId } }),
    publish: (sid: string) => request<SuspicionDetail>("POST", `${A}/suspicions/${id(sid)}/publish`),
    close: (sid: string, reason: string, message: string) =>
      request<SuspicionDetail>("POST", `${A}/suspicions/${id(sid)}/close`, { body: { reason, message } }),
    openIncident: (sid: string, body: IncidentCreate) => request<SuspicionDetail>("POST", `${A}/suspicions/${id(sid)}/incident`, { body }),
    message: (sid: string, kind: "internal" | "reporter", body: string, reporterId?: string) =>
      request<SuspicionDetail>("POST", `${A}/suspicions/${id(sid)}/messages`, {
        body: reporterId ? { kind, body, reporter_id: reporterId } : { kind, body },
      }),
  },
  reporters: {
    block: (rid: string, blocked: boolean) => request<SuspicionReporter>("PATCH", `${A}/reporters/${id(rid)}`, { body: { blocked } }),
  },
};
