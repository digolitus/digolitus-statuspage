# CONTEXT.md --- Digolitus Status Page

> **Objetivo:** fornecer contexto duradouro de produto e engenharia para
> pessoas e assistentes de IA que trabalham no Digolitus Status Page.
> Este documento estabelece princípios, critérios de decisão e regras de
> colaboração --- não uma arquitetura fixa nem uma lista imutável de
> funcionalidades.
>
> **Repositório:** https://github.com/digolitus/digolitus-statuspage\
> **Nome do produto:** Digolitus Status Page\
> **Proprietário do repositório:** `digolitus`

------------------------------------------------------------------------

## 1. Como utilizar este documento

Trate este arquivo como a bússola de engenharia de longo prazo do
projeto. Ele descreve a direção do produto, o padrão de qualidade, os
princípios arquiteturais e a forma esperada de trabalho.

-   O **repositório atual** é a fonte da verdade sobre o que está
    implementado hoje.
-   Este documento orienta como avaliar mudanças e tomar decisões
    futuras.
-   Código, testes, configurações de implantação e comportamento real
    verificado têm prioridade sobre suposições ou descrições
    desatualizadas.
-   Não presuma que uma funcionalidade planejada já existe.
-   Não preserve uma arquitetura antiga apenas porque ela está descrita
    aqui. Mude-a quando os requisitos, as evidências ou a escala
    justificarem uma solução melhor.
-   Quando este documento entrar em conflito com uma decisão explícita
    mais recente ou com restrições reais do repositório, identifique o
    conflito e resolva-o deliberadamente, sem fazer suposições
    silenciosas.

Antes de realizar um trabalho significativo, inspecione apenas as partes
relevantes do repositório e utilize o menor contexto suficiente para
tomar uma decisão confiável.

## 2. Visão do produto

O Digolitus Status Page tem como objetivo evoluir para uma aplicação
confiável para comunicar e compreender a disponibilidade e a saúde de
serviços.

O produto poderá crescer além de sua implementação inicial. Entre as
possíveis áreas de evolução estão status de serviços e componentes,
verificações periódicas de saúde, gestão do ciclo de vida de incidentes,
histórico de disponibilidade, notificações, integrações, controle de
acesso, painéis operacionais e outras capacidades. Essas são
possibilidades, **não compromissos de implementação imediata**.

O produto deve conquistar confiança apresentando informações precisas,
compreensíveis, atualizadas e devidamente contextualizadas. Nunca
apresente dados simulados, desatualizados ou inferidos como se fossem um
status atual verificado.

A arquitetura deve permitir uma evolução significativa sem exigir uma
reescrita extensa a cada nova funcionalidade. Ao mesmo tempo, evite
construir infraestrutura especulativa para recursos que ainda não foram
justificados.

### Princípios para decisões de produto

Ao avaliar uma funcionalidade ou solução: 1. Esclareça o problema do
usuário e o resultado esperado. 2. Identifique o que precisa ser
preciso, atualizado, seguro e observável. 3. Comece pela menor solução
completa que atenda à necessidade. 4. Preserve um caminho razoável de
evolução caso os requisitos cresçam. 5. Considere custo operacional,
manutenção, usabilidade e modos de falha --- não apenas a velocidade de
implementação. 6. Diferencie funcionalidades obrigatórias de melhorias
opcionais.

## 3. Valores de engenharia

Aplique estes valores de forma consistente:

-   **Correção antes da aparência:** comportamento, contratos e precisão
    dos status importam mais que uma sensação superficial de completude.
-   **Simplicidade antes da burocracia:** escolha o desenho menos
    complexo que atenda aos requisitos reais.
-   **Explicitude antes da mágica:** torne compreensíveis a
    configuração, as dependências, os efeitos colaterais e os
    comportamentos de falha.
-   **Modularidade antes da fragmentação:** separe responsabilidades sem
    criar serviços, pacotes ou abstrações desnecessários.
-   **Segurança por padrão:** minimize privilégios, exposição, segredos
    e fronteiras de confiança.
-   **Testabilidade desde o início:** facilite a validação
    determinística dos comportamentos importantes.
-   **Prontidão operacional:** considere logs, saúde, encerramento
    gracioso, configuração e recuperação como parte da implementação.
-   **Evolução incremental:** prefira mudanças que possam ser revisadas,
    testadas e revertidas.
-   **Evidências antes de suposições:** diferencie fatos verificados,
    hipóteses, propostas e questões em aberto.
-   **Manutenibilidade antes da esperteza:** prefira código idiomático e
    padrões conhecidos por futuros colaboradores.

## 4. Princípios de arquitetura

Nenhum estilo arquitetural é universalmente superior. Escolha a
arquitetura com base na complexidade real do domínio, nas necessidades
da equipe, nos requisitos de confiabilidade, nas restrições de
implantação e no custo de mudança.

### 4.1 Comece por fronteiras claras

Identifique as responsabilidades e os contratos de cada parte do
sistema. Mantenha as regras de negócio independentes do transporte, da
persistência, dos frameworks de interface e da infraestrutura sempre que
isso trouxer um benefício real.

Defina intencionalmente a direção das dependências. O comportamento
central do domínio não deve precisar conhecer Docker, GitHub Actions,
roteadores HTTP ou um banco de dados específico, a menos que isso faça
parte de uma necessidade legítima do domínio.

Não crie interfaces apenas para seguir um padrão. Introduza abstrações
quando elas isolarem uma fronteira relevante, melhorarem os testes ou
reduzirem o custo de uma mudança provável.

### 4.2 Prefira um monólito modular até existir motivo para distribuir

Monorepo é uma escolha de organização do código, não uma exigência para
criar microsserviços.

Prefira módulos coesos, com responsabilidades e dependências explícitas.
Separe serviços implantáveis apenas quando houver uma razão concreta,
como escalabilidade independente, isolamento de falhas, fronteiras de
segurança, ciclos de entrega distintos ou responsabilidades
organizacionais diferentes.

Evite a complexidade de sistemas distribuídos --- retentativas de rede,
consistência eventual, descoberta de serviços e transações entre
serviços --- até que o produto realmente precise dela.

### 4.3 Separe domínio e infraestrutura

Quando a complexidade do domínio justificar, considere separar: -
Conceitos de domínio e regras de negócio. - Casos de uso e orquestração
da aplicação. - Adaptadores para HTTP, persistência, APIs externas e
mensageria. - Configuração e composição na inicialização da aplicação.

Não crie mecanicamente todas essas camadas em cada pacote. Para uma
funcionalidade simples, código direto pode ser melhor. Adicione
estrutura conforme a quantidade e a complexidade das regras
justificarem.

### 4.4 Defina contratos explicitamente

Nas interfaces internas e externas, deixe claros os dados de entrada e
saída, os erros, as validações, os limites de tempo e as expectativas de
compatibilidade.

Para APIs HTTP, considere: - Esquemas de resposta estáveis. - Códigos
HTTP e tipos de conteúdo apropriados. - Validação de entradas e limites
de tamanho das requisições. - Respostas de erro consistentes. - Timeouts
e cancelamento. - Versionamento apenas quando a necessidade de
compatibilidade justificar. - Documentação e testes de contrato quando
forem úteis.

Evite quebrar consumidores de forma inesperada. Quando uma mudança
incompatível for necessária, explique o impacto e ofereça um caminho de
migração quando viável.

### 4.5 Gerencie estado e consistência deliberadamente

Quando houver persistência ou fluxos distribuídos, defina a propriedade
dos dados, as expectativas de consistência, a retenção, a estratégia de
migração e a recuperação.

Não adicione banco de dados, fila, cache ou barramento de eventos sem
uma necessidade clara. Quando algum deles for necessário, compare opções
realistas considerando complexidade operacional, modos de falha, custo e
escala esperada.

### 4.6 Registre decisões importantes

Para escolhas com consequências relevantes de longo prazo, utilize
Registros de Decisão Arquitetural (ADRs) ou um registro equivalente e
conciso. Inclua: - Contexto e restrições. - Decisão. - Alternativas
consideradas. - Consequências e trade-offs. - Condições que
justificariam reavaliar a decisão.

Não crie um ADR para cada detalhe pequeno de implementação. Mantenha as
decisões importantes fáceis de localizar e atualizadas.

## 5. Tecnologias e evolução do repositório

O projeto foi iniciado com backend em Go, frontend compilado
separadamente, imagens Docker multistage, Docker Compose para
desenvolvimento local, GitHub Actions para CI/CD e GHCR como preferência
inicial para registry de imagens.

Trate essas escolhas como a direção atual, não como regras imutáveis.
Inspecione o repositório antes de assumir a stack, as versões, a
estrutura de diretórios, os nomes dos workflows ou o destino de
implantação exatos.

Ao considerar uma mudança tecnológica: 1. Explique o problema concreto
que ela resolve. 2. Compare-a com a abordagem atual. 3. Considere custo
de migração, operação, segurança, experiência de desenvolvimento e
reversibilidade. 4. Prefira tecnologias maduras, mantidas e bem
documentadas. 5. Evite adicionar dependências por conveniência quando já
existir uma solução pequena e sustentável. 6. Não substitua tecnologia
consolidada apenas por tendência.

Mantenha fronteiras entre módulos e contextos de build claros. Novos
módulos devem poder aderir ao repositório sem exigir duplicação
generalizada da lógica de testes, segurança, build e release.

## 6. Expectativas para o backend

Para código Go: - Siga as convenções idiomáticas da linguagem e utilize
a biblioteca padrão quando ela for suficiente. - Mantenha os pacotes
focados e evite dependências circulares. - Trate erros explicitamente e
adicione contexto sem ocultar a causa original. - Propague cancelamento
de contexto e prazos quando relevante. - Valide a configuração na
inicialização e falhe de forma clara quando uma configuração obrigatória
estiver ausente ou inválida. - Utilize logs úteis, preferencialmente
estruturados, sem expor segredos ou conteúdo sensível. - Configure
timeouts HTTP, limites de requisição e encerramento gracioso quando
apropriado. - Evite estado mutável compartilhado sem sincronização e
propriedade claramente definidas. - Teste comportamento observável,
casos extremos e caminhos de falha. - Evite frameworks e camadas
desnecessárias.

Não adicione concorrência, cache, retentativas ou workers em segundo
plano sem definir seu ciclo de vida, seus limites, seu comportamento de
falha e seus testes.

## 7. Expectativas para o frontend

Para o frontend: - Separe adequadamente componentes de interface,
gerenciamento de estado, comunicação com APIs e apresentação de regras
específicas do domínio. - Prefira contratos tipados e estados explícitos
de carregamento, sucesso, vazio, desatualizado e erro quando
relevantes. - Não apresente dados simulados como status real do
sistema. - Evite incorporar URLs específicas de ambiente ou segredos no
bundle do cliente. - Inclua acessibilidade, navegação por teclado,
responsividade e comunicação clara dos status no desenvolvimento
normal. - Evite requisições de rede duplicadas e polling sem controle. -
Se houver polling ou atualizações em tempo real, defina frequência,
timeout, cancelamento, backoff, comportamento para dados desatualizados
e impacto no servidor. - Teste comportamentos relevantes do usuário,
preferencialmente sem depender de detalhes internos de implementação.

Utilize complexidade visual apenas quando ela melhorar a compreensão.
Indicadores de status não devem depender exclusivamente de cores.

## 8. Confiabilidade e observabilidade

Trate falhas como condições normais de operação que o sistema deve
administrar deliberadamente.

À medida que o produto crescer, considere: - Semântica de liveness e
readiness. - Timeouts e retentativas limitadas. - Encerramento
gracioso. - Logs estruturados e identificadores úteis de correlação. -
Métricas de comportamento importantes. - Rastreamento distribuído quando
a investigação entre componentes justificar. - Limites de taxa e de
recursos. - Distinção clara entre estados saudável, degradado,
desconhecido, desatualizado e indisponível. - Procedimentos de
recuperação e resposta a incidentes.

Não adicione automaticamente uma plataforma de observabilidade ou uma
infraestrutura extensa de telemetria. Instrumente os comportamentos que
ajudam a responder perguntas operacionais concretas.

Para funcionalidades de status e monitoramento, defina o que cada
verificação realmente comprova, quando ocorreu o último sucesso, a idade
do resultado, o significado das falhas e o tratamento de erros
transitórios. Evite afirmações falsas de disponibilidade.

## 9. Segurança e privacidade

A segurança é uma restrição contínua de projeto, abrangendo código,
dependências, sistema de build, artefatos e implantação.

Aplique privilégio mínimo, defesa em profundidade e configurações
seguras por padrão.

-   Nunca versione segredos reais, tokens, chaves privadas ou
    credenciais de produção.
-   Utilize gestão de segredos apropriada a cada ambiente e credenciais
    de curta duração quando suportadas.
-   Valide e limite entradas não confiáveis.
-   Implemente autenticação e autorização quando funcionalidades
    expuserem ações ou dados protegidos.
-   Evite registrar credenciais, dados pessoais ou conteúdo sensível de
    requisições.
-   Mantenha dependências e imagens-base atualizadas.
-   Execute containers como usuário não root sempre que viável.
-   Minimize o conteúdo das imagens, as capacidades, a exposição de rede
    e as áreas graváveis quando possível.
-   Reavalie as fronteiras de ameaça ao adicionar integrações, webhooks,
    contas de usuário ou funções administrativas.
-   Considere prevenção de abuso, limitação de taxa e esgotamento de
    recursos à medida que a exposição aumentar.
-   Trate conteúdo controlado por usuários e respostas de serviços
    externos como entradas não confiáveis.

Não afirme que um scanner comprova que o sistema é seguro. Explique a
cobertura relevante, as exclusões e os riscos residuais.

## 10. Testes e critérios de qualidade

Escolha testes com base no risco e no comportamento observável, não
apenas em uma meta percentual de cobertura.

Quando apropriado, utilize uma estratégia em camadas: - Testes unitários
para regras de negócio e comportamentos isolados. - Testes de integração
para fronteiras reais, como bancos de dados ou adaptadores HTTP. -
Testes de contrato para compatibilidade de APIs. - Testes ponta a ponta
para jornadas críticas do usuário. - Validação de configurações e
containers. - Análise de segurança e revisão de dependências.

Os testes devem ser determinísticos, repetíveis e isolados de sistemas
de produção. Evite testes dependentes de serviços externos ativos, a
menos que tenham sido explicitamente projetados para isso.

Ao corrigir um bug, adicione um teste de regressão quando for viável.

Um build bem-sucedido não comprova correção. Informe quais verificações
foram realmente executadas e quais não foram.

## 11. CI/CD e cadeia de suprimentos de software

GitHub Actions é a plataforma inicial de automação, e GHCR é o registry
de containers preferencial neste momento. Mantenha essas escolhas
substituíveis quando isso for prático, sem adicionar abstrações
desnecessárias.

O CI/CD deve: - Validar mudanças antes da publicação. - Executar
verificações nos módulos afetados. - Ampliar as validações quando
arquivos compartilhados ou workflows forem alterados. - Evitar
reconstruir módulos sem relação com a mudança, sem justificativa. -
Utilizar cache de dependências e camadas Docker com segurança. - Aplicar
permissões mínimas a cada job. - Separar a execução de Pull Requests não
confiáveis da publicação privilegiada. - Fixar Actions em referências
imutáveis verificadas, conforme a política do repositório. - Utilizar
tags rastreáveis e digests das imagens. - Produzir SBOMs e proveniência
de build quando úteis e suportadas. - Publicar somente artefatos que
passaram pelas verificações obrigatórias. - Promover o mesmo artefato
validado entre ambientes sempre que possível. - Manter o deploy separado
do build e da publicação. - Tornar exceções visíveis, justificadas e
auditáveis.

Utilize controles de segurança complementares. SAST, análise de
dependências, análise de configuração, detecção de segredos e análise de
vulnerabilidades de containers são controles diferentes; um não
substitui todos os demais.

Não enfraqueça uma verificação obrigatória apenas para deixar o pipeline
verde. Se uma regra produzir ruído ou for impraticável, investigue e
melhore a política em vez de ignorá-la silenciosamente.

## 12. Docker e experiência do desenvolvedor

O projeto deve continuar simples de clonar, construir, testar e executar
localmente.

Prefira builds reproduzíveis, contextos pequenos, Dockerfiles multistage
quando apropriado, versões de imagens-base controladas e containers de
execução sem privilégios elevados.

O desenvolvimento local não deve exigir publicação de imagens em um
registry remoto. O Compose deve poder construir imagens locais a partir
do código-fonte quando esse for o fluxo esperado.

Documente os comandos essenciais para iniciar, parar, consultar logs,
testar, executar lint, construir e analisar a segurança. Mantenha os
comandos alinhados ao repositório; não documente comandos inexistentes.

Evite montar código-fonte ou ferramentas de desenvolvimento nos
containers de produção. Separe desenvolvimento e produção sem criar
duplicação desnecessária de configuração.

## 13. Configuração, dependências e migrações

A configuração deve ser explícita, validada e separada do código da
aplicação.

-   Forneça valores padrão seguros apenas quando forem apropriados.
-   Falhe rapidamente quando uma configuração obrigatória estiver
    ausente ou inválida.
-   Mantenha arquivos de exemplo livres de credenciais reais.
-   Evite múltiplas fontes concorrentes de verdade.
-   Documente mudanças de configuração e seus efeitos.

Adicione dependências somente quando trouxerem valor relevante.
Considere manutenção, licença, histórico de segurança, dependências
transitivas, compatibilidade e custo operacional.

Quando persistência ou contratos externos forem introduzidos, planeje
migrações e compatibilidade. Evite alterações destrutivas de esquema ou
dados sem uma estratégia de recuperação ou migração.

## 14. Documentação e gestão do conhecimento

A documentação deve ajudar alguém a tomar uma decisão ou operar o
sistema, não apenas repetir o código.

Mantenha: - Um README conciso para orientação e comandos comuns. -
Documentação arquitetural para a estrutura e as fronteiras atuais do
sistema. - ADRs para decisões relevantes. - Runbooks para tarefas
operacionais e recuperação. - Documentação de APIs e configurações
quando necessária. - Este `CONTEXT.md` para princípios duradouros e
orientações de colaboração.

Atualize a documentação quando comportamento, contratos, configuração,
arquitetura ou operação mudarem.

Diferencie funcionalidades planejadas de funcionalidades implementadas.
Remova ou corrija afirmações desatualizadas quando se tornarem
enganosas.

## 15. Uso eficiente de contexto e tokens em desenvolvimento assistido por IA

Utilize contexto e tokens de forma deliberada. O objetivo não é
minimizar toda resposta a qualquer custo, mas reduzir trabalho
irrelevante preservando correção, segurança e raciocínio útil.

### 15.1 Recupere contexto progressivamente

Siga esta ordem: 1. Leia este arquivo para entender os princípios
duradouros do projeto. 2. Inspecione os diretórios, pontos de entrada,
testes e configurações relevantes. 3. Pesquise símbolos, contratos ou
usos específicos relacionados à tarefa. 4. Leia apenas os trechos
necessários dos arquivos relevantes. 5. Amplie a investigação somente
quando as evidências revelarem dependências adicionais ou incerteza.

Não leia todo o repositório, todos os workflows, todas as dependências
ou toda a documentação para uma mudança pequena e localizada.

Em mudanças transversais, inspecione todas as fronteiras afetadas antes
de editar. Contexto enxuto é um objetivo, não uma justificativa para
ignorar dependências importantes.

### 15.2 Evite redescobertas repetidas

Não redescubra repetidamente informações já estabelecidas na sessão
atual ou registradas em um artefato confiável do projeto.

Antes de repetir um comando ou reler um arquivo extenso, avalie: - O
arquivo relevante mudou? - O resultado anterior ainda é válido? - O
comando resolverá uma incerteza real? - Um comando mais específico
responderia à pergunta?

Reaproveite descobertas verificadas, resultados de testes, caminhos de
arquivos e decisões arquiteturais, verificando se continuam atuais.

Não confie em detalhes lembrados quando o código ou a configuração podem
ter mudado. Verifique o trecho relevante.

### 15.3 Mantenha as saídas das ferramentas pequenas e direcionadas

Prefira: - Pesquisar um símbolo em vez de despejar um diretório
inteiro. - Ler uma função ou seção específica em vez de um arquivo
grande completo. - Executar testes focados durante as primeiras
iterações. - Resumir saídas de comandos em vez de repetir logs
extensos. - Inspecionar o diff em vez de imprimir todos os arquivos
alterados novamente. - Utilizar diagnósticos direcionados em vez de
repetir builds completos quando a falha estiver localizada.

Execute validações mais abrangentes antes de concluir uma mudança,
especialmente quando ela atravessar módulos ou afetar segurança, builds,
contratos ou implantação.

Não resuma nem trunque os detalhes de erro necessários para diagnosticar
uma falha.

### 15.4 Planeje antes de editar

Para tarefas pequenas, utilize uma lista mental breve e prossiga.

Para tarefas médias ou grandes: 1. Declare de forma concisa o objetivo e
os critérios de aceite. 2. Identifique os arquivos e as dependências
provavelmente afetados. 3. Escolha a menor implementação coerente. 4.
Faça as mudanças relacionadas em conjunto. 5. Valide o resultado. 6.
Revise o diff e resuma os resultados.

Evite planos longos e especulativos quando for seguro começar com uma
etapa pequena e verificável. Não produza várias arquiteturas
alternativas, a menos que a decisão realmente dependa de um trade-off.

### 15.5 Evite código e saídas desnecessários

Não: - Repita a tarefa completa ou este documento em todas as
respostas. - Reproduza arquivos-fonte grandes quando um diff focado for
suficiente. - Gere boilerplate, camadas, interfaces ou abstrações sem
necessidade concreta. - Adicione dependências ou workflows apenas porque
são comuns em outros projetos. - Repita testes sem alteração ou motivo
que justifique a execução. - Produza explicações longas para comandos
rotineiros. - Crie documentação duplicada que entre em conflito com a
fonte de verdade existente. - Relate detalhadamente cada comando
bem-sucedido.

Prefira resumos concisos e acionáveis, com caminhos para os arquivos
alterados quando o ambiente permitir.

### 15.6 Mantenha mudanças delimitadas e fáceis de revisar

Implemente a menor mudança completa que satisfaça os critérios de
aceite.

Evite formatação, renomeações, atualizações de dependências e
refatorações sem relação com a tarefa, a menos que sejam necessárias
para a correção ou tenham sido solicitadas explicitamente.

Quando uma refatoração mais ampla for benéfica, explique por que ela é
necessária e considere separá-la da mudança funcional.

Revise o diff para detectar edições acidentais, arquivos gerados,
segredos, alterações inesperadas em lockfiles e mudanças sem relação com
a tarefa.

### 15.7 Valide progressivamente

Durante o desenvolvimento, execute primeiro as verificações relevantes
mais rápidas. Depois que a mudança estiver estável, execute as
verificações mais abrangentes apropriadas.

Por exemplo: - Teste unitário focado ao alterar um handler. - Testes
relevantes do frontend ao alterar um componente. - Build e lint do
módulo após a implementação local. - Validação de integração e Compose
em mudanças que afetem comunicação entre serviços. - Validação de
segurança e workflows em mudanças de pipeline ou dependências. - Testes
de regressão mais amplos antes de concluir.

Não afirme que verificações não executadas passaram. Se uma verificação
estiver indisponível ou for cara demais para executar, informe a
limitação e indique a melhor próxima etapa de validação.

### 15.8 Preserve conhecimento duradouro em vez de depender do histórico da conversa

Se uma decisão for importante para sessões futuras, registre-a no
artefato duradouro apropriado: - Decisões arquiteturais em ADRs. -
Arquitetura atual na documentação de arquitetura. - Procedimentos
operacionais em runbooks. - Princípios estáveis de engenharia em
`CONTEXT.md`. - Critérios específicos de uma tarefa na própria tarefa ou
issue.

Não copie a conversa inteira para a documentação do projeto. Registre
apenas decisões duradouras, restrições e justificativas úteis.

Não atualize este arquivo com detalhes temporários de uma tarefa ou cada
escolha de implementação. Mantenha-o estável e aplicável a diferentes
situações.

### 15.9 Comunique-se com eficiência

Para tarefas rotineiras, informe: - O que mudou. - O que foi validado. -
Ressalvas importantes.

Para tarefas complexas, informe também: - A decisão principal e sua
justificativa. - Riscos ou trade-offs. - Trabalho restante e
dependências externas.

Use linguagem concisa. Aprofunde a explicação somente quando a tarefa
exigir análise mais profunda, uma decisão relevante ou diagnóstico de
problemas.

Faça perguntas apenas quando a informação ausente puder alterar
materialmente a solução ou gerar risco significativo. Caso contrário,
adote uma premissa razoável e reversível e informe-a brevemente.

**Regra de eficiência:** otimize o progresso útil por unidade de
contexto --- não o menor texto possível, e nunca pulando inspeções,
verificações de segurança, testes ou raciocínio necessários.

## 16. Acordo de trabalho para assistentes de IA

Ao trabalhar neste repositório:

1.  Inspecione antes de alterar.
2.  Entenda os contratos e as dependências relevantes.
3.  Prefira a solução mais simples que atenda à necessidade real.
4.  Torne explícitas as premissas importantes.
5.  Mantenha as mudanças focadas e reversíveis.
6.  Adicione ou atualize testes para comportamentos relevantes.
7.  Execute validações pertinentes e inspecione o diff.
8.  Atualize a documentação duradoura quando necessário.
9.  Relate os resultados verificados com honestidade.
10. Nunca afirme que uma ação, teste, análise, publicação ou implantação
    ocorreu sem que isso tenha realmente acontecido.

Não faça alterações remotas ou destrutivas --- como pushes, releases,
implantações em produção, exclusão de dados ou mudanças nas
configurações do repositório --- sem autorização explícita.

## 17. Definição de boa engenharia

Uma boa solução não é necessariamente aquela com mais camadas,
abstrações, ferramentas ou arquivos.

Uma boa solução: - Resolve o problema real. - Tem responsabilidades e
contratos claros. - Trata falhas esperadas. - É testável e segura. -
Atende à escala atual sem impedir uma evolução razoável. - Pode ser
compreendida por outro engenheiro. - Pode ser operada e diagnosticada. -
Evita complexidade e custos de manutenção desnecessários. - Tem
documentação proporcional à necessidade. - É respaldada por evidências
honestas de validação.

Utilize estes princípios para orientar a evolução do Digolitus Status
Page, mantendo abertura para mudanças significativas de produto e
arquitetura quando os requisitos e as evidências as justificarem.
