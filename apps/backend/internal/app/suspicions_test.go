package app

import (
	"context"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/auth"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/suspicion"
)

var reporterLinkToken = regexp.MustCompile(`/suspeitas/entrar\?token=([A-Za-z0-9_-]+)`)

// enableSuspicions turns the feature on as the administrator.
func enableSuspicions(t *testing.T, h *harness) {
	t.Helper()
	f := mustStatus(t, h.call(http.MethodPut, "/api/v1/admin/features/suspicions", map[string]any{"enabled": true}), http.StatusOK).json(t)
	if f["enabled"] != true || f["updated_by"] != "Admin" {
		t.Fatalf("feature: %v", f)
	}
}

// reporterSession signs in a reporter through the e-mail link flow.
func reporterSession(t *testing.T, h *harness, email, name string) *harness {
	t.Helper()
	anon := h.anonymous()
	h.mailer.take()
	mustStatus(t, anon.call(http.MethodPost, "/api/v1/reporter/login-link", map[string]any{"email": email, "name": name}), http.StatusAccepted)
	mails := h.mailer.take()
	if len(mails) != 1 || mails[0].To != strings.ToLower(email) {
		t.Fatalf("link mail: %+v", mails)
	}
	m := reporterLinkToken.FindStringSubmatch(mails[0].Text)
	if m == nil || !strings.Contains(mails[0].Text, publicBaseURL) {
		t.Fatalf("no link in %q", mails[0].Text)
	}
	r := mustStatus(t, anon.call(http.MethodPost, "/api/v1/reporter/login", map[string]any{"token": m[1]}), http.StatusOK)
	var cookie string
	for _, c := range (&http.Response{Header: r.header}).Cookies() {
		if c.Name == suspicion.CookieName {
			if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteStrictMode {
				t.Fatalf("reporter cookie flags: %+v", c)
			}
			cookie = c.Value
		}
	}
	if cookie == "" {
		t.Fatal("no reporter cookie")
	}
	// The link is single use.
	mustStatus(t, anon.call(http.MethodPost, "/api/v1/reporter/login", map[string]any{"token": m[1]}), http.StatusUnprocessableEntity)
	c := *anon
	c.reporter = cookie
	return &c
}

func reportBody(componentID string) map[string]any {
	return map[string]any{"component_id": componentID, "symptom": "slow", "frequency": "intermittent", "reach": "many_places",
		"noticed": "last_30m", "location": "Escritório SP", "error_ref": "HTTP 504", "description": "Builds parados na fila."}
}

func (h *harness) runSuspicionWorker() {
	h.app.Suspicions.RunOnce(context.Background(), slog.New(slog.DiscardHandler))
}

func TestSuspicionsFeatureSwitch(t *testing.T) {
	h := newHarness(t)
	anon := h.anonymous()
	// Off until an administrator turns it on: routes answer 404.
	if f := mustStatus(t, anon.call(http.MethodGet, "/api/v1/features", nil), http.StatusOK).json(t); f["suspicions"] != false {
		t.Fatalf("features: %v", f)
	}
	if code := mustStatus(t, anon.call(http.MethodGet, "/api/v1/suspicions", nil), http.StatusNotFound).errorCode(t); code != "feature_disabled" {
		t.Fatalf("code = %s", code)
	}
	mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/suspicions", nil), http.StatusNotFound)

	// Only features.manage switches it.
	viewer := h.userWith("Leitor")
	mustStatus(t, viewer.call(http.MethodPut, "/api/v1/admin/features/suspicions", map[string]any{"enabled": true}), http.StatusForbidden)
	enableSuspicions(t, h)
	if f := mustStatus(t, anon.call(http.MethodGet, "/api/v1/features", nil), http.StatusOK).json(t); f["suspicions"] != true {
		t.Fatalf("features: %v", f)
	}
	mustStatus(t, anon.call(http.MethodGet, "/api/v1/suspicions", nil), http.StatusOK)
	list := mustStatus(t, viewer.call(http.MethodGet, "/api/v1/admin/features", nil), http.StatusOK).json(t)
	if f := list["data"].([]any)[0].(map[string]any); f["key"] != "suspicions" || f["enabled"] != true || f["locked"] != false {
		t.Fatalf("list: %v", list)
	}
	audit := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/audit-events?resource_type=feature_flag", nil), http.StatusOK).json(t)
	if total(audit) != 1 {
		t.Fatalf("audit: %v", audit)
	}

	// Locked by the deployment: off whatever the switch says.
	l := newHarnessWith(t, func(o *Options) { o.SuspicionsLocked = true })
	if code := mustStatus(t, l.call(http.MethodPut, "/api/v1/admin/features/suspicions", map[string]any{"enabled": true}), http.StatusConflict).errorCode(t); code != "feature_locked" {
		t.Fatalf("code = %s", code)
	}
	mustStatus(t, l.anonymous().call(http.MethodGet, "/api/v1/suspicions", nil), http.StatusNotFound)
}

func TestSuspicionLifecycle(t *testing.T) {
	h := newHarness(t)
	enableSuspicions(t, h)
	team := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/teams", map[string]any{"name": "Plataforma CI"}), http.StatusCreated).json(t)
	group := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/component-groups", map[string]any{"name": "Jenkins", "team_id": str(team, "id")}), http.StatusCreated).json(t)
	agents := h.createComponent("Agentes de build", map[string]any{"group_id": str(group, "id")})
	hidden := h.createComponent("Interno", map[string]any{"is_public": false})

	if p := mustStatus(t, h.anonymous().call(http.MethodGet, "/api/v1/reporter/providers", nil), http.StatusOK).json(t); p["email"] != true || p["google"] != false {
		t.Fatalf("providers: %v", p)
	}
	// Reporting requires a reporter session; an admin session is not one.
	mustStatus(t, h.call(http.MethodPost, "/api/v1/reporter/suspicions", reportBody(str(agents, "id"))), http.StatusUnauthorized)

	ana := reporterSession(t, h, "Ana.Souza@Empresa.com", "Ana Souza")
	bruno := reporterSession(t, h, "bruno@empresa.com", "")
	me := mustStatus(t, ana.call(http.MethodGet, "/api/v1/reporter/me", nil), http.StatusOK).json(t)
	if me["name"] != "Ana Souza" || me["email"] != "ana.souza@empresa.com" {
		t.Fatalf("me: %v", me)
	}
	// Validation and non-public components.
	bad := reportBody(str(agents, "id"))
	bad["noticed"] = "other"
	mustStatus(t, ana.call(http.MethodPost, "/api/v1/reporter/suspicions", bad), http.StatusUnprocessableEntity)
	mustStatus(t, ana.call(http.MethodPost, "/api/v1/reporter/suspicions", reportBody(str(hidden, "id"))), http.StatusUnprocessableEntity)

	first := mustStatus(t, ana.call(http.MethodPost, "/api/v1/reporter/suspicions", reportBody(str(agents, "id"))), http.StatusCreated).json(t)
	if first["status"] != "pending" || first["report_count"].(float64) != 1 || obj(first, "component")["group"] != "Jenkins" {
		t.Fatalf("first: %v", first)
	}
	id := str(first, "id")
	// Same component: grouped into the same suspicion; once per reporter.
	h.clock.Advance(40 * time.Minute)
	other := reportBody(str(agents, "id"))
	other["symptom"] = "error"
	second := mustStatus(t, bruno.call(http.MethodPost, "/api/v1/reporter/suspicions", other), http.StatusCreated).json(t)
	if str(second, "id") != id || second["report_count"].(float64) != 2 {
		t.Fatalf("not grouped: %v", second)
	}
	if code := mustStatus(t, ana.call(http.MethodPost, "/api/v1/reporter/suspicions", reportBody(str(agents, "id"))), http.StatusConflict).errorCode(t); code != "already_reported" {
		t.Fatalf("code = %s", code)
	}

	// Pending suspicions are not public.
	anon := h.anonymous()
	if pub := mustStatus(t, anon.call(http.MethodGet, "/api/v1/suspicions", nil), http.StatusOK).json(t); len(list(pub, "data")) != 0 {
		t.Fatalf("public before publishing: %v", pub)
	}
	queue := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/suspicions?status=open", nil), http.StatusOK).json(t)
	if total(queue) != 1 || obj(queue, "counts")["pending"].(float64) != 1 {
		t.Fatalf("queue: %v", queue)
	}
	row := list(queue, "data")[0].(map[string]any)
	if obj(row, "team")["name"] != "Plataforma CI" || row["report_count"].(float64) != 2 || obj(row, "summary")["symptom"] != "slow" {
		t.Fatalf("row: %v", row)
	}

	// Publish: visible publicly, without who reported or what they wrote.
	pubd := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/suspicions/"+id+"/publish", nil), http.StatusOK).json(t)
	if pubd["status"] != "published" || obj(pubd, "published_by")["name"] != "Admin" {
		t.Fatalf("published: %v", pubd)
	}
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/suspicions/"+id+"/publish", nil), http.StatusConflict)
	pub := mustStatus(t, anon.call(http.MethodGet, "/api/v1/suspicions", nil), http.StatusOK)
	if strings.Contains(string(pub.body), "empresa.com") || strings.Contains(string(pub.body), "Builds parados") {
		t.Fatalf("public list leaks reporter data: %s", pub.body)
	}
	item := list(pub.json(t), "data")[0].(map[string]any)
	if item["report_count"].(float64) != 2 || item["recent_count"].(float64) != 2 || len(list(item, "recent")) != 6 {
		t.Fatalf("public item: %v", item)
	}

	// Internal note and message to the first reporter (default).
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/suspicions/"+id+"/messages", map[string]any{"kind": "internal", "body": "Autoscaler parado."}), http.StatusCreated)
	det := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/suspicions/"+id+"/messages", map[string]any{"kind": "reporter", "body": "Acontece só com label docker?"}), http.StatusCreated).json(t)
	msgs := list(det, "messages")
	if len(msgs) != 2 || obj(msgs[1].(map[string]any), "reporter")["name"] != "Ana Souza" || len(list(det, "reports")) != 2 {
		t.Fatalf("detail: %v", det)
	}
	if d := obj(det, "distribution"); len(list(d, "symptom")) != 2 {
		t.Fatalf("distribution: %v", d)
	}
	h.mailer.take()
	h.runSuspicionWorker()
	mails := h.mailer.take()
	if len(mails) != 1 || mails[0].To != "ana.souza@empresa.com" || !strings.Contains(mails[0].Text, "label docker") || !strings.Contains(mails[0].Subject, "#S-0001") {
		t.Fatalf("mails: %+v", mails)
	}
	h.runSuspicionWorker()
	if again := h.mailer.take(); len(again) != 0 {
		t.Fatalf("sent twice: %+v", again)
	}

	// The reporter sees the team's message (not who wrote it), never internal notes, and replies.
	mine := mustStatus(t, ana.call(http.MethodGet, "/api/v1/reporter/suspicions", nil), http.StatusOK)
	if strings.Contains(string(mine.body), "Autoscaler") {
		t.Fatalf("internal note leaked: %s", mine.body)
	}
	mm := list(list(mine.json(t), "data")[0].(map[string]any), "messages")
	if len(mm) != 1 || mm[0].(map[string]any)["author_name"] != "Plataforma CI" {
		t.Fatalf("my messages: %v", mm)
	}
	mustStatus(t, ana.call(http.MethodPost, "/api/v1/reporter/suspicions/"+id+"/messages", map[string]any{"body": "Sim, só nesses."}), http.StatusCreated)
	if bm := list(list(mustStatus(t, bruno.call(http.MethodGet, "/api/v1/reporter/suspicions", nil), http.StatusOK).json(t), "data")[0].(map[string]any), "messages"); len(bm) != 0 {
		t.Fatalf("bruno sees ana's conversation: %v", bm)
	}

	// Close: reason required; every reporter is told.
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/suspicions/"+id+"/close", map[string]any{"reason": "maybe", "message": "x"}, invalidRequest()), http.StatusUnprocessableEntity)
	closed := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/suspicions/"+id+"/close", map[string]any{"reason": "no_evidence", "message": "Pico de builds, já normalizado."}), http.StatusOK).json(t)
	if closed["status"] != "closed" || closed["close_reason"] != "no_evidence" {
		t.Fatalf("closed: %v", closed)
	}
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/suspicions/"+id+"/incident", map[string]any{"title": "x", "message": "y", "impact": "partial_outage"}), http.StatusConflict)
	h.runSuspicionWorker()
	if mails := h.mailer.take(); len(mails) != 2 {
		t.Fatalf("close mails: %+v", mails)
	}
	if pub := mustStatus(t, anon.call(http.MethodGet, "/api/v1/suspicions", nil), http.StatusOK).json(t); len(list(pub, "data")) != 0 {
		t.Fatalf("closed still public: %v", pub)
	}

	// After closing, a new report starts a new suspicion.
	h.clock.Advance(2 * time.Hour)
	again := mustStatus(t, ana.call(http.MethodPost, "/api/v1/reporter/suspicions", reportBody(str(agents, "id"))), http.StatusCreated).json(t)
	if str(again, "id") == id || again["number"].(float64) != 2 {
		t.Fatalf("again: %v", again)
	}

	// Open an incident from it: linked, defaults to the reported component;
	// reports while the incident is open join it; resolution is announced once.
	det = mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/suspicions/"+str(again, "id")+"/incident", map[string]any{
		"title": "Lentidão nos agentes", "message": "Investigando.", "impact": "degraded_performance"}), http.StatusCreated).json(t)
	inc := obj(det, "incident")
	if det["status"] != "incident" || inc["status"] != "investigating" {
		t.Fatalf("incident: %v", det)
	}
	full := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/incidents/"+str(inc, "id"), nil), http.StatusOK).json(t)
	if comps := list(full, "components"); len(comps) != 1 || comps[0].(map[string]any)["id"] != str(agents, "id") {
		t.Fatalf("incident components: %v", full)
	}
	joined := mustStatus(t, bruno.call(http.MethodPost, "/api/v1/reporter/suspicions", reportBody(str(agents, "id"))), http.StatusCreated).json(t)
	if str(joined, "id") != str(again, "id") || joined["status"] != "incident" {
		t.Fatalf("joined: %v", joined)
	}
	h.runSuspicionWorker()
	h.mailer.take()
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/incidents/"+str(inc, "id")+"/resolve", map[string]any{"message": "Resolvido."}), http.StatusOK)
	h.runSuspicionWorker()
	h.runSuspicionWorker()
	if mails := h.mailer.take(); len(mails) != 2 || !strings.Contains(mails[0].Subject, "Incidente resolvido") {
		t.Fatalf("resolved mails: %+v", mails)
	}

	// Logout ends the session.
	mustStatus(t, ana.call(http.MethodPost, "/api/v1/reporter/logout", nil), http.StatusNoContent)
	mustStatus(t, ana.call(http.MethodGet, "/api/v1/reporter/me", nil), http.StatusUnauthorized)
}

func TestSuspicionScopeAndAbuse(t *testing.T) {
	h := newHarness(t)
	enableSuspicions(t, h)
	t1 := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/teams", map[string]any{"name": "Squad A"}), http.StatusCreated).json(t)
	own := h.createComponent("API A", map[string]any{"team_id": str(t1, "id")})
	orphan := h.createComponent("Sem dono", nil)
	extra := []map[string]any{}
	for _, n := range []string{"C1", "C2", "C3", "C4", "C5"} {
		extra = append(extra, h.createComponent(n, nil))
	}

	role := mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/roles", map[string]any{
		"name": "Triagem do time", "scope": "team", "permissions": []string{auth.PermSuspicionsManage}}), http.StatusCreated).json(t)
	mustStatus(t, h.call(http.MethodPost, "/api/v1/admin/users", map[string]any{
		"email": "squad.a@example.com", "name": "Squad A", "password": "a long enough password",
		"role_ids": []string{str(role, "id")}, "team_ids": []string{str(t1, "id")}}), http.StatusCreated)
	squad := h.login("squad.a@example.com", "a long enough password")
	squad.choosePassword("a long enough password", "squad a own password")
	nobody := h.userWith("Sem permissão")

	rep := reporterSession(t, h, "carla@empresa.com", "Carla")
	a := mustStatus(t, rep.call(http.MethodPost, "/api/v1/reporter/suspicions", reportBody(str(own, "id"))), http.StatusCreated).json(t)
	b := mustStatus(t, rep.call(http.MethodPost, "/api/v1/reporter/suspicions", reportBody(str(orphan, "id"))), http.StatusCreated).json(t)

	mustStatus(t, nobody.call(http.MethodGet, "/api/v1/admin/suspicions", nil), http.StatusForbidden)
	// The team user only sees and acts on their team's components.
	q := mustStatus(t, squad.call(http.MethodGet, "/api/v1/admin/suspicions?queue=all", nil), http.StatusOK).json(t)
	if total(q) != 1 || list(q, "data")[0].(map[string]any)["id"] != str(a, "id") {
		t.Fatalf("team queue: %v", q)
	}
	mustStatus(t, squad.call(http.MethodGet, "/api/v1/admin/suspicions/"+str(b, "id"), nil), http.StatusForbidden)
	mustStatus(t, squad.call(http.MethodPost, "/api/v1/admin/suspicions/"+str(b, "id")+"/publish", nil), http.StatusForbidden)
	mustStatus(t, squad.call(http.MethodPost, "/api/v1/admin/suspicions/"+str(a, "id")+"/publish", nil), http.StatusOK)
	// Opening an incident also needs incidents.manage.
	mustStatus(t, squad.call(http.MethodPost, "/api/v1/admin/suspicions/"+str(a, "id")+"/incident", map[string]any{
		"title": "Falha", "message": "Investigando.", "impact": "partial_outage"}), http.StatusForbidden)
	// General queue: components without a team.
	g := mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/suspicions?queue=general", nil), http.StatusOK).json(t)
	if total(g) != 1 || list(g, "data")[0].(map[string]any)["id"] != str(b, "id") {
		t.Fatalf("general queue: %v", g)
	}
	// Assignee.
	d := mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/suspicions/"+str(b, "id"), map[string]any{"assignee_id": str(obj(mustStatus(t, h.call(http.MethodGet, "/api/v1/auth/me", nil), http.StatusOK).json(t), "user"), "id")}), http.StatusOK).json(t)
	if obj(d, "assignee")["name"] != "Admin" {
		t.Fatalf("assignee: %v", d)
	}

	// Rate limit: 5 reports per hour.
	for i := range 3 {
		mustStatus(t, rep.call(http.MethodPost, "/api/v1/reporter/suspicions", reportBody(str(extra[i], "id"))), http.StatusCreated)
	}
	if code := mustStatus(t, rep.call(http.MethodPost, "/api/v1/reporter/suspicions", reportBody(str(extra[3], "id"))), http.StatusTooManyRequests).errorCode(t); code != "rate_limited" {
		t.Fatalf("code = %s", code)
	}

	// Blocking ends the session and refuses new sign-ins; team users cannot block.
	repID := str(obj(list(mustStatus(t, h.call(http.MethodGet, "/api/v1/admin/suspicions/"+str(b, "id"), nil), http.StatusOK).json(t), "reports")[0].(map[string]any), "reporter"), "id")
	mustStatus(t, squad.call(http.MethodPatch, "/api/v1/admin/reporters/"+repID, map[string]any{"blocked": true}), http.StatusForbidden)
	mustStatus(t, h.call(http.MethodPatch, "/api/v1/admin/reporters/"+repID, map[string]any{"blocked": true}), http.StatusOK)
	mustStatus(t, rep.call(http.MethodGet, "/api/v1/reporter/me", nil), http.StatusUnauthorized)
	anon := h.anonymous()
	mustStatus(t, anon.call(http.MethodPost, "/api/v1/reporter/login-link", map[string]any{"email": "carla@empresa.com"}), http.StatusAccepted)
	m := reporterLinkToken.FindStringSubmatch(h.mailer.take()[0].Text)
	if code := mustStatus(t, anon.call(http.MethodPost, "/api/v1/reporter/login", map[string]any{"token": m[1]}), http.StatusForbidden).errorCode(t); code != "reporter_blocked" {
		t.Fatalf("code = %s", code)
	}
	// Link limit per address.
	mustStatus(t, anon.call(http.MethodPost, "/api/v1/reporter/login-link", map[string]any{"email": "carla@empresa.com"}), http.StatusAccepted)
	mustStatus(t, anon.call(http.MethodPost, "/api/v1/reporter/login-link", map[string]any{"email": "carla@empresa.com"}), http.StatusTooManyRequests)
	// Cross-site requests are refused.
	mustStatus(t, rep.call(http.MethodPost, "/api/v1/reporter/logout", nil, withHeader("Sec-Fetch-Site", "cross-site")), http.StatusForbidden)
}
