# ADR 0014 — Histórico de status e uptime diário

- **Status:** proposto
- **Data:** 2026-11-11

## Contexto

O produto só guardava o estado atual. A auditoria (`audit_events`) registra o status declarado, mas não o status efetivo, que também depende de incidentes e manutenções ([ADR 0003](0003-status-policy.md)). Também não há registro do que os provedores de terceiros informaram ao longo do tempo. Os pedidos são:

- o histórico do status de cada componente;
- o histórico dos serviços de terceiros;
- na página pública, uma barra diária por componente com seletor de 30, 60 e 90 dias e o percentual de uptime.

## Decisão

**Componentes**

- `component_status_changes` guarda cada mudança do **status efetivo**, só acrescentando linhas: status anterior e novo, causa (`component`, `incident`, `maintenance` ou `baseline`), id da causa, autor e instante. `status = NULL` marca o componente desativado, que conta como "sem dados".
- O status efetivo só muda por escritas explícitas: nada inicia ou termina sozinho. Por isso cada serviço que pode mudá-lo chama `statushistory.Record` no fim da sua transação.
  - `Record` recalcula todos os componentes com `statuspolicy.Evaluate` e grava apenas as diferenças em relação ao último registro.
  - O histórico é gravado se, e somente se, a mudança for confirmada.
  - Uma única regra de status continua valendo.
- `pg_advisory_xact_lock` serializa os recálculos. Em READ COMMITTED, quem espera o lock lê, ao recebê-lo, tudo o que o detentor anterior confirmou. Assim o último registro é sempre o status em vigor, mesmo com incidentes e alterações de componentes concorrentes. O `changed_at` nunca é anterior ao registro anterior do componente.
- Na subida, `statushistory.Baseline` registra os componentes ainda sem histórico. A operação é idempotente. O histórico começa no deploy, sem reconstruir o passado.

**Uptime**

- `GET /api/v1/status/uptime?days=30|60|90&timezone=<IANA>` calcula, por dia no fuso pedido, os segundos em cada status, o pior status do dia e os incidentes do dia.
- O uptime segue a regra do Atlassian Statuspage: interrupção grave conta 100% como indisponível e interrupção parcial conta 30%. Performance degradada e manutenção não descontam.
- O cálculo é uma função pura (`statushistory.Daily`), feito na leitura sobre os intervalos. Não há agregados materializados.
- A barra de grupo e subgrupo é apresentação: o pior status do dia entre os componentes e a média do uptime deles.

**Terceiros**

- A cada leitura bem-sucedida, `saveSnapshot` grava em `external_component_status_changes` a mudança de status de cada componente do provedor. Também mantém `external_events`, com os incidentes e manutenções vistos: primeira e última vez em que foram vistos, e `closed_at` quando o provedor deixa de informá-los.
- O nome do provedor e do componente é copiado e as chaves estrangeiras usam `ON DELETE SET NULL`, para o registro sobreviver à exclusão do provedor.
- Exibição: o que está em andamento aparece como estado atual. Os históricos (`/outages` e `/events`) listam só o que já foi resolvido. Os períodos de indisponibilidade são derivados das mudanças gravadas.

## Alternativas consideradas

- **Gatilho no banco:** o status efetivo depende de três tabelas e da política de status. Repetir essa regra em SQL criaria duas fontes de verdade.
- **Derivar da auditoria:** a auditoria guarda diffs heterogêneos do status declarado e não o efetivo. Seria uma reconstrução frágil.
- **Agregados diários materializados:** desnecessários no volume atual, porque 90 dias de mudanças por componente cabem numa consulta indexada. Ficam como evolução se a leitura pesar, por exemplo com um cache na borda.
- **Gravar terceiros só ao resolver:** perderia o início exato do problema e a duração real. A regra "só ao resolver" vale para a exibição.

## Consequências

- Escritas de componentes, incidentes e manutenções passam por um lock global curto. No volume esperado (operações humanas) isso é irrelevante, mas deve ser reavaliado se houver alterações automáticas em alta frequência, como monitoramento.
- Uma nova fonte de status efetivo, como o monitoramento automático, deve chamar `Record` e ser considerada no cálculo dentro dele.
- Períodos em que o poller não conseguiu ler o provedor mantêm o último status conhecido no histórico de terceiros.
