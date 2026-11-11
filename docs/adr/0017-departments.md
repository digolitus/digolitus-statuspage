# ADR 0017 — Departamentos opcionais acima dos times

- **Status:** aceito
- **Data:** 2026-11-11

## Contexto

Organizações grandes agrupam times em diretorias, gerências e coordenações. O admin só tinha uma lista plana de times ([ADR 0009](0009-team-ownership-and-scoped-roles.md)), sem como mostrar essa estrutura nem onde ficam pessoas sem time (por exemplo, um gerente).

## Decisão

- **Hierarquia opcional de até 4 níveis.** Tabela `departments` com `parent_id`. O nível é a profundidade na árvore; não é guardado. O serviço recusa ciclos e árvores com mais de 4 níveis, inclusive ao mover um departamento com filhos.
- **Nomes dos níveis são rótulos** (`department_levels`, ex.: Diretoria › Gerência › Coordenação). Nível sem nome aparece como “Nível N” e continua utilizável.
- **Times** ficam em no máximo um departamento, de qualquer nível, ou em nenhum (`teams.department_id`, `ON DELETE RESTRICT`).
- **Pessoas** aparecem pelos times e também podem ficar direto num departamento (`user_departments`). `users.job_title` guarda um cargo livre.
- **Só organização.** Nenhuma permissão ou regra de dono depende de departamento. A gestão usa `teams.manage`; o vínculo pelo usuário usa `users.manage`. Tudo é auditado (`resource_type = department`).
- **Exclusão** só de departamento sem filhos e sem times (409 `department_in_use`); pessoas ligadas direto são desvinculadas.
- **Leitura** carrega a árvore inteira (limite de 200 departamentos) e calcula nível, caminho e contagens em memória. Mudanças estruturais travam a tabela (`SHARE ROW EXCLUSIVE`) para que as checagens de profundidade e ciclo vejam uma árvore estável.

## Consequências

- Nada muda para quem não usa departamentos.
- A página pública não mostra departamentos. Pode ser adicionado depois.
- Escopo de papel por departamento (ex.: gerente opera todos os times abaixo) fica para depois, se necessário; a árvore já permite calcular o conjunto de times.
