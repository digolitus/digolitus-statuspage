# Digolitus Status Page

Plataforma para publicar o status de serviços e comunicar incidentes e manutenções programadas.

- **Área pública** (`/`): status geral, componentes, incidentes ativos com linha do tempo, manutenções em andamento e programadas, e histórico.
- **Área administrativa** (`/admin`): componentes, incidentes, manutenções, templates e histórico operacional.
- **API REST** (`/api/v1`), documentada em OpenAPI 3.0, com Swagger UI em `/swagger/`.

> **Status declarado, não monitorado.** Todos os status são informados por operadores. A aplicação não faz verificações automáticas de disponibilidade, e a interface diz isso explicitamente.
>
> **Administração sem autenticação.** Nesta versão, quem alcança `/admin` ou `/api/v1/admin/` pode alterar todos os dados. Por padrão, o Compose publica a aplicação apenas em `127.0.0.1`. Leia [docs/operations.md](docs/operations.md#administração-sem-autenticação) antes de expor a aplicação em qualquer rede.

## Início rápido

Pré-requisitos: Docker 24+ com o plugin Compose. Não é preciso login em registry nem serviços externos.

```bash
docker compose -f deploy/docker-compose.yml up --build
```

| URL | Conteúdo |
|---|---|
| <http://localhost:8080> | Página pública |
| <http://localhost:8080/admin> | Área administrativa |
| <http://localhost:8080/swagger/> | Swagger UI |
| <http://localhost:8080/api/openapi.yaml> | Especificação OpenAPI |

As migrações do banco são aplicadas automaticamente na inicialização do backend. O banco começa com 7 templates de incidente e 6 de manutenção, sem componentes, incidentes ou manutenções.

### Comandos do dia a dia

```bash
make up          # sobe frontend, backend e PostgreSQL (foreground)
make ps          # estado e saúde dos containers
make logs        # logs de todos os serviços
make migrate     # aplica migrações pendentes manualmente
make down        # para os containers; os dados são preservados
make db-reset    # DESTRUTIVO: apaga o volume do banco (pede confirmação)
```

Variáveis opcionais (porta, credenciais locais, `ADMIN_API_ENABLED` etc.) estão em [deploy/.env.example](deploy/.env.example). Copie-o para `deploy/.env` para sobrescrever os valores padrão.

## Arquitetura

```text
Navegador ──► Nginx (frontend, 127.0.0.1:8080)
                ├── /                   SPA React (página pública e /admin)
                ├── /api/v1/...         ─┐
                ├── /api/v1/admin/...   ─┤ rate limit mais restrito
                ├── /api/openapi.yaml   ─┼──► Backend Go :8080 (rede interna)
                ├── /swagger/           ─┤          │
                └── /healthz, /readyz   ─┘          ▼
                                             PostgreSQL 18 (rede interna, volume pgdata)
```

- **Backend:** monólito modular em Go (`apps/backend`), SQL explícito com `pgx`, migrações versionadas e embutidas no binário. Detalhes em [docs/architecture.md](docs/architecture.md).
- **Frontend:** React + TypeScript + Vite (`apps/frontend`). Consome exclusivamente a API.
- **Decisões:** [docs/adr/](docs/adr/).

## API

Toda funcionalidade de negócio está disponível pela API. Resumo:

| Área | Endpoints |
|---|---|
| Saúde | `GET /healthz` (liveness), `GET /readyz` (inclui PostgreSQL) |
| Pública | `GET /api/v1/status`, `/components`, `/incidents[/{id}]`, `/maintenances[/{id}]` |
| Componentes | `GET/POST /api/v1/admin/components`, `GET/PATCH .../{id}` |
| Incidentes | `GET/POST /api/v1/admin/incidents`, `GET/PATCH .../{id}`, `GET/POST .../{id}/updates`, `POST .../{id}/resolve` |
| Manutenções | `GET/POST /api/v1/admin/maintenances`, `GET/PATCH .../{id}`, `GET/POST .../{id}/updates`, `POST .../{id}/start\|complete\|cancel` |
| Templates | `GET/POST /api/v1/admin/incident-templates`, `GET/PATCH .../{id}` (idem para `maintenance-templates`) |
| Auditoria | `GET /api/v1/admin/audit-events` |

O contrato completo (schemas, erros, paginação, filtros, ordenação e transições de estado) está em [`apps/backend/api/openapi.yaml`](apps/backend/api/openapi.yaml). Os testes falham se uma rota implementada não estiver documentada (ou vice-versa) e validam as respostas reais contra os schemas.

Exemplo:

```bash
curl -s -X POST http://localhost:8080/api/v1/admin/components \
  -H 'Content-Type: application/json' -d '{"name":"API pública"}'

curl -s -X POST http://localhost:8080/api/v1/admin/maintenances \
  -H 'Content-Type: application/json' \
  -d '{"title":"Atualização do banco","timezone":"America/Sao_Paulo",
       "scheduled_start":"2026-11-20T02:00","scheduled_end":"2026-11-20T04:00"}'
```

### Ciclos de vida

- **Incidente:** `investigating ⇄ identified ⇄ monitoring → resolved`. `resolved` é terminal (409 em novas atualizações). Atualizações publicadas são imutáveis.
- **Manutenção:** `scheduled → in_progress → completed` e `scheduled → cancelled`. Nada muda automaticamente por horário; janelas atrasadas são sinalizadas (`start_overdue`, `end_overdue`).
- **Status geral:** calculado por uma política única ([ADR 0003](docs/adr/0003-status-policy.md)). Manutenção nunca é apresentada como indisponibilidade.

## Desenvolvimento sem Docker

Requisitos: Go 1.27+, Node.js 24 (LTS) com npm, PostgreSQL 16+.

```bash
# Backend (porta 8080)
cd apps/backend
DATABASE_URL='postgres://user:pass@localhost:5432/statuspage?sslmode=disable' go run ./cmd/api

# Frontend (Vite em http://localhost:5173, com proxy para o backend em :8080)
cd apps/frontend
npm ci
npm run dev
```

Configuração do backend (variáveis de ambiente, validadas na inicialização):

| Variável | Padrão | Descrição |
|---|---|---|
| `DATABASE_URL` | — (obrigatória) | URL de conexão PostgreSQL |
| `PORT` | `8080` | Porta HTTP |
| `DB_MAX_CONNS` | `10` | Tamanho máximo do pool (1–100) |
| `DB_AUTO_MIGRATE` | `true` | Aplica migrações na inicialização |
| `ADMIN_API_ENABLED` | `true` | `false` remove todas as rotas administrativas (API pública somente leitura) |
| `REQUEST_TIMEOUT` | `10s` | Tempo máximo por requisição (1s–1m) |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` (logs JSON) |

## Testes e verificações

```bash
make test-backend      # unitários + validação da especificação OpenAPI
make test-integration  # suíte completa contra PostgreSQL (TEST_DATABASE_URL)
make test-frontend     # Vitest + Testing Library
make lint              # gofmt, go vet, ESLint
make build             # compila backend e frontend (inclui checagem de tipos)
make scan              # Trivy local (vulnerabilidades, segredos, configuração)
```

Os testes de integração criam um banco isolado por teste. Para isso, precisam de um usuário com permissão de `CREATE DATABASE`:

```bash
TEST_DATABASE_URL='postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable' make test-integration
```

## CI

O workflow [`.github/workflows/ci.yml`](.github/workflows/ci.yml) executa em pull requests e em pushes para `main`. Ele só executa as verificações dos módulos alterados; mudanças em arquivos compartilhados (workflows, Compose, Makefile) acionam tudo. As verificações são:

- **Backend:** `gofmt`, `go mod verify`/`tidy -diff`, `go vet`, testes com `-race` contra PostgreSQL (incluindo contrato OpenAPI), `govulncheck` e `gosec`.
- **Frontend:** `npm ci`, ESLint, Vitest, checagem de tipos + build e `npm audit` das dependências de produção.
- **Containers:** build das imagens, validação do Compose, smoke test da stack completa via Nginx (incluindo a verificação de que backend e banco não ficam expostos) e Trivy nas imagens.
- **Repositório:** Trivy (vulnerabilidades, segredos e configuração).
- **Pull requests:** revisão de dependências.

As Actions são fixadas por SHA de commit. O Dependabot propõe atualizações semanais de Go, npm, imagens Docker e Actions.

**Ainda não existem** workflows de publicação de imagens nem de deploy. O destino de produção não foi definido.

## Estrutura

```text
apps/backend/     API Go (cmd/, internal/, api/openapi.yaml, migrações em internal/platform/database/migrations)
apps/frontend/    React + Vite + Nginx
deploy/           Docker Compose e exemplo de variáveis
docs/             Arquitetura, operação e ADRs
scripts/          Utilitários (ex.: atualização da Swagger UI embutida)
CONTEXT.md        Princípios de engenharia do projeto
```

## Limitações conhecidas

- Sem autenticação e autorização na administração (veja [ADR 0004](docs/adr/0004-admin-without-authentication.md)).
- Sem verificações automáticas de saúde, cálculo de uptime, notificações ou assinaturas.
- Sem publicação de imagens nem deploy automatizado.
