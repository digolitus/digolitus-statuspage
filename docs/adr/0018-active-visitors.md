# ADR 0018 — Quem está acessando agora

- **Status:** proposto
- **Data:** 2026-11-11

## Contexto

O admin precisa mostrar quantas pessoas estão com a página pública aberta e quais usuários estão no admin, quase em tempo real. Restrições: LGPD (sem dados pessoais de visitantes), baixo custo e nenhuma infraestrutura nova. O backend pode rodar em várias réplicas, então a contagem não pode ficar só na memória de um processo.

## Decisão

- **Sinal de presença (heartbeat).** Cada aba envia `POST` a cada 60 s enquanto está visível. A aba conta como presente por 2 min após o último sinal, então um sinal perdido não derruba a contagem.
  - Página pública: `POST /api/v1/presence` com um `tab_id` aleatório gerado na aba (`sessionStorage`, some ao fechar) e a área (`status`, `incident`, `maintenance`, `announcements`, `subscriptions`, `other`).
  - Admin: `POST /api/v1/admin/presence` com o rótulo da tela e `hidden`. Abas do admin continuam enviando em segundo plano, marcadas como `hidden`.
- **Privacidade.** O servidor guarda só o SHA-256 do `tab_id`, a área e o horário. Não guarda IP, user agent nem cookie. As linhas são apagadas 10 min após o último sinal. O histórico (`presence_samples`) guarda só contagens por minuto e é mantido por 48 h.
- **Armazenamento.** `presence_heartbeats` é uma tabela `UNLOGGED` no Postgres existente: não passa pelo WAL (escrita barata) e é esvaziada após um crash, o que só zera a contagem. Custo estimado: uma escrita por aba por minuto (1.000 abas ≈ 17 escritas/s).
- **Amostragem.** Cada réplica roda `presence.Service.Run`: a cada minuto grava a contagem do minuto (`GREATEST` entre réplicas, idempotente) e apaga o que expirou.
- **Abuso.** Abas novas são limitadas por processo (token bucket: 10/s, rajada de 600), sem guardar IP. Acima do limite, o sinal é ignorado com 204. Abas já conhecidas nunca são limitadas. Um `tab_id` de admin não pode ser assumido por outro usuário. O número é sempre apresentado como aproximado.
- **Acesso.** `GET /api/v1/admin/presence` exige a permissão nova `presence.read` ("Ver acessos em tempo real"). O sinal do admin é aceito de qualquer usuário autenticado.
- **Desligar.** `PRESENCE_ENABLED=false` remove as rotas e o amostrador. A presença também fica desligada sem a API de admin.

## Alternativas descartadas

- **Contagem em memória:** errada com várias réplicas.
- **Redis:** infraestrutura nova sem necessidade nesta escala.
- **WebSocket/SSE:** conexões longas no nginx e no backend para um número que tolera 30 s de atraso.
- **Logs de acesso ou IP:** dado pessoal e contagem de robôs.

## Consequências

- Robôs e leitores sem JavaScript não são contados. Uma aba em segundo plano da página pública sai da contagem em até 2 min.
- O painel do admin consulta a cada 30 s. A tabela de sinais é limitada pelo limitador e pela limpeza de 10 min.
- Se a escala crescer muito, o mesmo contrato HTTP permite trocar o armazenamento (por exemplo, Redis) sem mudar o frontend.
