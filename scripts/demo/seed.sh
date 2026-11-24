#!/usr/bin/env bash
# Popula uma instância VAZIA com dados fictícios de demonstração (empresa
# "Nimbus"): departamentos, times, usuários, grupos/subgrupos, componentes,
# incidentes (ativo e resolvidos), manutenções e comunicados.
#
# Usa só a API REST. Serve para conhecer a ferramenta e gerar as capturas do
# README. Não rode em produção.
#
#   BASE_URL=http://localhost:8080 SETUP_TOKEN=<token do log> scripts/demo/seed.sh
#
# Sem SETUP_TOKEN, faz login com ADMIN_EMAIL/ADMIN_PASSWORD (usuário existente).
# Depois, opcionalmente, scripts/demo/history.sql espalha o histórico de status
# pelos últimos 90 dias (barras de disponibilidade).
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
ADMIN_NAME="${ADMIN_NAME:-Ana Souza}"
ADMIN_EMAIL="${ADMIN_EMAIL:-ana.souza@nimbus.example}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-demo-password-123}"
USER_PASSWORD="${USER_PASSWORD:-demo-password-123}"

command -v jq >/dev/null || { echo "jq é obrigatório" >&2; exit 1; }
JAR="$(mktemp)"
trap 'rm -f "$JAR"' EXIT

# api METHOD PATH [JSON] -> imprime o corpo; falha com a resposta em caso de erro.
api() {
  local out code
  out="$(curl -sS -b "$JAR" -c "$JAR" -X "$1" "$BASE_URL$2" \
    -H 'Content-Type: application/json' ${3:+--data "$3"} -w '\n%{http_code}')"
  code="${out##*$'\n'}"; out="${out%$'\n'*}"
  if [ "$code" -ge 400 ]; then echo "$1 $2 -> $code: $out" >&2; exit 1; fi
  printf '%s' "$out"
}
id() { jq -r .id; }
# at MINUTOS [FORMATO]: instante UTC relativo a agora (negativo = passado). GNU ou BSD date.
at() { local s=$(( $(date +%s) + $1 * 60 )) f="${2:-%Y-%m-%dT%H:%M:%SZ}"
  date -u -d "@$s" +"$f" 2>/dev/null || date -u -r "$s" +"$f"; }
DAY=1440

echo "→ Sessão em $BASE_URL"
if [ -n "${SETUP_TOKEN:-}" ]; then
  api POST /api/v1/auth/setup "$(jq -n --arg t "$SETUP_TOKEN" --arg n "$ADMIN_NAME" \
    --arg e "$ADMIN_EMAIL" --arg p "$ADMIN_PASSWORD" '{setup_token:$t,name:$n,email:$e,password:$p}')" >/dev/null
else
  api POST /api/v1/auth/login "$(jq -n --arg e "$ADMIN_EMAIL" --arg p "$ADMIN_PASSWORD" '{email:$e,password:$p}')" >/dev/null
fi

if [ "$(api GET '/api/v1/admin/components?limit=1' | jq '.data | length')" != 0 ]; then
  echo "A instância já tem componentes; o seed é só para instâncias vazias." >&2; exit 1
fi

echo "→ Departamentos e times"
api PUT /api/v1/admin/department-levels '{"names":["Diretoria","Área"]}' >/dev/null
dept() { api POST /api/v1/admin/departments "$(jq -n --arg n "$1" --arg d "$2" --arg p "${3:-}" \
  '{name:$n,description:$d} + (if $p=="" then {} else {parent_id:$p} end)')" | id; }
D_TEC=$(dept "Tecnologia" "Engenharia, plataforma e segurança")
D_NEG=$(dept "Negócios" "Produtos financeiros e atendimento")
D_PLAT=$(dept "Plataforma" "Infraestrutura e confiabilidade" "$D_TEC")
D_PROD=$(dept "Produtos digitais" "Experiência do cliente na web e no app" "$D_TEC")
D_PAG=$(dept "Meios de pagamento" "Cartões, Pix e boleto" "$D_NEG")

team() { api POST /api/v1/admin/teams "$(jq -n --arg n "$1" --arg k "$2" --arg d "$3" --arg dep "$4" --argjson tags "$5" \
  '{name:$n,kind:$k,description:$d,department_id:$dep,tags:$tags,
    documentation_url:("https://wiki.nimbus.example/" + ($n|ascii_downcase|gsub(" ";"-"))),
    support_url:"https://nimbus.example/suporte"}')" | id; }
T_SRE=$(team "SRE" "Squad" "Infraestrutura, banco de dados e observabilidade" "$D_PLAT" '["infra","plantão"]')
T_CHK=$(team "Checkout" "Squad" "Carrinho, checkout e área do cliente" "$D_PROD" '["web"]')
T_ID=$(team "Identidade" "Squad" "Login, OAuth e cadastro" "$D_PROD" '["segurança"]')
T_PAG=$(team "Pagamentos" "Tribo" "Processamento de cartões, Pix e boleto" "$D_PAG" '["pix","cartões"]')

echo "→ Usuários"
ROLES="$(api GET /api/v1/admin/roles)"
role() { jq -r --arg n "$1" '.data[] | select(.name==$n) | .id' <<<"$ROLES" | head -1; }
OPER="$(role "Operador do time")"; [ -n "$OPER" ] || OPER="$(jq -r '.data[-1].id' <<<"$ROLES")"
user() { api POST /api/v1/admin/users "$(jq -n --arg n "$1" --arg e "$2" --arg j "$3" --arg r "$OPER" \
  --arg t "$4" --arg d "$5" --arg p "$USER_PASSWORD" \
  '{name:$n,email:$e,job_title:$j,password:$p,role_ids:[$r],team_ids:[$t],department_ids:[$d]}')" >/dev/null; }
user "Bruno Lima" "bruno.lima@nimbus.example" "Engenheiro SRE" "$T_SRE" "$D_PLAT"
user "Carla Mendes" "carla.mendes@nimbus.example" "Tech lead" "$T_CHK" "$D_PROD"
user "Diego Rocha" "diego.rocha@nimbus.example" "Engenheiro de software" "$T_PAG" "$D_PAG"
user "Elisa Prado" "elisa.prado@nimbus.example" "Analista de segurança" "$T_ID" "$D_PROD"

echo "→ Grupos e componentes"
group() { api POST /api/v1/admin/component-groups "$(jq -n --arg n "$1" --arg d "$2" --argjson o "$3" --arg t "${4:-}" --arg p "${5:-}" \
  '{name:$n,description:$d,display_order:$o} + (if $t=="" then {} else {team_id:$t} end)
   + (if $p=="" then {} else {parent_id:$p} end)')" | id; }
comp() { # nome ordem grupo subgrupo time descrição [url]
  api POST /api/v1/admin/components "$(jq -n --arg n "$1" --argjson o "$2" --arg g "$3" --arg s "$4" --arg t "$5" --arg d "$6" --arg u "${7:-}" \
  '{name:$n,display_order:$o,description:$d,show_description:($d!=""),is_public:true}
   + (if $g=="" then {} else {group_id:$g} end) + (if $s=="" then {} else {subgroup_id:$s} end)
   + (if $t=="" then {} else {team_id:$t} end) + (if $u=="" then {} else {url:$u} end)')" | id; }

G_WEB=$(group "Site e app" "Canais usados pelos clientes" 10 "$T_CHK")
C_SITE=$(comp "Site" 1 "$G_WEB" "" "" "Loja e páginas institucionais" "https://nimbus.example")
C_CHECKOUT=$(comp "Checkout" 2 "$G_WEB" "" "" "")
C_CONTA=$(comp "Área do cliente" 3 "$G_WEB" "" "" "")
C_APP=$(comp "App iOS e Android" 4 "$G_WEB" "" "" "")

G_PAG=$(group "Pagamentos" "Meios de pagamento aceitos" 20 "$T_PAG")
SG_CARD=$(group "Cartões" "" 1 "" "$G_PAG")
SG_PIX=$(group "Pix" "" 2 "" "$G_PAG")
C_CRED=$(comp "Cartão de crédito" 1 "$G_PAG" "$SG_CARD" "" "")
C_DEB=$(comp "Cartão de débito" 2 "$G_PAG" "$SG_CARD" "" "")
C_PIXQR=$(comp "Pix QR Code" 1 "$G_PAG" "$SG_PIX" "" "")
C_PIXREF=$(comp "Devoluções Pix" 2 "$G_PAG" "$SG_PIX" "" "")
C_BOLETO=$(comp "Boleto" 3 "$G_PAG" "" "" "Emissão e compensação de boletos")

G_API=$(group "APIs para parceiros" "Integrações de lojistas e parceiros" 30 "$T_ID")
C_API=$(comp "API pública" 1 "$G_API" "" "" "REST v2" "https://developers.nimbus.example")
C_HOOK=$(comp "Webhooks" 2 "$G_API" "" "" "")
C_AUTH=$(comp "Autenticação (OAuth)" 3 "$G_API" "" "" "")

G_INFRA=$(group "Infraestrutura" "" 40 "$T_SRE")
C_DB=$(comp "Banco de dados" 1 "$G_INFRA" "" "" "")
C_CDN=$(comp "CDN e imagens" 2 "$G_INFRA" "" "" "")
C_FILA=$(comp "Processamento de pedidos" 3 "$G_INFRA" "" "" "")
C_HELP=$(comp "Central de ajuda" 50 "" "" "$T_CHK" "Artigos e chat de atendimento" "https://ajuda.nimbus.example")

echo "→ Incidentes resolvidos (histórico)"
past_incident() { # título impacto início fim componentes mensagem-inicial mensagem-final
  local i; i=$(api POST /api/v1/admin/incidents "$(jq -n --arg t "$1" --arg im "$2" --arg s "$3" --argjson c "$5" --arg m "$6" \
    '{title:$t,impact:$im,status:"investigating",started_at:$s,component_ids:$c,message:$m}')" | id)
  api POST "/api/v1/admin/incidents/$i/updates" '{"status":"identified","message":"Causa identificada. A correção está sendo aplicada."}' >/dev/null
  api POST "/api/v1/admin/incidents/$i/resolve" "$(jq -n --arg r "$4" --arg m "$7" '{resolved_at:$r,message:$m}')" >/dev/null
}
past_incident "Lentidão no checkout" degraded_performance "$(at $((-12*DAY)))" "$(at $((-12*DAY+95)))" \
  "[\"$C_CHECKOUT\"]" "Alguns clientes relatam lentidão ao finalizar a compra." \
  "Normalizado após aumento de capacidade. Nenhum pedido foi perdido."
past_incident "Falha na emissão de boletos" partial_outage "$(at $((-27*DAY)))" "$(at $((-27*DAY+180)))" \
  "[\"$C_BOLETO\"]" "A emissão de boletos está falhando para parte dos pedidos." \
  "O banco parceiro corrigiu a instabilidade. Boletos emitidos normalmente."
past_incident "Indisponibilidade da API pública" major_outage "$(at $((-41*DAY)))" "$(at $((-41*DAY+50)))" \
  "[\"$C_API\",\"$C_HOOK\"]" "A API pública está retornando erro 503." \
  "Um certificado expirado foi renovado e o monitoramento de validade foi ampliado."
past_incident "Atraso na entrega de webhooks" degraded_performance "$(at $((-63*DAY)))" "$(at $((-63*DAY+120)))" \
  "[\"$C_HOOK\"]" "Notificações de webhook estão chegando com até 15 minutos de atraso." \
  "A fila foi drenada e as entregas estão em dia."

echo "→ Incidente em andamento"
I_PIX=$(api POST /api/v1/admin/incidents "$(jq -n --arg s "$(at -40)" --argjson c "[\"$C_PIXQR\"]" \
  '{title:"Instabilidade na confirmação de pagamentos Pix",impact:"degraded_performance",status:"investigating",
    started_at:$s,component_ids:$c,
    message:"Alguns pagamentos Pix estão levando mais tempo para serem confirmados. Estamos investigando."}')" | id)
api POST "/api/v1/admin/incidents/$I_PIX/updates" '{"status":"identified","message":"Identificamos lentidão no provedor de mensageria do Pix. As confirmações estão sendo processadas com atraso médio de 3 minutos."}' >/dev/null

echo "→ Manutenções"
api POST /api/v1/admin/maintenances "$(jq -n --arg s "$(at $((3*DAY)) %Y-%m-%dT%H:00)" --arg e "$(at $((3*DAY+120)) %Y-%m-%dT%H:00)" \
  --argjson c "[\"$C_DB\",\"$C_CONTA\"]" \
  '{title:"Atualização do banco de dados",timezone:"UTC",scheduled_start:$s,scheduled_end:$e,expected_impact:"partial_outage",
    component_ids:$c,message:"A área do cliente ficará somente leitura durante a janela. Compras e pagamentos não serão afetados."}')" >/dev/null
api POST /api/v1/admin/maintenances "$(jq -n --arg s "$(at $((9*DAY)) %Y-%m-%dT%H:00)" --arg e "$(at $((9*DAY+60)) %Y-%m-%dT%H:00)" \
  --argjson c "[\"$C_CDN\"]" \
  '{title:"Troca do provedor de CDN",timezone:"UTC",scheduled_start:$s,scheduled_end:$e,expected_impact:"operational",
    component_ids:$c,message:"Migração gradual, sem impacto esperado."}')" >/dev/null

echo "→ Comunicados"
CATS="$(api GET /api/v1/admin/announcement-categories)"
cat_id() { jq -r '.data[0].id // empty' <<<"$CATS"; }
CAT="$(cat_id)"
[ -n "$CAT" ] || CAT=$(api POST /api/v1/admin/announcement-categories '{"name":"Novidades","color":"blue"}' | id)
ann() { local a; a=$(api POST /api/v1/admin/announcements "$(jq -n --arg t "$1" --arg b "$2" --arg im "$3" --argjson pin "$4" \
  --arg c "$CAT" --arg tm "$5" '{title:$t,body:$b,importance:$im,is_pinned:$pin,category_id:$c,team_id:$tm}')" | id)
  api POST "/api/v1/admin/announcements/$a/publish" '{}' >/dev/null; }
ann "Nova versão da API de pagamentos" $'A **API v2** já está disponível, com Pix parcelado e estorno parcial.\n\nA v1 será desativada em março. Veja o [guia de migração](https://developers.nimbus.example).' important true "$T_ID"
ann "Horário estendido do atendimento" "O chat da central de ajuda passa a funcionar das 7h às 23h, todos os dias." info false "$T_CHK"

echo "✓ Pronto. Acesse $BASE_URL e $BASE_URL/admin ($ADMIN_EMAIL)."
