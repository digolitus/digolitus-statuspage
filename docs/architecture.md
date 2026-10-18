# Arquitetura

Descreve a estrutura **atual**. Princípios gerais ficam no [`CONTEXT.md`](../CONTEXT.md); decisões com trade-offs duradouros, em [`adr/`](adr/).

## Visão geral

Um monólito modular em Go expõe a API REST, que é o contrato central do produto, e persiste tudo em PostgreSQL. O frontend React é um cliente dessa API, sem regras de negócio exclusivas. O Nginx serve o frontend e faz proxy para o backend.

## Backend (`apps/backend`)

```text
cmd/api            ponto de entrada: `serve` (padrão) e `migrate`
cmd/healthcheck    probe para o container distroless (sem shell)
api/               openapi.yaml (contrato) embutido no binário
internal/
  app/             composição: rotas, middlewares, health; testes de contrato e integração HTTP
  statuspolicy/    política de status (pura, sem I/O)
  statuspage/      resumo público (/api/v1/status) em snapshot consistente
  component/       componentes: modelo, validação, SQL, casos de uso, handlers
  incident/        incidentes e linha do tempo
  maintenance/     manutenções e transições
  templates/       templates de incidente e manutenção
  audit/           histórico operacional (gravação e consulta)
  announcement/    comunicados, categorias e times responsáveis
  auth/            login, sessões, configuração inicial, usuários, papéis e catálogo de permissões
  external/        serviços de terceiros: cadastro, leitura de Statuspage (anti-SSRF) e poller
  tsgen/           gerador de tipos TypeScript a partir do OpenAPI (cmd/openapi-ts)
  docs/            Swagger UI embutida (assets oficiais vendorizados)
  apperror/        vocabulário de erros esperados (validação, 404, 409)
  platform/        infraestrutura compartilhada:
    database/      pool pgx, transações, migrações embutidas
    httpx/         roteador com registro de rotas, JSON, erros, paginação, middlewares
    clock/         relógio do servidor (UTC, microssegundos) e parsing de instantes/timezones
    testsupport/   banco PostgreSQL isolado por teste
```

Cada módulo de domínio segue o mesmo formato enxuto, sem interfaces artificiais:

- **Modelo e validação** (`<módulo>.go`): tipos, regras e transições. Não dependem de HTTP.
- **Persistência** (`store.go`): SQL explícito e parametrizado, sobre a interface `database.Querier`. A mesma função roda dentro ou fora de uma transação.
- **Casos de uso** (`service.go`): orquestram a transação. A alteração, a entrada na linha do tempo e o registro de auditoria são gravados juntos, de forma atômica. Linhas são bloqueadas com `SELECT ... FOR UPDATE` antes de transições, o que serializa operações concorrentes.
- **HTTP** (`http.go`): decodificação, chamada do caso de uso e serialização.

### Pipeline HTTP

`X-Request-ID` (aceito se válido, senão gerado) → log de acesso (JSON, sem query string) → recuperação de pânico → headers de segurança (`nosniff`, `no-store`) → timeout por contexto → roteador. Corpos são limitados a 64 KiB, campos desconhecidos são rejeitados e erros seguem um envelope único com `code` estável. Rotas inexistentes e métodos não permitidos também respondem em JSON.

As rotas administrativas passam por um único wrapper (`app.New`). Ele valida a sessão (`auth.Guard`), recusa requisições de outros sites, aplica a permissão da rota conforme `adminPolicy` (`internal/app/policy.go`, negando o que não estiver listado) e coloca o usuário no contexto, para que a auditoria registre o autor ([ADR 0008](adr/0008-local-authentication-and-rbac.md)).

### Modelo de dados

| Tabela | Papel |
|---|---|
| `components` | Estado atual declarado de cada componente; nunca é excluído (`is_active`); `group_id` e `url` opcionais |
| `component_groups` | Grupos de apresentação; status derivado dos componentes; excluível só quando vazio |
| `incidents` | Estado atual do incidente (`status`, `resolved_at` coerentes por `CHECK`) |
| `incident_components` | Componentes afetados atualmente |
| `incident_updates` | Linha do tempo publicada, imutável, com snapshot dos componentes quando alterados |
| `maintenances` | Janela prevista (UTC + timezone IANA de apresentação) e instantes reais de início/fim/cancelamento |
| `maintenance_components` | Componentes afetados |
| `maintenance_updates` | Linha do tempo publicada, imutável |
| `incident_templates`, `maintenance_templates` | Textos reutilizáveis; nomes únicos sem diferenciar maiúsculas |
| `announcements` | Comunicados (`draft`/`published`/`archived`; agendado = `published_at` futuro), com categoria e time opcionais |
| `announcement_categories`, `teams` | Classificação e responsáveis; nomes únicos sem diferenciar maiúsculas; em uso não são excluídos |
| `announcement_components` | Componentes relacionados a um comunicado |
| `external_sources` | Provedores de terceiros (URL base `https`), último resultado da leitura e eventos abertos |
| `external_components` | Componentes importados do provedor; `is_selected` define o que é publicado; `is_present = false` quando sumiram do provedor |
| `users`, `user_roles` | Contas locais (hash Argon2id, bloqueio por tentativas, senha provisória) e seus papéis |
| `roles` | Papéis com subconjunto do catálogo de permissões; o papel de sistema concede todas |
| `sessions` | Sessões do servidor; guarda só o SHA-256 do token do cookie |
| `setup_tokens` | Token de uso único da configuração inicial (só o hash) |
| `audit_events` | Histórico operacional com diff antes/depois, autor, origem e request ID |
| `schema_migrations` | Versão e checksum de cada migração aplicada |

O estado atual fica separado do histórico: editar um incidente nunca reescreve atualizações já publicadas, e incidentes copiam o texto do template em vez de referenciá-lo.

Instantes são `timestamptz`, gerados pelo relógio do servidor em UTC e truncados para microssegundos. Horários de manutenção informados como hora local são interpretados no timezone IANA enviado junto. Nem o timezone da máquina nem o do navegador entram nesse cálculo.

### Migrações

Os arquivos ficam em `internal/platform/database/migrations` (`NNNN_nome.sql`) e são embutidos no binário:

- São aplicados em ordem, cada um na sua transação, sob `pg_advisory_lock`.
- O checksum de cada migração aplicada é registrado. Alterar uma migração já aplicada é erro.
- Um binário mais antigo se recusa a rodar sobre um esquema mais novo.
- As migrações só avançam: uma correção é sempre uma nova migração.

## Frontend (`apps/frontend`)

```text
src/api/         tipos gerados do OpenAPI (schema.gen.ts) + apelidos, cliente HTTP (timeout, cancelamento, ApiError) e endpoints
src/lib/         formatação (rótulos pt-BR, datas com timezone), roteador mínimo, hooks de carga e polling
src/components/  UI compartilhada (selos com ícone + texto, estados vazio/erro/carregando, confirmação, timeline)
src/public/      página pública e páginas de detalhe
src/admin/       área administrativa
```

- **Roteamento:** um roteador mínimo sobre a History API, sem dependência extra. Nginx e Vite fazem fallback para `index.html`.
- **Atualização da página pública:** a cada 60 s, uma requisição por vez, pausada com a aba oculta. Em caso de falha, o intervalo dobra a cada erro até 5 min. Se uma atualização falha, os dados anteriores continuam visíveis e marcados como desatualizados, com o horário de geração.
- **Formulários:** validação básica no cliente, para dar retorno rápido. A API continua sendo a autoridade, e seus erros por campo são exibidos junto aos campos.
- **Ações importantes** (resolver, iniciar, concluir, cancelar, desativar) pedem confirmação explícita.

## Contrato e testes

- `api/openapi.yaml` é escrito à mão e versionado ([ADR 0002](adr/0002-openapi-contract.md)).
- `internal/app/spec_test.go` valida a especificação e compara as rotas registradas com as documentadas, nos dois sentidos.
- Os testes de integração HTTP sobem a aplicação real contra PostgreSQL e validam cada requisição e resposta com `kin-openapi`. Os schemas de resposta usam `additionalProperties: false`, então campos não documentados falham.
