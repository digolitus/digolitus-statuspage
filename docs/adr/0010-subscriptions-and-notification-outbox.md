# ADR 0010 — Inscrições e entrega de notificações por outbox

- **Status:** aceito
- **Data:** 2026-10-18

## Contexto

Pessoas e sistemas querem receber as atualizações da página: incidentes, manutenções e comunicados. Os canais são e-mail, webhook, Slack e feeds RSS/Atom. A entrega:

- não pode atrasar nem desfazer a operação de quem publica;
- não pode se perder em falhas;
- não pode virar vetor de spam ou de SSRF.

## Decisão

- **Outbox transacional.**
  - Cada mudança pública grava um `notification_event` na mesma transação da operação (`notify.Record`). Comunicados agendados geram o evento quando ficam visíveis, marcados por `announcements.notified_at`.
  - Um worker no próprio backend (`NOTIFY_WORKER`, intervalo `NOTIFY_INTERVAL`):
    1. expande os eventos em `outbox_messages`, uma por inscrição interessada, com `UNIQUE (event_id, subscription_id)`;
    2. entrega com `FOR UPDATE SKIP LOCKED`, o que permite rodar em várias réplicas.
  - Falhas são repetidas com backoff, em até 6 tentativas. Webhook ou Slack com 15 falhas seguidas é desativado e o motivo fica registrado.
  - Dados antigos são removidos periodicamente.
- **Filtro por componente.** Uma inscrição recebe tudo ou só os componentes escolhidos. Eventos sem componente (avisos gerais) chegam sempre.
- **Canais:**
  - **E-mail** por SMTP (`starttls`, `tls` ou `none`), em texto puro. Cabeçalhos sanitizados contra injeção de CRLF. `List-Unsubscribe` com one-click (RFC 8058).
  - **Webhook:** POST JSON assinado com `X-Digolitus-Signature: sha256=HMAC(segredo, timestamp + "." + corpo)`, com o timestamp em `X-Digolitus-Timestamp`.
  - **Slack:** incoming webhook. A URL precisa ser `https://hooks.slack.com/services/…` e o texto é escapado.
  - **RSS/Atom:** gerados dos eventos, sem cadastro.
  - Webhook e Slack usam o cliente `platform/safehttp`: só HTTPS, sem redirecionamentos, endereços privados bloqueados salvo `NOTIFY_ALLOW_PRIVATE_NETWORKS`.
- **Autoinscrição com confirmação dupla (double opt-in).**
  - Toda inscrição pública (inclusive de webhook e Slack) só começa depois do clique no link enviado por e-mail.
  - A resposta é sempre 202, sem revelar se o e-mail já existe. Se já existe, a pessoa recebe um link de gerenciamento.
  - Há limites de reenvio (10 min) e de pendências (300 por hora), e o nginx limita a taxa de `/api/v1/subscriptions`.
  - Os links levam tokens HMAC (chave em `app_secrets`) com propósito separado para "confirmar" e para "gerenciar". A confirmação é um POST disparado por clique, porque scanners de link só fazem GET. A interface remove o token da barra de endereço.
  - Sem SMTP, só os feeds ficam disponíveis ao público, e administradores com `subscriptions.manage` ainda criam integrações de webhook e Slack, ativas na hora.
- **SMS não é oferecido:** exige provedor pago, verificação de número e custos por mensagem. O canal pode ser adicionado como um novo `Sender` sem mudar o outbox.

## Consequências

- Nenhuma fila ou serviço novo: o PostgreSQL faz o papel de fila. Se o volume crescer muito, o outbox pode ser drenado para uma fila externa sem mudar o modelo.
- Endereços de inscritos são dados pessoais. Só `subscriptions.manage` os vê, a exclusão é definitiva e o cancelamento leva um clique.
- A entrega é "pelo menos uma vez": receptores de webhook devem deduplicar por `X-Digolitus-Delivery`.
