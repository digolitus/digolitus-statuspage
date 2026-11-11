# ADR 0015 — Status de componentes informado por ferramentas de monitoramento

- **Status:** aceito
- **Data:** 2026-11-11

## Contexto

Até aqui todo status era declarado por operadores ([ADR 0003](0003-status-policy.md)). Quem já monitora os serviços no UptimeRobot ou no Healthchecks.io precisava repetir à mão no status page o que a ferramenta já sabia, com atraso. Grafana Cloud e Datadog estão no horizonte com a mesma necessidade.

## Decisão

- Um módulo `internal/monitoring` recebe **alertas por webhook (push)**. O backend não consulta as APIs das ferramentas: não guardamos chaves delas, não há agendamento e o status muda em segundos.
- Cada **integração** (`monitoring_integrations`) tem um token aleatório de 256 bits, mostrado uma única vez; o banco guarda só o SHA-256. O token vai em `?token=` (o UptimeRobot não envia cabeçalhos próprios) ou em `Authorization: Bearer`. O backend não registra query strings no log e o Nginx não registra esse caminho; o Nginx também limita a taxa.
- Um único endpoint, `POST /api/v1/hooks/monitoring`. A integração identifica a ferramenta, e um tradutor por ferramenta converte o corpo (JSON ou formulário) em "monitor X caiu/voltou". Grafana e Datadog entram como novos tradutores, sem mudar o resto.
- Alertas de monitores desconhecidos criam o **monitor** (`monitoring_monitors`) sem componente. O operador liga cada monitor a um componente e escolhe o status enquanto ele estiver fora do ar (`degraded_performance`, `partial_outage` ou `major_outage`; padrão `major_outage`).
- **Política:** o status do monitor é mais uma entrada de `statuspolicy`. O status efetivo é o mais grave entre o declarado, os monitores fora do ar, os incidentes e a manutenção. O monitor nunca esconde um problema declarado, e quando volta o status declarado é restaurado sozinho.
- O componente só é apresentado como **monitorado** depois que a ferramenta mandou ao menos um alerta de um monitor ligado (um monitor que nunca falou não prova nada). `overall.basis` passa a valer `declared`, `mixed` ou `monitored`.
- Cada mudança de estado de um monitor ligado gera um registro no histórico operacional (`monitoring_integration`) e no histórico de status (`cause_type = monitoring`, [ADR 0014](0014-status-history.md)), na mesma transação. Os últimos 200 alertas de cada integração ficam em `monitoring_events` para diagnóstico, inclusive os ilegíveis, que são respondidos com 422 para a ferramenta mostrar a falha.
- Desativar ou remover uma integração retira o efeito dos seus monitores. Com `ADMIN_API_ENABLED=false` o webhook não existe, porque ele altera status.
- Permissão nova `monitoring.manage`; leitura liberada a qualquer usuário autenticado, como os demais cadastros.

## Alternativas consideradas

- **Consultar as APIs das ferramentas (pull):** exige guardar chaves de terceiros, um poller por ferramenta e lidar com limites de API. Fica como opção futura para quem não puder configurar webhooks.
- **Só avisar a equipe e esperar confirmação:** mais seguro contra falsos positivos, mas o Rod escolheu a atualização automática; o operador continua podendo desligar um monitor ou a integração.
- **HMAC no corpo:** UptimeRobot e Healthchecks.io não assinam webhooks. O token de 256 bits no endereço tem força equivalente; a troca do endereço é imediata.
- **Abrir incidente automaticamente:** comunica mais, mas um texto gerado por máquina na página pública precisa de revisão. Fica para uma fase seguinte, como rascunho.

## Consequências

- O status publicado pode mudar sem ação humana. A página pública indica quais componentes são monitorados, e a administração mostra a origem de cada alerta ativo.
- Webhooks não detectam silêncio: se a ferramenta parar de enviar, o último estado fica valendo. Healthchecks.io cobre isso para jobs (alerta quando o job não dá sinal); para o UptimeRobot, o próprio serviço é a fonte.
- Ferramentas que repetem o mesmo alerta não geram mudanças repetidas: só a troca de estado conta.
