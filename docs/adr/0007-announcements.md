# ADR 0007 — Comunicados gerais

- **Status:** aceito
- **Data:** 2026-10-18

## Contexto

A equipe precisa avisar os clientes sobre assuntos que não são incidentes nem manutenções: novidades, mudanças de política, descontinuações, lembretes. Esses avisos não devem alterar nenhum status, precisam de uma página própria organizada por dia e de um sinal de "novo" na navegação.

## Decisão

- Um módulo `internal/announcement`, separado de incidentes e manutenções, com três recursos: comunicados, categorias e times responsáveis. Comunicados nunca entram na política de status.
- **Ciclo de vida:** rascunho → publicado (com `published_at` futuro o estado apresentado é `scheduled`) → arquivado. Um comunicado publicado ou agendado pode voltar a rascunho, e um arquivado pode ser restaurado com a data original. Só rascunhos podem ser excluídos. Transições usam `SELECT ... FOR UPDATE` e são auditadas na mesma transação, como nos demais módulos.
- **Agendamento sem job:** o estado "agendado" é derivado de `published_at > agora` na consulta. Não há processo em segundo plano que possa falhar ou atrasar a publicação.
- **Categorias** têm uma paleta fechada de cores (sem CSS arbitrário). **Categorias e times em uso** não são excluídos (`ON DELETE RESTRICT`, 409); são desativados e deixam de ser oferecidos. Associações existentes são mantidas.
- **Texto com formatação leve** (parágrafos, listas, negrito, links http/https), armazenado como texto puro e renderizado pelo frontend como nós React. Nenhum HTML vindo do banco é injetado na página.
- **Página pública** `/announcements`: fixados no topo, demais agrupados por dia no fuso do visitante, filtro por categoria e página por comunicado.
- **Sinal de novidade:** o menu público mostra quantos comunicados foram publicados depois do último que o visitante viu. Essa marca fica no `localStorage` do navegador. Sem registro, contam como novos os dos últimos 14 dias. É uma conveniência por visitante: não exige conta nem cookies no servidor, e a página funciona normalmente quando o armazenamento está bloqueado.

## Alternativas consideradas

- **Reaproveitar incidentes com impacto "nenhum":** misturaria avisos com o histórico de disponibilidade e com a política de status.
- **Markdown completo ou HTML no servidor:** aumentaria a superfície de XSS e exigiria uma biblioteca de sanitização. O subconjunto atual cobre o uso esperado.
- **Notificações por e-mail ou push:** exigem assinaturas, consentimento e infraestrutura de envio. Ficam para quando houver esse requisito.

## Consequências

- O indicador de novidade é por navegador: o mesmo visitante em outro dispositivo verá os comunicados como novos.
- Editar um comunicado publicado altera o texto exibido; a página mostra "atualizado em" e a auditoria guarda o diff.
