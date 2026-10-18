# ADR 0006 — Status de serviços de terceiros lido de páginas Atlassian Statuspage

- **Status:** aceito
- **Data:** 2026-10-18

## Contexto

A plataforma depende de serviços externos (GitHub, Veracode etc.). Quem acompanha a página precisa ver o estado desses serviços sem sair dela, como nos "third-party components" do Atlassian Statuspage. Muitos provedores publicam páginas Statuspage, que expõem o endpoint público `GET /api/v2/summary.json` com componentes, incidentes e manutenções.

Ler URLs informadas pela administração é um vetor de SSRF. Desde o [ADR 0008](0008-local-authentication-and-rbac.md), só usuários com a permissão `third_party.manage` cadastram provedores; as proteções abaixo continuam valendo para eles.

## Decisão

- Um módulo `internal/external` cadastra **provedores** (`external_sources`) pela URL base da página e importa seus componentes (`external_components`). A administração escolhe quais componentes publicar.
- O backend consulta `summary.json` periodicamente (padrão: 2 min, `EXTERNAL_STATUS_INTERVAL`) e grava um snapshot. O frontend nunca chama o provedor diretamente: a página pública continua dependendo apenas da nossa API e da nossa CSP.
- Vários processos podem rodar o poller: cada ciclo reserva provedores com `FOR UPDATE SKIP LOCKED`.
- **Status de terceiros não entra no status geral nem no dos grupos.** Ele aparece em seção própria (`third_party` em `/api/v1/status`), recolhida por provedor, com o status informado pelo provedor, eventos abertos que afetam os componentes escolhidos e link para a página original.
- Se a última leitura bem-sucedida for mais antiga que `max(3 × intervalo, 10 min)`, o provedor é marcado como desatualizado (`stale`) e seus componentes aparecem como `unknown`. Não exibimos como "operacional" um dado que não conseguimos confirmar.
- Componentes que somem do provedor são mantidos com `is_present = false` (a seleção não se perde); grupos do provedor viram apenas um rótulo (`group_name`).

Controles contra SSRF e abuso:

- Somente `https`, sem credenciais na URL; hosts que são IP literal, `localhost`, `.local` e `.internal` são recusados no cadastro.
- No momento da conexão, o dialer recusa endereços não públicos (loopback, privados, link-local, CGNAT, multicast etc.), o que também cobre DNS rebinding. Proxy de ambiente ignorado; até 3 redirecionamentos, todos `https` e revalidados.
- Timeout por requisição, resposta limitada a 2 MiB, no máximo 500 componentes e 10 eventos por provedor, 50 provedores.
- `EXTERNAL_STATUS_ALLOW_PRIVATE_NETWORKS=true` libera redes privadas, apenas para páginas internas conhecidas.
- No Compose, só o backend ganha uma rede de saída (`egress`); o PostgreSQL continua sem acesso à internet.

## Alternativas consideradas

- **Embutir a página do provedor (iframe/widget):** dependeria de CSP e cookies de terceiros, quebraria o layout e não permitiria escolher componentes.
- **Buscar no navegador:** exporia os visitantes ao provedor, exigiria abrir `connect-src` na CSP e não teria histórico nem detecção de dado desatualizado.
- **Somar ao status geral:** um problema do GitHub faria a nossa plataforma parecer indisponível, sem que tenhamos declarado isso. Se no futuro for desejado, deve ser uma regra explícita por componente, decidida pela operação.
- **Fila ou serviço separado de coleta:** desnecessário no volume atual; o poller dentro do monólito com `SKIP LOCKED` basta.

## Consequências

- O backend passa a ter tráfego de saída, o que precisa ser permitido na infraestrutura de destino.
- Hoje só o formato Atlassian Statuspage é suportado (`provider = 'statuspage'`). Outros formatos (RSS, APIs próprias) entram como novos fetchers, sem mudar a apresentação.
- Enquanto a administração não tiver autenticação, qualquer cliente que a alcance pode fazer o backend consultar URLs `https` públicas. As proteções acima limitam esse risco a hosts públicos.
