# ADR 0013 — Subgrupos opcionais de componentes

- **Status:** aceito
- **Data:** 2026-11-11

## Contexto

Os grupos de componentes ([migração 0003](../../apps/backend/internal/platform/database/migrations/0003_component_groups.sql)) tinham um único nível. Áreas maiores (ex.: "Pagamentos") precisam separar seus componentes em blocos menores ("Cartões", "Pix") sem obrigar todo componente a ter esse nível intermediário. A hierarquia pedida é **Grupo → Subgrupo (opcional) → Componente**, com subgrupos gerenciados no mesmo menu de grupos.

## Decisão

- **Mesma tabela.** Um subgrupo é uma linha de `component_groups` com `parent_id` preenchido. Nome, descrição, ordem e time funcionam igual nos dois níveis, e a API continua em `/api/v1/admin/component-groups` (campo `parent_id`).
- **Dois níveis, não uma árvore.** `parent_id` só aponta para um grupo principal. Transformar grupo em subgrupo (ou o contrário) só é permitido com o grupo vazio (`409 component_group_level_locked`). Mover um subgrupo para outro grupo é permitido e leva seus componentes.
- **`components.group_id` continua sendo o grupo principal**; `components.subgroup_id` é opcional. Clientes que só conhecem grupos seguem funcionando: agrupamentos, filtros e contagens por `group_id` incluem os componentes dos subgrupos.
- **Integridade no banco:** a FK composta `(subgroup_id, group_id) → component_groups (id, parent_id)` garante que o subgrupo pertence ao grupo do componente, e `ON UPDATE CASCADE` move os componentes quando o subgrupo muda de grupo. As regras de nível são aplicadas no serviço com bloqueio das linhas envolvidas.
- **Nomes únicos por pai:** índice único `(parent_id, lower(name)) NULLS NOT DISTINCT`. "API" pode existir em grupos diferentes.
- **Dono efetivo:** time do componente, senão o do subgrupo, senão o do grupo.
- **Página pública:** `groups` em `/api/v1/status` traz grupos e subgrupos (com `parent_id`), cada um com status derivado de seus componentes (`statuspolicy.Group`); `components` traz `subgroup_id`.

## Alternativas consideradas

- **Tabela `component_subgroups` separada:** duplicaria CRUD, validação, auditoria e tela, sem ganho real.
- **Árvore de profundidade livre (`parent_id` recursivo, `group_id` apontando para qualquer nível):** exigiria consultas recursivas e mudaria o significado de `group_id` para os clientes atuais. Pode ser revisto se surgir a necessidade de mais níveis.

## Consequências

- `Component`, `PublicComponent`, `ComponentGroup` e `PublicComponentGroup` ganham campos novos (`subgroup_id`, `parent_id`, `subgroup_count`); mudança aditiva no contrato.
- A movimentação de componentes por cascata (ao mover um subgrupo) é registrada na auditoria do subgrupo, não em cada componente.
