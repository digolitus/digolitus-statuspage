# ADR 0019 — Reportar suspeita

- **Status:** proposto
- **Data:** 2026-11-12

## Contexto

Usuários percebem falhas antes do monitoramento ou do time responsável. Queremos um canal no estilo "down detector": qualquer pessoa vê as suspeitas públicas em aberto e, com um login simples, reporta um problema. O time do componente faz a triagem no admin. A funcionalidade precisa ser fácil de ligar e desligar.

## Decisão

- **Conta de reportante separada dos usuários do admin.** Tabela `reporters`, login por link mágico no e-mail (vale 15 min, uso único, até 3 por 15 min por e-mail) ou Google. Sessão em cookie próprio `dsp_reporter` (HttpOnly, SameSite=Strict, 30 dias), independente do login do admin. Reportante bloqueado perde as sessões e não consegue entrar.
- **Agrupamento.** No máximo uma suspeita aberta (`pending` ou `published`) por componente, garantido por índice único parcial e advisory lock. Um relato novo soma-se a ela. Se a suspeita já virou incidente e o incidente não foi resolvido, o relato também é somado a ela. Cada pessoa reporta uma vez por suspeita.
- **Ciclo de vida.** `pending` (Pendente de triagem) → `published` (visível na lista pública, em análise) → `incident` (abre um incidente pré-preenchido e ligado) ou `closed` (motivo obrigatório: sem evidência de falha generalizada, duplicada ou fora de escopo, mais uma mensagem). Reportantes recebem e-mail ao finalizar, ao virar incidente e quando o incidente é resolvido.
- **Fila.** A suspeita vai para o time responsável pelo componente (componente, subgrupo ou grupo). Sem time, vai para a fila geral. Usuários com papel restrito a times só veem a própria fila.
- **Permissões.** Nova `suspicions.manage` (escopável por time), dada na migração a todo papel com `incidents.manage`. Abrir incidente também exige `incidents.manage`. Bloquear reportante exige a permissão global. Nova `features.manage` para ligar e desligar funcionalidades.
- **Conversa.** Anotações internas (nunca expostas) e mensagens com o reportante na mesma linha do tempo, visualmente separadas. Reportante responde por "Minhas suspeitas".
- **Privacidade.** A lista pública mostra só componente, contagens, distribuição dos sintomas e horários. Nome e e-mail ficam no admin.
- **Abuso.** Até 5 relatos por hora e 20 respostas por hora por reportante (429). Campos de escolha são listas fechadas; textos têm limite de tamanho.
- **Feature flag.** Tabela `feature_flags`, alterada em Configurações › Funcionalidades (começa desligada). `SUSPICIONS_ENABLED=false` trava a funcionalidade desligada no servidor, sem que o admin consiga religar. Desligada, as rotas respondem 404 `feature_disabled` e a página pública esconde o link.
- **E-mails.** Saída transacional própria (`reporter_mails`), entregue pelo mesmo worker de notificações, com backoff e até 8 tentativas.

## Alternativas descartadas

- **Usar os usuários do admin:** misturaria pessoas externas com quem opera o sistema.
- **Janela de tempo para agrupar (ex.: 30 min):** mais regras e suspeitas duplicadas para o mesmo problema. Uma suspeita aberta por componente é mais simples de entender.
- **Relato anônimo:** sem rastreio de quem abriu e sem canal de resposta.

## Consequências

- Para login com Google, cadastrar `{PUBLIC_BASE_URL}/api/v1/reporter/oauth/google/callback` como URI de redirecionamento no mesmo cliente OAuth do admin.
- O link mágico exige e-mail configurado. Sem e-mail e sem Google, ninguém consegue reportar.
- A rota só existe com a API de admin ligada.
