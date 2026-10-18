# ADR 0003 — Política de status declarado

- **Status:** aceito
- **Data:** 2026-10-18

## Contexto

A página precisa comunicar o status geral e o de cada componente a partir de três fontes: o status declarado do componente, os incidentes ativos e as manutenções em andamento. Não existem verificações automáticas, então nada pode ser apresentado como disponibilidade verificada. Também era preciso evitar que uma manutenção num único componente marcasse a plataforma inteira como indisponível.

## Decisão

A política está implementada em um único lugar (`internal/statuspolicy`), como função pura e coberta por testes de tabela.

1. **Status efetivo do componente** = o mais grave entre:
   - o status declarado;
   - o status implícito de cada incidente ativo que o afeta (`minor` → `degraded_performance`, `major` → `partial_outage`, `critical` → `major_outage`);
   - `maintenance`, se houver manutenção em andamento que o afete.
2. **Gravidade:** `major_outage` > `partial_outage` > `degraded_performance` > `maintenance` > `operational`. Assim, manutenção nunca esconde uma indisponibilidade.
3. **Status geral** = o mais grave entre os componentes públicos e ativos, considerando também incidentes ativos que não afetam nenhum componente público (o impacto deles vale para a plataforma).
4. Se o mais grave for `maintenance`, o status geral é `maintenance`, comunicado como "manutenção em N componentes" e nunca como indisponibilidade.
5. Sem componentes públicos e sem incidentes ativos, o status geral é `unknown`. A plataforma não afirma estar operacional sem ter o que declarar.
6. A resposta inclui `overall.basis = "declared"`, e a interface informa que não há monitoramento automático.

Manutenções programadas (ainda não iniciadas) não alteram o status.

## Alternativas consideradas

- **Status geral como média ou maioria dos componentes:** esconderia indisponibilidades graves.
- **Incidentes não influenciarem o status do componente:** obrigaria o operador a atualizar dois lugares e geraria inconsistências.
- **Manutenção com a mesma gravidade de indisponibilidade parcial:** contraria o requisito de não alarmar por trabalho planejado.

## Consequências

- Resolver um incidente restaura automaticamente o status declarado.
- Se o status declarado de um componente for pior que o dos incidentes, ele prevalece até que um operador o altere.

## Reavaliar quando

- Houver verificações automáticas. A política precisará distinguir fontes (`basis`), definir a idade máxima de resultados e o tratamento de falhas transitórias.
