package app

import "github.com/digolitus/digolitus-statuspage/apps/backend/internal/auth"

// anyUser marks routes open to every authenticated, active user. Reading the
// operational data (components, incidents...) is shared by all roles so the
// admin screens and pickers work; changing it requires a permission.
var anyUser = []string{}

func perms(p ...string) []string { return p }

// adminPolicy is the single authorization table of the admin API: every
// admin route must be listed (TestEveryAdminRouteHasPolicy), and routes
// missing here are denied at runtime.
//
// PATCH /components/{id} is reachable with components.status, but the
// component handler then only accepts a status change unless the user also
// holds components.manage.
var adminPolicy = auth.Policy{
	"GET /api/v1/admin/permissions": anyUser,

	"GET /api/v1/admin/components":               anyUser,
	"GET /api/v1/admin/components/{id}":          anyUser,
	"POST /api/v1/admin/components":              perms(auth.PermComponentsManage),
	"PATCH /api/v1/admin/components/{id}":        perms(auth.PermComponentsManage, auth.PermComponentsStatus),
	"GET /api/v1/admin/component-groups":         anyUser,
	"GET /api/v1/admin/component-groups/{id}":    anyUser,
	"POST /api/v1/admin/component-groups":        perms(auth.PermComponentsManage),
	"PATCH /api/v1/admin/component-groups/{id}":  perms(auth.PermComponentsManage),
	"DELETE /api/v1/admin/component-groups/{id}": perms(auth.PermComponentsManage),

	"GET /api/v1/admin/external-sources":                 anyUser,
	"GET /api/v1/admin/external-sources/{id}":            anyUser,
	"GET /api/v1/admin/external-sources/{id}/components": anyUser,
	"POST /api/v1/admin/external-sources":                perms(auth.PermThirdPartyManage),
	"PATCH /api/v1/admin/external-sources/{id}":          perms(auth.PermThirdPartyManage),
	"DELETE /api/v1/admin/external-sources/{id}":         perms(auth.PermThirdPartyManage),
	"POST /api/v1/admin/external-sources/{id}/sync":      perms(auth.PermThirdPartyManage),
	"PUT /api/v1/admin/external-sources/{id}/selection":  perms(auth.PermThirdPartyManage),

	"GET /api/v1/admin/incidents":               anyUser,
	"GET /api/v1/admin/incidents/{id}":          anyUser,
	"GET /api/v1/admin/incidents/{id}/updates":  anyUser,
	"POST /api/v1/admin/incidents":              perms(auth.PermIncidentsManage),
	"PATCH /api/v1/admin/incidents/{id}":        perms(auth.PermIncidentsManage),
	"POST /api/v1/admin/incidents/{id}/updates": perms(auth.PermIncidentsManage),
	"POST /api/v1/admin/incidents/{id}/resolve": perms(auth.PermIncidentsManage),

	"GET /api/v1/admin/maintenances":                anyUser,
	"GET /api/v1/admin/maintenances/{id}":           anyUser,
	"GET /api/v1/admin/maintenances/{id}/updates":   anyUser,
	"POST /api/v1/admin/maintenances":               perms(auth.PermMaintenancesManage),
	"PATCH /api/v1/admin/maintenances/{id}":         perms(auth.PermMaintenancesManage),
	"POST /api/v1/admin/maintenances/{id}/updates":  perms(auth.PermMaintenancesManage),
	"POST /api/v1/admin/maintenances/{id}/start":    perms(auth.PermMaintenancesManage),
	"POST /api/v1/admin/maintenances/{id}/complete": perms(auth.PermMaintenancesManage),
	"POST /api/v1/admin/maintenances/{id}/cancel":   perms(auth.PermMaintenancesManage),

	"GET /api/v1/admin/incident-templates":           anyUser,
	"GET /api/v1/admin/incident-templates/{id}":      anyUser,
	"POST /api/v1/admin/incident-templates":          perms(auth.PermTemplatesManage),
	"PATCH /api/v1/admin/incident-templates/{id}":    perms(auth.PermTemplatesManage),
	"GET /api/v1/admin/maintenance-templates":        anyUser,
	"GET /api/v1/admin/maintenance-templates/{id}":   anyUser,
	"POST /api/v1/admin/maintenance-templates":       perms(auth.PermTemplatesManage),
	"PATCH /api/v1/admin/maintenance-templates/{id}": perms(auth.PermTemplatesManage),

	"GET /api/v1/admin/announcements":                   anyUser,
	"GET /api/v1/admin/announcements/{id}":              anyUser,
	"POST /api/v1/admin/announcements":                  perms(auth.PermAnnouncementsWrite),
	"PATCH /api/v1/admin/announcements/{id}":            perms(auth.PermAnnouncementsWrite),
	"DELETE /api/v1/admin/announcements/{id}":           perms(auth.PermAnnouncementsWrite),
	"POST /api/v1/admin/announcements/{id}/publish":     perms(auth.PermAnnouncementsPublish),
	"POST /api/v1/admin/announcements/{id}/unpublish":   perms(auth.PermAnnouncementsPublish),
	"POST /api/v1/admin/announcements/{id}/archive":     perms(auth.PermAnnouncementsPublish),
	"POST /api/v1/admin/announcements/{id}/restore":     perms(auth.PermAnnouncementsPublish),
	"GET /api/v1/admin/announcement-categories":         anyUser,
	"GET /api/v1/admin/announcement-categories/{id}":    anyUser,
	"POST /api/v1/admin/announcement-categories":        perms(auth.PermAnnouncementsSetting),
	"PATCH /api/v1/admin/announcement-categories/{id}":  perms(auth.PermAnnouncementsSetting),
	"DELETE /api/v1/admin/announcement-categories/{id}": perms(auth.PermAnnouncementsSetting),
	"GET /api/v1/admin/teams":                           anyUser,
	"GET /api/v1/admin/teams/{id}":                      anyUser,
	"POST /api/v1/admin/teams":                          perms(auth.PermTeamsManage),
	"PATCH /api/v1/admin/teams/{id}":                    perms(auth.PermTeamsManage),
	"DELETE /api/v1/admin/teams/{id}":                   perms(auth.PermTeamsManage),

	"GET /api/v1/admin/subscriptions":            perms(auth.PermSubscriptionsManage),
	"POST /api/v1/admin/subscriptions":           perms(auth.PermSubscriptionsManage),
	"GET /api/v1/admin/subscriptions/{id}":       perms(auth.PermSubscriptionsManage),
	"PATCH /api/v1/admin/subscriptions/{id}":     perms(auth.PermSubscriptionsManage),
	"DELETE /api/v1/admin/subscriptions/{id}":    perms(auth.PermSubscriptionsManage),
	"POST /api/v1/admin/subscriptions/{id}/test": perms(auth.PermSubscriptionsManage),

	"GET /api/v1/admin/audit-events": perms(auth.PermAuditRead),

	"GET /api/v1/admin/users":                perms(auth.PermUsersManage),
	"GET /api/v1/admin/users/{id}":           perms(auth.PermUsersManage),
	"POST /api/v1/admin/users":               perms(auth.PermUsersManage),
	"PATCH /api/v1/admin/users/{id}":         perms(auth.PermUsersManage),
	"POST /api/v1/admin/users/{id}/password": perms(auth.PermUsersManage),
	"GET /api/v1/admin/roles":                perms(auth.PermRolesManage, auth.PermUsersManage),
	"GET /api/v1/admin/roles/{id}":           perms(auth.PermRolesManage, auth.PermUsersManage),
	"POST /api/v1/admin/roles":               perms(auth.PermRolesManage),
	"PATCH /api/v1/admin/roles/{id}":         perms(auth.PermRolesManage),
	"DELETE /api/v1/admin/roles/{id}":        perms(auth.PermRolesManage),
}
