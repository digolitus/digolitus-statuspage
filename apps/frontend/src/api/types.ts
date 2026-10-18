// Friendly names for the types generated from the API contract
// (schema.gen.ts, produced from apps/backend/api/openapi.yaml). Do not
// declare API shapes by hand here: change the contract and regenerate.
import type * as S from "./schema.gen";

export type ComponentStatus = S.ComponentStatus;
export type OverallStatus = S.OverallStatus;
export type IncidentStatus = S.IncidentStatus;
export type OpenIncidentStatus = S.OpenIncidentStatus;
export type IncidentImpact = S.IncidentImpact;
export type MaintenanceStatus = S.MaintenanceStatus;
export type ExpectedImpact = S.ExpectedImpact;
export type MaintenanceUpdateKind = S.MaintenanceUpdate["kind"];

export type Pagination = S.Pagination;

/** Generic paginated envelope (every *List schema has this shape). */
export interface Page<T> {
  data: T[];
  pagination: Pagination;
}

export type ComponentRef = S.ComponentRef;
export type Component = S.Component;
export type ComponentGroup = S.ComponentGroup;
export type PublicComponent = S.PublicComponent;
export type PublicComponentGroup = S.PublicComponentGroup;
export type IncidentUpdate = S.IncidentUpdate;
export type PublicIncident = S.PublicIncident;
export type Incident = S.Incident;
export type MaintenanceUpdate = S.MaintenanceUpdate;
export type PublicMaintenance = S.PublicMaintenance;
export type Maintenance = S.Maintenance;
export type IncidentTemplate = S.IncidentTemplate;
export type MaintenanceTemplate = S.MaintenanceTemplate;
export type AuditEvent = S.AuditEvent;
export type StatusSummary = S.StatusSummary;
export type PublicThirdPartySource = S.PublicThirdPartySource;
export type ExternalSource = S.ExternalSource;
export type ExternalComponent = S.ExternalComponent;
export type ExternalComponentStatus = S.ExternalComponentStatus;

export type AnnouncementState = S.AnnouncementState;
export type AnnouncementImportance = S.AnnouncementImportance;
export type CategoryColor = S.CategoryColor;
export type Announcement = S.Announcement;
export type PublicAnnouncement = S.PublicAnnouncement;
export type AnnouncementCategory = S.AnnouncementCategory;
export type PublicAnnouncementCategory = S.PublicAnnouncementCategory;
export type Team = S.Team;

export type ErrorBody = S.Error;
export type FieldError = NonNullable<S.Error["error"]["details"]>[number];

// Request bodies.
export type ComponentCreate = S.ComponentCreate;
export type ComponentPatch = S.ComponentPatch;
export type ComponentGroupCreate = S.ComponentGroupCreate;
export type ComponentGroupPatch = S.ComponentGroupPatch;
export type IncidentCreate = S.IncidentCreate;
export type IncidentPatch = S.IncidentPatch;
export type IncidentUpdateCreate = S.IncidentUpdateCreate;
export type MaintenanceCreate = S.MaintenanceCreate;
export type MaintenancePatch = S.MaintenancePatch;
export type IncidentTemplateCreate = S.IncidentTemplateCreate;
export type IncidentTemplatePatch = S.IncidentTemplatePatch;
export type MaintenanceTemplateCreate = S.MaintenanceTemplateCreate;
export type MaintenanceTemplatePatch = S.MaintenanceTemplatePatch;
export type ExternalSourceCreate = S.ExternalSourceCreate;
export type ExternalSourcePatch = S.ExternalSourcePatch;
export type AnnouncementCreate = S.AnnouncementCreate;
export type AnnouncementPatch = S.AnnouncementPatch;
export type AnnouncementCategoryCreate = S.AnnouncementCategoryCreate;
export type AnnouncementCategoryPatch = S.AnnouncementCategoryPatch;
export type TeamCreate = S.TeamCreate;
export type TeamPatch = S.TeamPatch;
