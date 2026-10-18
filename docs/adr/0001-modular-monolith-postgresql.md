# ADR 0001 — Monólito modular em Go com PostgreSQL e SQL explícito

- **Status:** aceito
- **Data:** 2026-10-18

## Contexto

O produto passou de "hello world" para gestão de componentes, incidentes, manutenções, templates e auditoria. Isso exige persistência durável, consistência transacional entre o estado atual e o histórico, e testes contra um banco real. A equipe é pequena e a carga esperada é modesta.

## Decisão

- **Um único serviço Go**, organizado em módulos por domínio (`component`, `incident`, `maintenance`, `templates`, `audit`, `statuspage`) sobre uma camada de plataforma (`database`, `httpx`, `clock`).
- **PostgreSQL** como única fonte de verdade.
- **`pgx/v5` com SQL explícito e parametrizado**, sem ORM. Uma interface `Querier` permite que a mesma função rode no pool ou numa transação.
- **Migrações próprias e embutidas** (cerca de 100 linhas): ordem por versão, uma transação por migração, `pg_advisory_lock`, checksum e recusa a esquemas mais novos.
- **Integridade no banco:** `CHECK` para enumerações e coerência de estados, `FOREIGN KEY`, unicidade e índices para as consultas de listagem.

## Alternativas consideradas

- **Microsserviços por domínio:** sem justificativa de escala, isolamento ou equipes distintas. Trariam consistência eventual entre incidentes e componentes.
- **ORM (GORM/ent):** mais abstração do que o domínio pede e menos controle sobre bloqueios, consultas de timeline e índices.
- **Ferramenta de migração externa (goose, golang-migrate):** válida, mas acrescenta dependência e binário para um problema pequeno. Pode ser adotada se surgirem necessidades como migrações em Go ou rollback.

## Consequências

- Uma alteração e seu registro de histórico são gravados de forma atômica. Transições usam `SELECT ... FOR UPDATE`, e há teste de concorrência cobrindo isso.
- O SQL é revisado como código, e consultas novas exigem índices pensados explicitamente.
- O backend não inicia sem banco (`/readyz` retorna 503 enquanto o banco está indisponível; `/healthz` continua 200).

## Reavaliar quando

- Houver necessidade de escalar leitura pública separadamente (por exemplo, cache/CDN do resumo ou réplica de leitura).
- Surgirem workers em segundo plano (verificações automáticas, notificações) com ciclos de vida próprios.
