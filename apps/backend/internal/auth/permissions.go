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
	PermAuditRead            Permission = "audit.read"
	PermUsersManage          Permission = "users.manage"
	PermRolesManage          Permission = "roles.manage"
)

// PermissionInfo describes a permission for the admin UI.
type PermissionInfo struct {
	Key         string `json:"key"`
	Group       string `json:"group"`
	Label       string `json:"label"`
	Description string `json:"description"`
	// Sensitive permissions allow changing who can do what.
	Sensitive bool `json:"sensitive"`
}

// Catalogue lists every permission in display order.
var Catalogue = []PermissionInfo{
	{PermComponentsStatus, "Componentes", "Alterar status de componentes", "Muda apenas o status declarado de componentes existentes.", false},
	{PermComponentsManage, "Componentes", "Gerenciar componentes e grupos", "Cria, edita, reorganiza e desativa componentes; cria, edita e exclui grupos.", false},
	{PermThirdPartyManage, "Componentes", "Gerenciar serviços de terceiros", "Cadastra provedores externos e escolhe os componentes exibidos.", false},
	{PermIncidentsManage, "Indisponibilidades", "Gerenciar incidentes", "Abre incidentes, publica atualizações e resolve.", false},
	{PermMaintenancesManage, "Indisponibilidades", "Gerenciar manutenções", "Programa, inicia, conclui e cancela manutenções.", false},
	{PermTemplatesManage, "Indisponibilidades", "Gerenciar templates", "Cria e edita templates de incidente e de manutenção.", false},
	{PermAnnouncementsWrite, "Comunicados", "Escrever comunicados", "Cria e edita rascunhos e o texto de comunicados.", false},
	{PermAnnouncementsPublish, "Comunicados", "Publicar comunicados", "Publica, agenda, despublica, arquiva e restaura comunicados.", false},
	{PermAnnouncementsSetting, "Comunicados", "Gerenciar categorias e times", "Cria, edita, desativa e exclui categorias e times de comunicados.", false},
	{PermAuditRead, "Administração", "Consultar histórico operacional", "Lê o registro de alterações (auditoria).", false},
	{PermUsersManage, "Administração", "Gerenciar usuários", "Cria usuários, atribui papéis, redefine senhas e desativa contas.", true},
	{PermRolesManage, "Administração", "Gerenciar papéis", "Cria e edita papéis e suas permissões.", true},
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
