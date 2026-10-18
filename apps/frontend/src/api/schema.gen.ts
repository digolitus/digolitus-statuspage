// Code generated from apps/backend/api/openapi.yaml. DO NOT EDIT.
// Regenerate with: make generate-types (go run ./cmd/openapi-ts in apps/backend).

export type AuditEvent = {
  action: string;
  /** Valores relevantes anteriores (`before`) e posteriores (`after`). */
  changes: Record<string, unknown>;
  id: number;
  occurred_at: Timestamp;
  request_id: string | null;
  resource_id: string;
  resource_type: "component" | "component_group" | "incident" | "maintenance" | "incident_template" | "maintenance_template" | "external_source";
  /** Canal de origem (ex.: admin_api). Não identifica uma pessoa. */
  source: string;
};

export type AuditEventList = {
  data: AuditEvent[];
  pagination: Pagination;
};

export type Component = {
  created_at: Timestamp;
  description: string;
  display_order: number;
  /** Grupo do componente; `null` quando não agrupado. */
  group_id: string | null;
  id: string;
  is_active: boolean;
  is_public: boolean;
  name: string;
  slug: string;
  status: ComponentStatus;
  updated_at: Timestamp;
};

export type ComponentCreate = {
  name: string;
  description?: string;
  display_order?: number;
  group_id?: string | null;
  is_public?: boolean;
  /** Único. Derivado do nome quando omitido. */
  slug?: string;
  status?: ComponentStatus;
};

export type ComponentGroup = {
  /** Componentes associados (inclusive ocultos e desativados). */
  component_count: number;
  created_at: Timestamp;
  description: string;
  display_order: number;
  id: string;
  name: string;
  updated_at: Timestamp;
};

export type ComponentGroupCreate = {
  /** Único (sem diferenciar maiúsculas). */
  name: string;
  description?: string;
  display_order?: number;
};

export type ComponentGroupList = {
  data: ComponentGroup[];
  pagination: Pagination;
};

export type ComponentGroupPatch = {
  description?: string;
  display_order?: number;
  name?: string;
};

export type ComponentList = {
  data: Component[];
  pagination: Pagination;
};

export type ComponentPatch = {
  description?: string;
  display_order?: number;
  /** `null` remove o componente do grupo. */
  group_id?: string | null;
  is_active?: boolean;
  is_public?: boolean;
  name?: string;
  slug?: string;
  status?: ComponentStatus;
};

export type ComponentRef = {
  id: string;
  name: string;
  slug: string;
};

/** Status declarado (ou efetivo, nas rotas públicas) de um componente. */
export type ComponentStatus = "operational" | "degraded_performance" | "partial_outage" | "major_outage" | "maintenance";

export type Error = {
  error: {
    /** Código estável para tratamento programático. */
    code: string;
    /** Mensagem legível (pt-BR). */
    message: string;
    details?: {
      field: string;
      message: string;
    }[];
    request_id?: string;
  };
};

export type ExpectedImpact = "none" | "degraded_performance" | "partial_outage" | "major_outage";

export type ExternalComponent = {
  description: string;
  /** Identificador no provedor. */
  external_id: string;
  group_name: string | null;
  id: string;
  /** Falso quando o provedor deixou de listar o componente. */
  is_present: boolean;
  is_selected: boolean;
  name: string;
  status: ExternalComponentStatus;
};

export type ExternalComponentList = {
  data: ExternalComponent[];
};

/** Status informado pelo provedor (`under_maintenance` vira `maintenance`); `unknown` quando não reconhecido ou desatualizado. */
export type ExternalComponentStatus = "operational" | "degraded_performance" | "partial_outage" | "major_outage" | "maintenance" | "unknown";

export type ExternalSelection = {
  component_ids: string[];
};

export type ExternalSource = {
  /** Normalizada para https://host. */
  base_url: string;
  component_count: number;
  created_at: Timestamp;
  display_order: number;
  id: string;
  /** Indicador geral do provedor. */
  indicator: "none" | "minor" | "major" | "critical" | "maintenance" | null;
  indicator_description: string;
  /** Inativos não são sincronizados nem publicados. */
  is_active: boolean;
  last_checked_at: NullableTimestamp;
  /** Erro da última tentativa, se falhou. */
  last_error: string | null;
  last_success_at: NullableTimestamp;
  name: string;
  /** Nome da página informado pelo provedor. */
  page_name: string;
  provider: "statuspage";
  selected_count: number;
  /** Última leitura bem-sucedida mais antiga que max(3 × intervalo, 10 min). */
  stale: boolean;
  updated_at: Timestamp;
};

export type ExternalSourceCreate = {
  /** URL pública da página (https). Ex.: https://www.githubstatus.com */
  url: string;
  display_order?: number;
  /** Padrão: nome informado pelo provedor. */
  name?: string;
};

export type ExternalSourceList = {
  data: ExternalSource[];
};

export type ExternalSourcePatch = {
  display_order?: number;
  is_active?: boolean;
  name?: string;
};

export type Health = {
  status: "ok" | "ready";
};

export type Incident = {
  components: ComponentRef[];
  created_at: Timestamp;
  id: string;
  impact: IncidentImpact;
  /** Notas operacionais; nunca expostas publicamente. */
  internal_notes: string;
  resolved_at: NullableTimestamp;
  started_at: Timestamp;
  status: IncidentStatus;
  template_id: string | null;
  title: string;
  updated_at: Timestamp;
  latest_update?: IncidentUpdate;
  /** Presente nas consultas individuais. */
  updates?: IncidentUpdate[];
};

export type IncidentCreate = {
  impact: IncidentImpact;
  /** Primeira mensagem pública. */
  message: string;
  title: string;
  component_ids?: UUIDList;
  internal_notes?: string;
  /** Início conhecido (RFC 3339 com offset, não futuro). Padrão: agora. */
  started_at?: string;
  status?: OpenIncidentStatus;
  template_id?: string;
};

/** Impacto: minor → degraded_performance, major → partial_outage, critical → major_outage. */
export type IncidentImpact = "minor" | "major" | "critical";

export type IncidentList = {
  data: Incident[];
  pagination: Pagination;
};

export type IncidentPatch = {
  impact?: IncidentImpact;
  internal_notes?: string;
  started_at?: string;
  title?: string;
};

export type IncidentResolve = {
  message: string;
};

/** `resolved` é terminal. */
export type IncidentStatus = "investigating" | "identified" | "monitoring" | "resolved";

export type IncidentTemplate = {
  created_at: Timestamp;
  description: string;
  /** Orientação para quem preenche; não é publicada. */
  guidance: string;
  id: string;
  impact: IncidentImpact;
  is_active: boolean;
  /** Mensagem sugerida. */
  message: string;
  name: string;
  status: IncidentStatus;
  /** Título sugerido. */
  title: string;
  updated_at: Timestamp;
};

export type IncidentTemplateCreate = {
  impact: IncidentImpact;
  message: string;
  /** Único (sem diferenciar maiúsculas). */
  name: string;
  title: string;
  description?: string;
  guidance?: string;
  is_active?: boolean;
  status?: IncidentStatus;
};

export type IncidentTemplateList = {
  data: IncidentTemplate[];
  pagination: Pagination;
};

export type IncidentTemplatePatch = {
  description?: string;
  guidance?: string;
  impact?: IncidentImpact;
  is_active?: boolean;
  message?: string;
  name?: string;
  status?: IncidentStatus;
  title?: string;
};

export type IncidentUpdate = {
  /** Componentes afetados definidos por esta atualização; `null` quando não foram alterados. */
  components: ComponentRef[] | null;
  created_at: Timestamp;
  id: string;
  message: string;
  status: IncidentStatus;
};

export type IncidentUpdateCreate = {
  message: string;
  status: IncidentStatus;
  component_ids?: UUIDList;
};

export type IncidentUpdateList = {
  data: IncidentUpdate[];
  pagination: Pagination;
};

/** RFC 3339 com offset (ex. `2026-11-20T02:00:00-03:00`) ou data e hora locais (`2026-11-20T02:00`) interpretadas no `timezone`. */
export type InputInstant = string;

export type Maintenance = {
  cancelled_at: NullableTimestamp;
  completed_at: NullableTimestamp;
  components: ComponentRef[];
  created_at: Timestamp;
  description: string;
  end_overdue: boolean;
  expected_impact: ExpectedImpact;
  id: string;
  internal_notes: string;
  scheduled_end: Timestamp;
  scheduled_start: Timestamp;
  start_overdue: boolean;
  started_at: NullableTimestamp;
  status: MaintenanceStatus;
  template_id: string | null;
  timezone: string;
  title: string;
  updated_at: Timestamp;
  latest_update?: MaintenanceUpdate;
  updates?: MaintenanceUpdate[];
};

export type MaintenanceCreate = {
  scheduled_end: InputInstant;
  scheduled_start: InputInstant;
  timezone: string;
  title: string;
  component_ids?: UUIDList;
  description?: string;
  expected_impact?: ExpectedImpact;
  internal_notes?: string;
  /** Mensagem pública inicial (opcional). */
  message?: string;
  template_id?: string;
};

export type MaintenanceList = {
  data: Maintenance[];
  pagination: Pagination;
};

export type MaintenanceMessageCreate = {
  message: string;
};

export type MaintenancePatch = {
  component_ids?: UUIDList;
  description?: string;
  expected_impact?: ExpectedImpact;
  internal_notes?: string;
  scheduled_end?: InputInstant;
  scheduled_start?: InputInstant;
  timezone?: string;
  title?: string;
};

/** Transições: scheduled → in_progress → completed; scheduled → cancelled. */
export type MaintenanceStatus = "scheduled" | "in_progress" | "completed" | "cancelled";

export type MaintenanceTemplate = {
  created_at: Timestamp;
  description: string;
  expected_impact: ExpectedImpact;
  guidance: string;
  id: string;
  is_active: boolean;
  message: string;
  name: string;
  title: string;
  updated_at: Timestamp;
};

export type MaintenanceTemplateCreate = {
  message: string;
  name: string;
  title: string;
  description?: string;
  expected_impact?: ExpectedImpact;
  guidance?: string;
  is_active?: boolean;
};

export type MaintenanceTemplateList = {
  data: MaintenanceTemplate[];
  pagination: Pagination;
};

export type MaintenanceTemplatePatch = {
  description?: string;
  expected_impact?: ExpectedImpact;
  guidance?: string;
  is_active?: boolean;
  message?: string;
  name?: string;
  title?: string;
};

export type MaintenanceTransition = {
  message?: string;
};

export type MaintenanceUpdate = {
  created_at: Timestamp;
  id: string;
  kind: "created" | "message" | "started" | "completed" | "cancelled";
  /** `null` em transições publicadas sem mensagem. */
  message: string | null;
  status: MaintenanceStatus;
};

export type MaintenanceUpdateList = {
  data: MaintenanceUpdate[];
  pagination: Pagination;
};

export type NullableTimestamp = string | null;

export type OpenIncidentStatus = "investigating" | "identified" | "monitoring";

export type OverallStatus = "operational" | "degraded_performance" | "partial_outage" | "major_outage" | "maintenance" | "unknown";

export type Pagination = {
  limit: number;
  offset: number;
  total: number;
};

export type PublicComponent = {
  description: string;
  display_order: number;
  group_id: string | null;
  id: string;
  name: string;
  slug: string;
  status: ComponentStatus;
  updated_at: Timestamp;
};

export type PublicComponentGroup = {
  affected_components: number;
  /** Componentes públicos do grupo. */
  component_count: number;
  components_under_maintenance: number;
  description: string;
  display_order: number;
  id: string;
  name: string;
  status: ComponentStatus;
};

export type PublicComponentList = {
  data: PublicComponent[];
  pagination: Pagination;
};

export type PublicIncident = {
  /** Apenas componentes públicos. */
  components: ComponentRef[];
  created_at: Timestamp;
  id: string;
  impact: IncidentImpact;
  resolved_at: NullableTimestamp;
  started_at: Timestamp;
  status: IncidentStatus;
  title: string;
  updated_at: Timestamp;
  latest_update?: IncidentUpdate;
  updates?: IncidentUpdate[];
};

export type PublicIncidentList = {
  data: PublicIncident[];
  pagination: Pagination;
};

export type PublicMaintenance = {
  cancelled_at: NullableTimestamp;
  completed_at: NullableTimestamp;
  /** Apenas componentes públicos. */
  components: ComponentRef[];
  created_at: Timestamp;
  description: string;
  end_overdue: boolean;
  expected_impact: ExpectedImpact;
  id: string;
  scheduled_end: Timestamp;
  scheduled_start: Timestamp;
  start_overdue: boolean;
  started_at: NullableTimestamp;
  status: MaintenanceStatus;
  timezone: string;
  title: string;
  updated_at: Timestamp;
  latest_update?: MaintenanceUpdate;
  updates?: MaintenanceUpdate[];
};

export type PublicMaintenanceList = {
  data: PublicMaintenance[];
  pagination: Pagination;
};

export type PublicThirdPartySource = {
  components: ({
    group_name: string | null;
    id: string;
    name: string;
    status: ExternalComponentStatus;
  })[];
  /** Incidentes não resolvidos e manutenções em andamento do provedor que afetam os componentes selecionados. */
  events: ({
    impact: string;
    kind: "incident" | "maintenance";
    name: string;
    status: string;
    url: string | null;
  })[];
  id: string;
  last_success_at: NullableTimestamp;
  name: string;
  /** Página de status original do provedor. */
  page_url: string;
  /** Dados desatualizados; status reportados como `unknown`. */
  stale: boolean;
  status: OverallStatus;
};

export type StatusSummary = {
  active_incidents: PublicIncident[];
  /** Todos os componentes públicos (agrupados ou não), na ordem de exibição. */
  components: PublicComponent[];
  generated_at: Timestamp;
  /** Grupos com ao menos um componente público, com status derivado. */
  groups: PublicComponentGroup[];
  maintenances: {
    in_progress: PublicMaintenance[];
    upcoming: PublicMaintenance[];
  };
  overall: {
    /** Componentes públicos com status pior que operational (exceto manutenção). */
    affected_components: number;
    /** Origem do status. `declared` = informado por operadores, sem verificação automática. */
    basis: "declared";
    components_under_maintenance: number;
    status: OverallStatus;
  };
  /**
   * Serviços de terceiros com componentes selecionados. Status informado pelos provedores; **não afeta**
   * `overall`. Dados desatualizados vêm com `stale: true` e status `unknown`.
   */
  third_party: PublicThirdPartySource[];
};

/** RFC 3339 em UTC. */
export type Timestamp = string;

export type UUIDList = string[];
