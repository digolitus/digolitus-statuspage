# Digolitus Status Page

![Texto alternativo](./brand/logo-digolitus-statuspage-tema-light.png) 

Plataforma para publicar o status de serviços e comunicar incidentes e manutenções programadas.

- **Área pública** (`/`): status geral, eventos em andamento (só quando existem), componentes com grupos recolhíveis, manutenções programadas, histórico sob demanda e inscrição para receber atualizações (e-mail, Slack, webhook, RSS/Atom).
- **Área administrativa** (`/admin`): login local, usuários e papéis com permissões (globais ou por time), times responsáveis, componentes, grupos, incidentes, manutenções, templates, comunicados, inscrições, serviços de terceiros e histórico operacional com autor.
- **API REST** (`/api/v1`), documentada em OpenAPI 3.0, com Swagger UI em `/swagger/`.

> **Status declarado, não monitorado.** Todos os status são informados por operadores. A aplicação não faz verificações automáticas de disponibilidade, e a interface diz isso explicitamente.
>
> **Administração com login.** A área administrativa exige usuário e senha. Cada usuário tem papéis que definem o que pode alterar ([ADR 0008](docs/adr/0008-local-authentication-and-rbac.md)). O primeiro administrador é criado na primeira execução, com um token exibido no log do backend (veja abaixo).

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

### Primeiro acesso

Na primeira execução não existe usuário. O backend registra no log um token de uso único:

```bash
docker compose -f deploy/docker-compose.yml logs backend | grep setup_token
```

Abra <http://localhost:8080/admin>, informe o token, seu nome, e-mail e uma senha (12+ caracteres). Esse primeiro usuário recebe o papel **Administrador** (acesso total). Depois, em **Usuários** e **Papéis e permissões**, crie as contas da equipe. Quem recebe uma senha definida por um administrador escolhe a própria no primeiro login.

Perdeu o acesso? No servidor: `docker compose -f deploy/docker-compose.yml exec backend /api reset-password <e-mail>` (gera uma senha provisória). Detalhes em [docs/operations.md](docs/operations.md#acesso-e-usuários).

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

Backend ──► HTTPS (rede egress) ──► páginas de status de terceiros (summary.json)
```

- **Backend:** monólito modular em Go (`apps/backend`), SQL explícito com `pgx`, migrações versionadas e embutidas no binário. Detalhes em [docs/architecture.md](docs/architecture.md).
- **Frontend:** React + TypeScript + Vite (`apps/frontend`). Consome exclusivamente a API.
- **Decisões:** [docs/adr/](docs/adr/).

## API

Toda funcionalidade de negócio está disponível pela API. Resumo:

| Área | Endpoints |
|---|---|
| Saúde | `GET /healthz` (liveness), `GET /readyz` (inclui PostgreSQL) |
| Pública | `GET /api/v1/status`, `/components`, `/incidents[/{id}]`, `/maintenances[/{id}]`, `/announcements[/{id}]`, `/announcement-categories`, `/feed.atom`, `/feed.rss` |
| Inscrição | `GET /api/v1/subscriptions/options`, `POST /api/v1/subscriptions`, `POST .../confirm`, `POST/PUT .../manage`, `POST .../unsubscribe` (tokens dos links enviados por e-mail) |
| Componentes | `GET/POST /api/v1/admin/components`, `GET/PATCH .../{id}` (`group_id` associa a um grupo) |
| Grupos | `GET/POST /api/v1/admin/component-groups`, `GET/PATCH/DELETE .../{id}` (exclusão só de grupo vazio) |
| Incidentes | `GET/POST /api/v1/admin/incidents`, `GET/PATCH .../{id}`, `GET/POST .../{id}/updates`, `POST .../{id}/resolve` |
| Manutenções | `GET/POST /api/v1/admin/maintenances`, `GET/PATCH .../{id}`, `GET/POST .../{id}/updates`, `POST .../{id}/start\|complete\|cancel` |
| Templates | `GET/POST /api/v1/admin/incident-templates`, `GET/PATCH .../{id}` (idem para `maintenance-templates` e `announcement-templates`) |
| Comunicados | `GET/POST /api/v1/admin/announcements`, `GET/PATCH/DELETE .../{id}`, `POST .../{id}/publish\|unpublish\|archive\|restore`; categorias em `/api/v1/admin/announcement-categories` e times em `/api/v1/admin/teams` |
| Terceiros | `GET/POST /api/v1/admin/external-sources`, `GET/PATCH/DELETE .../{id}`, `POST .../{id}/sync`, `GET .../{id}/components`, `PUT .../{id}/selection` |
| Sessão | `GET/POST /api/v1/auth/setup`, `POST /api/v1/auth/login\|logout\|password`, `GET /api/v1/auth/me` |
| Acesso | `GET/POST /api/v1/admin/users`, `GET/PATCH .../{id}`, `POST .../{id}/password`; `GET/POST /api/v1/admin/roles`, `GET/PATCH/DELETE .../{id}`; `GET /api/v1/admin/permissions` |
| Times | `GET/POST /api/v1/admin/teams`, `GET/PATCH/DELETE .../{id}` (`team_id` em grupos e componentes) |
| Inscrições (admin) | `GET/POST /api/v1/admin/subscriptions`, `GET/PATCH/DELETE .../{id}`, `POST .../{id}/test` |
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
- **Comunicado:** `draft → published` (agendado enquanto `published_at` é futuro) `→ archived`; publicado ou agendado pode voltar a `draft`, arquivado pode ser restaurado. Só rascunhos são excluídos.
- **Status geral:** calculado por uma política única ([ADR 0003](docs/adr/0003-status-policy.md)). Manutenção nunca é apresentada como indisponibilidade.
- **Impacto:** incidentes e manutenções usam a mesma escala do status do componente: Operacional (`operational`, informativo), Performance degradada, Interrupção parcial e Interrupção grave. O seletor da administração mostra o ícone de cada nível.
- **Link do componente:** campo opcional `url` (http/https). Na página pública aparece como um ícone discreto ↗ ao lado do nome, abrindo em nova aba.
- **Descrição do componente:** interna por padrão; com `show_description`, aparece num ícone "i" discreto ao lado do nome.
- **Período do incidente:** `started_at` e `resolved_at` registram o início e o fim reais do impacto, que podem ser anteriores à abertura e ao encerramento na ferramenta. Os dois podem ser corrigidos depois, e a linha do tempo mantém o horário de cada publicação.
- **Grupos:** organizam componentes na página pública. Aparecem recolhidos, exibindo o status mais grave entre seus componentes; ao expandir, mostram os componentes.
- **Serviços de terceiros:** em `/admin/third-party`, cadastre a URL de uma página Atlassian Statuspage (ex.: `https://www.githubstatus.com`, `https://status.veracode.com`) e escolha os componentes a exibir. O backend consulta o provedor periodicamente; o status de terceiros aparece em seção própria e **não** altera o status geral ([ADR 0006](docs/adr/0006-third-party-status.md)).
- **Times responsáveis:** cada grupo ou componente pode ter um time (Squad, RT, Comunidade, Tribo…), com links de documentação e suporte e tags livres. Papéis com escopo "times" deixam cada time operar só os próprios componentes: status, incidentes, manutenções e comunicados ([ADR 0009](docs/adr/0009-team-ownership-and-scoped-roles.md)).
- **Inscrições:** o botão *Inscrever-se* oferece e-mail, Slack, webhook (JSON assinado com HMAC) e RSS/Atom, com filtro por componente e confirmação por e-mail. A entrega usa um outbox transacional com retentativas ([ADR 0010](docs/adr/0010-subscriptions-and-notification-outbox.md)). Configure SMTP e `PUBLIC_BASE_URL` em `deploy/.env` (veja [operações](docs/operations.md#inscrições-notificações)).
- **Comunicados:** avisos gerais que não são incidentes nem manutenções, escritos em `/admin/announcements` com categoria, time responsável, componentes relacionados, destaque, fixação e agendamento. O editor tem os modos visual (barra com negrito, itálico, sublinhado, tamanho, fonte, listas, citação, link e outros), markdown e pré-visualização ([ADR 0011](docs/adr/0011-announcement-text-format-and-editor.md)). Templates de comunicado preenchem o editor. A página `/announcements` agrupa por dia e o menu público (Componentes, Histórico, Comunicados) mostra quantos são novos para o visitante ([ADR 0007](docs/adr/0007-announcements.md)).

### Organização da página pública

1. Banner com o status geral, sempre visível.
2. **Acontecendo agora:** incidentes ativos e manutenções em andamento, só quando existem. Cada evento mostra a última atualização, e o restante da linha do tempo abre sob demanda.
3. **Componentes:** grupos recolhidos e componentes avulsos, intercalados pela ordem de exibição.
4. **Serviços de terceiros:** um item recolhido por provedor, só quando há algum configurado; ao expandir, mostra os componentes escolhidos, eventos abertos do provedor e o link para a página original.
5. **Manutenções programadas:** lista compacta, só quando há alguma.
6. **Histórico de eventos:** recolhido; é carregado ao expandir.

## Desenvolvimento sem Docker

Requisitos: Go 1.27+, Node.js 22.12+ ou 24 (LTS) com npm, PostgreSQL 16+.

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
| `EXTERNAL_STATUS_SYNC` | `true` | Leitura periódica das páginas de status de terceiros |
| `EXTERNAL_STATUS_INTERVAL` | `2m` | Intervalo da leitura (30s–1h) |
| `SESSION_COOKIE_SECURE` | `true` | Cookie de sessão só por HTTPS. O Compose local usa `false` (HTTP em 127.0.0.1) |
| `SESSION_TTL` | `12h` | Duração máxima da sessão (15m–720h) |
| `SESSION_IDLE_TIMEOUT` | `2h` | Encerra a sessão após inatividade (5m–`SESSION_TTL`) |
| `EXTERNAL_STATUS_ALLOW_PRIVATE_NETWORKS` | `false` | Permite provedores em redes privadas/loopback (veja [ADR 0006](docs/adr/0006-third-party-status.md)) |

## Testes e verificações

```bash
make test-backend      # unitários + validação da especificação OpenAPI
make test-integration  # suíte completa contra PostgreSQL (TEST_DATABASE_URL)
make test-frontend     # Vitest + Testing Library
make lint              # gofmt, go vet, ESLint
make build             # compila backend e frontend (inclui checagem de tipos)
make scan              # Trivy local (vulnerabilidades, segredos, configuração)
make generate-types    # regenera os tipos TypeScript a partir do OpenAPI
```

Os tipos da API usados pelo frontend (`apps/frontend/src/api/schema.gen.ts`) são **gerados** a partir de `apps/backend/api/openapi.yaml` ([ADR 0005](docs/adr/0005-typescript-types-from-openapi.md)). Depois de alterar o contrato, rode `make generate-types`: um teste do backend falha se o arquivo estiver desatualizado.

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

### Publicação de imagens

O workflow [`.github/workflows/publish.yml`](.github/workflows/publish.yml) publica as imagens no GHCR somente depois que o CI passou para o mesmo commit, em push para `main` ou em tag `v*` (nunca em pull requests). Antes do envio, a imagem é reconstruída a partir do commit aprovado e escaneada com Trivy. As imagens são multiplataforma (`linux/amd64` e `linux/arm64`) e vêm com proveniência e SBOM.

| Tag | Quando |
|---|---|
| `sha-<commit>` | Sempre. Imutável; é a tag a usar em deploys. |
| `main` | Push em `main`. |
| `X.Y.Z` e `X.Y` | Tag `vX.Y.Z`. |

`latest` não é publicada. Para rodar as imagens publicadas no lugar do build local:

```bash
docker login ghcr.io   # token com read:packages (os pacotes são privados enquanto o repositório for)
IMAGE_TAG=sha-<commit> docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.ghcr.yml up -d --no-build
```

### Deploy

**Ainda não existe workflow de deploy.** Ele depende de decisões que não estão no repositório: o destino (VPS, Kubernetes, serviço gerenciado), as credenciais, a política de aprovação por ambiente e a de rollback. Um ambiente remoto precisa de HTTPS (com `SESSION_COOKIE_SECURE=true`) para proteger senhas e sessões. Veja [docs/operations.md](docs/operations.md#deploy).

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

- Autenticação apenas local (sem SSO/OIDC, 2FA ou recuperação de senha por e-mail).
- Serviços de terceiros: apenas páginas no formato Atlassian Statuspage.
- Inscrições: sem SMS (exige provedor pago); e-mail, Slack e webhook pela página pública exigem SMTP configurado.
- Sem verificações automáticas de saúde ou cálculo de uptime.
- Sem deploy automatizado (a publicação de imagens existe).
