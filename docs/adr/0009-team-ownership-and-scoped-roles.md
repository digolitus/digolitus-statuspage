# ADR 0009 — Times responsáveis e papéis com escopo de time

- **Status:** aceito (complementa o [ADR 0008](0008-local-authentication-and-rbac.md))
- **Data:** 2026-10-18

## Contexto

Em instalações com muitos times (por exemplo, 20 squads com serviços próprios), cada time deve operar só os próprios componentes: status, incidentes, manutenções e comunicados. Os papéis do ADR 0008 valem para a plataforma inteira, o que obriga a escolher entre dar acesso demais ou centralizar a operação.

## Decisão

- **Times como entidade da plataforma.** Um time tem nome, tipo livre (Squad, RT, Comunidade, Tribo…), descrição e links de documentação e de suporte. Grupos e componentes podem ter um time responsável, opcional.
- **Dono efetivo de um componente** = `COALESCE(componente.team_id, grupo.team_id)`. O componente herda o time do grupo quando não tem um próprio.
- **Membros.** Usuários pertencem a zero ou mais times (`user_teams`).
- **Escopo do papel.** Cada papel tem `scope`:
  - `global`: vale para tudo;
  - `team`: vale só para os recursos dos times do usuário.

  Só permissões de operação são "escopáveis":
  - `components.status` e `components.manage`;
  - `incidents.manage` e `maintenances.manage`;
  - `announcements.write` e `announcements.publish`.

  Gestão de usuários, papéis, times, templates, terceiros e inscrições continua só global.
- **Onde a regra é aplicada:**
  - A guarda da rota exige a permissão em qualquer escopo.
  - Os serviços de domínio confirmam o escopo no recurso (`auth.RequireComponents`, `RequireTeam`, `RequireGroup`), dentro da transação.
  - Com uma permissão só de time, todos os componentes afetados precisam ser dos times do usuário e a lista não pode ser vazia, porque um evento sem componente afeta a plataforma toda.
  - Na criação e na edição, a regra vale antes e depois da mudança, para que ninguém "doe" nem "tome" um componente de outro time.
  - A violação responde 403 `outside_team_scope`, com mensagem clara.
- **Interface.** `GET /auth/me` informa `team_permissions` e os times do usuário. Com isso a interface esconde as ações fora do escopo e oferece só os componentes do time nos seletores. A API continua sendo a autoridade.

## Consequências

- **Responsabilidade compartilhada sem novos serviços.** Continua sendo um monólito modular com uma consulta extra por operação com escopo.
- **Papéis existentes não mudam:** viram `global`. A migração `0009` cria o papel "Operador do time" como ponto de partida.
- **Componentes sem dono** (nem próprio nem do grupo) só podem ser operados por papéis globais. Isso é intencional.
- **Exclusão de times.** Um time em uso por componentes, grupos ou comunicados não pode ser excluído (FK `RESTRICT` e 409). Ele deve ser desativado.
- **Escopo por mais de um time.** Fica para depois, se necessário (por exemplo, um papel ligado a times específicos e não aos times do usuário).
