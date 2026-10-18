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
export type AnnouncementTemplate = S.AnnouncementTemplate;
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
export type RoleScope = S.RoleScope;

// Subscriptions (notifications).
export type SubscriptionChannel = S.SubscriptionChannel;
export type SubscriptionStatus = S.SubscriptionStatus;
export type SubscriptionOptions = S.SubscriptionOptions;
export type SubscribeInput = S.SubscribeInput;
export type SubscriptionManageView = S.SubscriptionManageView;
export type Subscription = S.Subscription;
export type SubscriptionList = S.SubscriptionList;
export type SubscriptionCreated = S.SubscriptionCreated;
export type SubscriptionAdminCreate = S.SubscriptionAdminCreate;
export type SubscriptionAdminPatch = S.SubscriptionAdminPatch;

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
export type AnnouncementTemplateCreate = S.AnnouncementTemplateCreate;
export type AnnouncementTemplatePatch = S.AnnouncementTemplatePatch;
export type ExternalSourceCreate = S.ExternalSourceCreate;
export type ExternalSourcePatch = S.ExternalSourcePatch;
export type AnnouncementCreate = S.AnnouncementCreate;
export type AnnouncementPatch = S.AnnouncementPatch;
export type AnnouncementCategoryCreate = S.AnnouncementCategoryCreate;
export type AnnouncementCategoryPatch = S.AnnouncementCategoryPatch;
export type TeamCreate = S.TeamCreate;
export type TeamPatch = S.TeamPatch;

// Authentication and access control.
export type PermissionKey = S.PermissionKey;
export type PermissionInfo = S.PermissionInfo;
export type Me = S.Me;
export type User = S.User;
export type Role = S.Role;
export type UserCreate = S.UserCreate;
export type UserPatch = S.UserPatch;
export type RoleCreate = S.RoleCreate;
export type RolePatch = S.RolePatch;
export type SetupInput = S.SetupInput;
