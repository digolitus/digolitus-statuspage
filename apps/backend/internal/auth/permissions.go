package auth

// Permission is an action a role may grant. The catalogue is fixed in code
// because each permission maps to endpoints the backend enforces; roles
// choose subsets of it.
type Permission = string

// Permissions.
const (
	PermComponentsStatus     Permission = "components.status"
	PermComponentsManage     Permission = "components.manage"
	PermIncidentsManage      Permission = "incidents.manage"
	PermMaintenancesManage   Permission = "maintenances.manage"
	PermTemplatesManage      Permission = "templates.manage"
	PermAnnouncementsWrite   Permission = "announcements.write"
	PermAnnouncementsPublish Permission = "announcements.publish"
	PermAnnouncementsSetting Permission = "announcements.settings"
	PermThirdPartyManage     Permission = "third_party.manage"
	PermMonitoringManage     Permission = "monitoring.manage"
	PermTeamsManage          Permission = "teams.manage"
	PermSubscriptionsManage  Permission = "subscriptions.manage"
	PermAuditRead            Permission = "audit.read"
	PermPresenceRead         Permission = "presence.read"
	PermUsersManage          Permission = "users.manage"
	PermRolesManage          Permission = "roles.manage"
	PermBrandingManage       Permission = "branding.manage"
	PermSuspicionsManage     Permission = "suspicions.manage"
	PermFeaturesManage       Permission = "features.manage"
)

// PermissionInfo describes a permission for the admin UI.
type PermissionInfo struct {
	Key         string `json:"key"`
	Group       string `json:"group"`
	Label       string `json:"label"`
	Description string `json:"description"`
	// Sensitive permissions allow changing who can do what.
	Sensitive bool `json:"sensitive"`
	// Scopable permissions can be granted for the user's teams only (team
	// roles): they act on resources owned by a team.
	Scopable bool `json:"scopable"`
}

// Catalogue lists every permission in display order.
var Catalogue = []PermissionInfo{
	{PermComponentsStatus, "Componentes", "Alterar status de componentes", "Muda apenas o status declarado de componentes existentes.", false, true},
	{PermComponentsManage, "Componentes", "Gerenciar componentes e grupos", "Cria, edita, reorganiza e desativa componentes; cria, edita e exclui grupos.", false, true},
	{PermThirdPartyManage, "Componentes", "Gerenciar serviços de terceiros", "Cadastra provedores externos e escolhe os componentes exibidos.", false, false},
	{PermMonitoringManage, "Componentes", "Gerenciar monitoramento", "Cadastra integrações de monitoramento (UptimeRobot, Healthchecks.io) e liga monitores a componentes, o que muda o status exibido automaticamente.", false, false},
	{PermIncidentsManage, "Indisponibilidades", "Gerenciar incidentes", "Abre incidentes, publica atualizações e resolve.", false, true},
	{PermMaintenancesManage, "Indisponibilidades", "Gerenciar manutenções", "Programa, inicia, conclui e cancela manutenções.", false, true},
	{PermSuspicionsManage, "Indisponibilidades", "Triar suspeitas", "Analisa suspeitas reportadas por visitantes: publica, abre incidente, finaliza e conversa com quem reportou.", false, true},
	{PermTemplatesManage, "Indisponibilidades", "Gerenciar templates", "Cria e edita templates de incidente e de manutenção.", false, false},
	{PermAnnouncementsWrite, "Comunicados", "Escrever comunicados", "Cria e edita rascunhos e o texto de comunicados.", false, true},
	{PermAnnouncementsPublish, "Comunicados", "Publicar comunicados", "Publica, agenda, despublica, arquiva e restaura comunicados.", false, true},
	{PermAnnouncementsSetting, "Comunicados", "Gerenciar categorias", "Cria, edita, desativa e exclui categorias de comunicados.", false, false},
	{PermTeamsManage, "Administração", "Gerenciar times", "Cria e edita times responsáveis (tipo, documentação, suporte).", false, false},
	{PermSubscriptionsManage, "Administração", "Gerenciar inscrições", "Consulta e remove inscritos; cria integrações por webhook e Slack.", false, false},
	{PermBrandingManage, "Administração", "Gerenciar identidade visual", "Altera nome, textos, logotipo, favicon, fonte e cores da página pública e da tela de login.", false, false},
	{PermFeaturesManage, "Administração", "Ligar e desligar funcionalidades", "Liga e desliga funcionalidades opcionais, como Reportar suspeita.", false, false},
	{PermAuditRead, "Administração", "Consultar histórico operacional", "Lê o registro de alterações (auditoria).", false, false},
	{PermPresenceRead, "Administração", "Ver acessos em tempo real", "Vê quantos visitantes estão na página pública e quais usuários estão no admin agora.", false, false},
	{PermUsersManage, "Administração", "Gerenciar usuários", "Cria usuários, atribui papéis e times, redefine senhas e desativa contas.", true, false},
	{PermRolesManage, "Administração", "Gerenciar papéis", "Cria e edita papéis e suas permissões.", true, false},
}

var known = func() map[string]bool {
	m := make(map[string]bool, len(Catalogue))
	for _, p := range Catalogue {
		m[p.Key] = true
	}
	return m
}()

// Known reports whether p is in the catalogue.
func Known(p string) bool { return known[p] }

var scopable = func() map[string]bool {
	m := map[string]bool{}
	for _, p := range Catalogue {
		if p.Scopable {
			m[p.Key] = true
		}
	}
	return m
}()

// Role scopes.
const (
	ScopeGlobal = "global"
	ScopeTeam   = "team"
)
