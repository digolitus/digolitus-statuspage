# Changelog

Gerado a partir dos commits (Conventional Commits) por `scripts/release.sh`.

## v0.1.0 (2026-11-12)

### Novidades

- **backend:** Período real do incidente, descrição pública opcional, tags de times e templates de comunicado ([225c16f](https://github.com/digolitus/digolitus-statuspage/commit/225c16f66a329631ecbe3f2bd3fb1e7a6667b0ed))
- **frontend:** Editor rico de comunicados, menu agrupado, 'i' de descrição, tags e período do incidente ([4bbd5d0](https://github.com/digolitus/digolitus-statuspage/commit/4bbd5d0733f97c7441f8b03404f4d6bbe80463f0))
- **auth:** Login social com Google, Apple e Facebook para usuários cadastrados ([5c60c4b](https://github.com/digolitus/digolitus-statuspage/commit/5c60c4b953a26741381636ab3cea6a0b91097750))
- **history:** Histórico de status efetivo, terceiros e uptime diário ([be52f0a](https://github.com/digolitus/digolitus-statuspage/commit/be52f0ab3bda8a93facdb9698f4fe7796f041d73))
- **components:** Subgrupos opcionais (Grupo -> Subgrupo -> Componente) ([ccb65d9](https://github.com/digolitus/digolitus-statuspage/commit/ccb65d99c7448367669d330c69948dd4782d8395))
- **frontend:** Uptime bars and status history screens ([2bbd9fe](https://github.com/digolitus/digolitus-statuspage/commit/2bbd9fef05eaaa989b695b1f099e73190b04846d))
- **admin:** Menu groups always start collapsed; sticky sidebar ([ba897a2](https://github.com/digolitus/digolitus-statuspage/commit/ba897a251d30f11beb454961af26e07932b102c5))
- **presence:** Quem está acessando agora (página pública e admin) ([cb474ba](https://github.com/digolitus/digolitus-statuspage/commit/cb474ba0795819b0b2759d286572b9304706b0e1))
- **frontend:** Componente InfoTip comum e prop info em Field e PageHeader ([c3661fb](https://github.com/digolitus/digolitus-statuspage/commit/c3661fbde0bf701e3cd4a8e06c14fafbf36e9592))
- **monitoring:** Integrações com UptimeRobot e Healthchecks.io ([afe6a28](https://github.com/digolitus/digolitus-statuspage/commit/afe6a282a72e9ce4909d4393dd4319e703d3297c))
- **teams:** Departamentos opcionais (até 4 níveis) acima dos times ([4fbb2e9](https://github.com/digolitus/digolitus-statuspage/commit/4fbb2e9466bd304b71608d3e2b90f8857ed4c5b6))
- **frontend:** Explicações das telas movidas para o ícone "i" ([7693769](https://github.com/digolitus/digolitus-statuspage/commit/76937697f0432ca0260bdf76da0d8e1d7f76c189))
- **branding:** Identidade visual configurável da página pública e do login ([dd94c7b](https://github.com/digolitus/digolitus-statuspage/commit/dd94c7b4d7d24ced46045fe22608f219219541d4))

### Correções

- Espaço não separável como escape; dependency review só com a variável habilitada ([721f1f4](https://github.com/digolitus/digolitus-statuspage/commit/721f1f42e32126b59844b12fd4af606dd111cc47))
- **frontend:** "i" do evento de terceiro junto ao selo ([d42f28a](https://github.com/digolitus/digolitus-statuspage/commit/d42f28a171b45ba84daac04002c7f967b9715dbc))

### Build e dependências

- **backend:** Go 1.27.3 (corrige GO-2026-6809 em crypto/x509) ([71c71bb](https://github.com/digolitus/digolitus-statuspage/commit/71c71bb0d4afba8420de268a4120361fd37d0512))
- **frontend:** Atualizar brotli-libs na imagem (CVE-2026-111497) ([15498b6](https://github.com/digolitus/digolitus-statuspage/commit/15498b6abf7a04ea415fe973dc93c4b9af0c3041))
- **release:** Notas de versão e tags com git-cliff, sem CI ([beefec7](https://github.com/digolitus/digolitus-statuspage/commit/beefec7075a9a729da8bd2602074701c483c2344))

### Documentação

- ADR 0011 (formato do texto e editor) e novas funcionalidades ([5536e93](https://github.com/digolitus/digolitus-statuspage/commit/5536e93de2bf92c735d7e19964a49c0157cc108d))

### Testes

- **frontend:** Restringir a busca do período ao resumo do incidente ([b9cf7c2](https://github.com/digolitus/digolitus-statuspage/commit/b9cf7c2cd402ee7ec2820b44a663e705e84710aa))

### CI

- Tags v* não disparam mais o CI ([35144ca](https://github.com/digolitus/digolitus-statuspage/commit/35144ca68f2a5d2be5e3a65ab776ace9fb774521))

## v0.0.1 (2026-10-18)

### Novidades

- **backend:** API v1 com PostgreSQL, incidentes, manutenções, templates e auditoria ([2ff43f5](https://github.com/digolitus/digolitus-statuspage/commit/2ff43f5772acf0307d0c60681dacc39a40cee211))
- **frontend:** Página pública com dados reais e área administrativa ([a86ddd5](https://github.com/digolitus/digolitus-statuspage/commit/a86ddd5d4dc5473a29d7053fe565f2e8778d38f1))
- **backend:** Grupos de componentes ([b6f595d](https://github.com/digolitus/digolitus-statuspage/commit/b6f595d64083fed388253342976911a0e93e8203))
- Tipos TypeScript gerados a partir do OpenAPI ([d3336a4](https://github.com/digolitus/digolitus-statuspage/commit/d3336a40e75157a4ab541cd9619aea4ec0ae3e74))
- **frontend:** Grupos recolhíveis e reorganização da página pública ([e516b40](https://github.com/digolitus/digolitus-statuspage/commit/e516b407bb3985f9edfeef3fdd2e533363a24bf1))
- **backend:** Serviços de terceiros via Atlassian Statuspage ([fe591fa](https://github.com/digolitus/digolitus-statuspage/commit/fe591faf5e6dcb603ad23cc8f89e2a77eaa14f56))
- **frontend:** Serviços de terceiros na administração e na página pública ([1c1837e](https://github.com/digolitus/digolitus-statuspage/commit/1c1837e125b578942c34db2bf93e6b678e42f658))
- **backend:** Escala única de impacto, link do componente e comunicados ([ac48c18](https://github.com/digolitus/digolitus-statuspage/commit/ac48c18765b1bfe504935c5d1003e3b5bdb9d7c8))
- **frontend:** Seletor de impacto com ícones, link do componente e comunicados ([87b6d2c](https://github.com/digolitus/digolitus-statuspage/commit/87b6d2c79c5d8b2d06e0b99e659e71cd5e5758c4))
- **backend:** Autenticação local e papéis com permissões (RBAC) ([8ed8c97](https://github.com/digolitus/digolitus-statuspage/commit/8ed8c97f6f74d343d97079ba23b344fdf7fa9cd7))
- **frontend:** Login, configuração inicial, usuários e papéis ([af12d6e](https://github.com/digolitus/digolitus-statuspage/commit/af12d6ef9d677816c4395305352686d63859dd05))
- **backend:** Times responsáveis com escopo e inscrições com outbox ([9c91843](https://github.com/digolitus/digolitus-statuspage/commit/9c91843917e2a8350f12582b0ff30d601d9b74aa))
- **frontend:** Inscrições, times e escopo; harmonização visual ([f6f48bd](https://github.com/digolitus/digolitus-statuspage/commit/f6f48bd5173145945e44ef948bde74bead50aa8b))

### Correções

- **frontend:** Corrige tipos dos mocks de teste que quebravam o build ([1a676a4](https://github.com/digolitus/digolitus-statuspage/commit/1a676a4010b4a4b0e953a5008f4c7c85e83113d7))
- **backend:** DB_MAX_CONNS lido como 32 bits, sem conversão estreitante (gosec G109) ([2392ae1](https://github.com/digolitus/digolitus-statuspage/commit/2392ae1462c516662ef9223f63431e8746726732))
- **frontend:** Após a configuração inicial, ir para a visão geral ([30506ba](https://github.com/digolitus/digolitus-statuspage/commit/30506ba0cdbc737ac95a0ad42ffaafabce6cda19))
- **subscriptions:** Pedido duplicado simultâneo responde 202 em vez de 500 ([12f31a2](https://github.com/digolitus/digolitus-statuspage/commit/12f31a25100a4d8233110837d97417a367779454))
- **frontend:** Manter TypeScript 6.0 (typescript-eslint não suporta 7) ([d9ae49a](https://github.com/digolitus/digolitus-statuspage/commit/d9ae49aa8d73ec59d721c640620cf25be0ce57a4))
- **frontend:** Atualizar libexpat na imagem (CVE-2026-77214) ([a075641](https://github.com/digolitus/digolitus-statuspage/commit/a07564179c88a14c5530adbe11ab6daae3be06bc))

### Build e dependências

- PostgreSQL no Compose, endurecimento de containers e CI ([7f54e41](https://github.com/digolitus/digolitus-statuspage/commit/7f54e41702f920c428162927355ba736c6493487))
- **frontend:** Atualiza para React 19, Vite 8, Vitest 5, ESLint 10 e TS 6 ([d0fa1b0](https://github.com/digolitus/digolitus-statuspage/commit/d0fa1b082b3c3bff69bab3d8f2c0e9ef2ae2d427))
- **deps:** Bump the go-minor-patch group ([b846a12](https://github.com/digolitus/digolitus-statuspage/commit/b846a1261ccc8a08eae51bad50e4f3c184f7a954))
- **deps-dev:** Bump typescript from 6.0.3 to 7.0.2 in /apps/frontend ([41fb06c](https://github.com/digolitus/digolitus-statuspage/commit/41fb06cc9d4e4c3f3869c327022440e11faa8396))
- **deps:** Bump node in /apps/frontend ([fc405bc](https://github.com/digolitus/digolitus-statuspage/commit/fc405bcb29c428727c4beb6ad61814ff074ef4ed))
- **deps:** Bump nginxinc/nginx-unprivileged in /apps/frontend ([300eec5](https://github.com/digolitus/digolitus-statuspage/commit/300eec5b6fa0d389fbda6120dd72d09456d24f49))
- **deps-dev:** Bump @testing-library/jest-dom in /apps/frontend ([83f649b](https://github.com/digolitus/digolitus-statuspage/commit/83f649b70653fb46ac69e967f96fb0aa4e89b5b8))
- **deps-dev:** Bump jsdom from 29.1.1 to 30.1.3 in /apps/frontend ([455f5b2](https://github.com/digolitus/digolitus-statuspage/commit/455f5b24c6eec297e71a4e0bf5fc7bb4dcdbd9a9))

### Documentação

- README fiel ao estado atual, arquitetura, operação e ADRs ([ee5feaa](https://github.com/digolitus/digolitus-statuspage/commit/ee5feaaa54a7a0a8d64aa9ebc7f0f89ab253170e))
- Grupos, organização da página, tipos gerados, publicação e deploy ([6a73867](https://github.com/digolitus/digolitus-statuspage/commit/6a7386719e39bf6901694ffd59b178d2a1af0c3a))
- ADR 0006 e documentação de serviços de terceiros ([1812fff](https://github.com/digolitus/digolitus-statuspage/commit/1812fff48fe60af090db31422f698c9ce5ea3911))
- ADR 0007 (comunicados), revisão da política de impacto e README ([8eb8422](https://github.com/digolitus/digolitus-statuspage/commit/8eb8422271697b7fdd9711b3e872e78562789f4c))
- ADR 0008 (autenticação e RBAC) substitui o ADR 0004; operação de acesso ([9123ac1](https://github.com/digolitus/digolitus-statuspage/commit/9123ac1ed92d3e08624d0c416b853d5211749c6d))
- ADRs 0009 e 0010, operação de times e inscrições, variáveis SMTP ([a98fc48](https://github.com/digolitus/digolitus-statuspage/commit/a98fc4829973eed3d5af8d98c3de83c0598c1ca7))

### Testes

- **frontend:** Respostas 204 sem corpo no mock e opções do seletor de impacto ([9ee7afa](https://github.com/digolitus/digolitus-statuspage/commit/9ee7afac9975616f42b9372a2db4e2c7167004b3))
- **frontend:** Conferir a última tentativa de login ([d89725a](https://github.com/digolitus/digolitus-statuspage/commit/d89725ad2aace667b58ef9273981c98c4f3a0798))

### CI

- Publica imagens no GHCR após CI aprovado ([4786e99](https://github.com/digolitus/digolitus-statuspage/commit/4786e997d7308109c873a631b7aebe5f1f9d6de3))
- Remove workflow temporário de geração do lockfile ([a15141b](https://github.com/digolitus/digolitus-statuspage/commit/a15141b7b0010da777c02ebe00b7f31db4e75c0d))
- Achados do gosec como anotações no PR; dependency review só quando disponível ([ff9756e](https://github.com/digolitus/digolitus-statuspage/commit/ff9756e2b56ef739f331b990961449ed95f70215))
- Smoke test aponta a verificação que falhou e cobre login obrigatório e setup ([cafcb39](https://github.com/digolitus/digolitus-statuspage/commit/cafcb397f774424b62aaa169ef5b873c302196ad))
- Smoke test informa a linha e o comando que falharam ([e85d145](https://github.com/digolitus/digolitus-statuspage/commit/e85d1456e4465dc5963c65b725a77bae563bebdf))
- Corrigir falso positivo de porta publicada no smoke test ([d0341a1](https://github.com/digolitus/digolitus-statuspage/commit/d0341a1b87352bb989e9e57a91ce18c407c608f8))
- Publicar achados do Trivy das imagens como anotações ([f4f162a](https://github.com/digolitus/digolitus-statuspage/commit/f4f162a8261a1632f9b7abc517e1976e6028ce14))
- Validar tudo em push na main e em tags (sem filtro por caminho) ([48fbf2a](https://github.com/digolitus/digolitus-statuspage/commit/48fbf2a01a02c86c68c6df16131879eed0e3586a))

### Outros

- **frontend:** Regenera package-lock.json após atualização de dependências ([44ed441](https://github.com/digolitus/digolitus-statuspage/commit/44ed44152e8391f4f6205f5badd176883b7ab30f))
- **deploy:** Rede de saída para o backend e variáveis EXTERNAL_STATUS_* ([c4b1864](https://github.com/digolitus/digolitus-statuspage/commit/c4b186426fcd7d5c759bdb2f6557bf0263340c58))
- **deploy:** Variáveis de sessão e instruções do primeiro acesso ([b661488](https://github.com/digolitus/digolitus-statuspage/commit/b66148832c1224bcfdb863fc810c32be608842e2))


