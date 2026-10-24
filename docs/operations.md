# Operação (runbook local)

## Iniciar, acompanhar e parar

```bash
docker compose -f deploy/docker-compose.yml up --build        # ou: make up
docker compose -f deploy/docker-compose.yml ps                 # estado/saúde
docker compose -f deploy/docker-compose.yml logs -f backend    # logs JSON do backend
docker compose -f deploy/docker-compose.yml down               # para; dados preservados
```

A ordem de inicialização é garantida por health checks: PostgreSQL pronto (`pg_isready`) → backend pronto (`/readyz`, que inclui o banco) → frontend.

## Saúde

| Endpoint | Significado |
|---|---|
| `GET /healthz` | O processo responde HTTP. Não consulta o banco, para que uma queda do banco não reinicie o backend. |
| `GET /readyz` | Pronto para atender: PostgreSQL respondeu em até 2 s. Retorna 503 `database_unavailable` caso contrário. |

## Migrações

As migrações são aplicadas automaticamente na inicialização (`DB_AUTO_MIGRATE=true`). Para aplicá-las manualmente:

```bash
docker compose -f deploy/docker-compose.yml run --rm backend migrate   # ou: make migrate
```

O processo falha de forma explícita se uma migração já aplicada foi modificada, ou se o banco tem migrações que o binário desconhece (binário mais antigo que o esquema).

## Dados

- Os dados ficam no volume nomeado `pgdata` e sobrevivem a `down` e a reinícios.
- **Apagar todos os dados locais** é destrutivo e irreversível:

  ```bash
  make db-reset   # pede confirmação; equivale a: docker compose -f deploy/docker-compose.yml down --volumes
  ```

- Backup local, por exemplo:

  ```bash
  docker compose -f deploy/docker-compose.yml exec db pg_dump -U statuspage statuspage > backup.sql
  ```

## Correlação de logs

O Nginx gera um `X-Request-ID` para cada requisição e o encaminha ao backend. O backend registra o ID nos logs, devolve no header da resposta, inclui em respostas de erro (`error.request_id`) e grava no histórico operacional. Para investigar um erro relatado na interface, use o ID exibido para buscar nos logs.

## Acesso e usuários

A administração exige login. As decisões estão no [ADR 0008](adr/0008-local-authentication-and-rbac.md).

**Primeiro acesso.** Enquanto não existe usuário, o backend registra um token de uso único a cada inicialização:

```bash
docker compose -f deploy/docker-compose.yml logs backend | grep setup_token
# ou gere outro token (vale o mais recente):
docker compose -f deploy/docker-compose.yml exec backend /api setup-token
```

Em `/admin`, informe o token e os dados do primeiro administrador. Depois que ele é criado, o token deixa de valer e a tela de configuração não aparece mais.

**Papéis.** Em *Papéis e permissões*, combine as permissões do catálogo. O banco começa com três papéis:

| Papel | Permissões |
|---|---|
| Administrador (fixo) | Todas, inclusive usuários e papéis |
| Operador de incidentes | Incidentes, manutenções e status de componentes |
| Gestor de catálogo e comunicados | Componentes e grupos, status, serviços de terceiros, templates, comunicados (escrever e publicar), categorias e times |
| Operador do time (escopo: times) | Status, incidentes, manutenções e comunicados, só nos componentes dos times do usuário |

Usuários sem papel só consultam os dados. Ninguém concede permissões que não tem. *Gerenciar usuários* e *Gerenciar papéis* são sensíveis: dê-as só a quem pode decidir quem faz o quê.

**Times e responsabilidade compartilhada** ([ADR 0009](adr/0009-team-ownership-and-scoped-roles.md)).

1. Cadastre os times em *Times responsáveis*: nome, tipo, descrição, documentação e suporte.
2. Vincule cada componente ou grupo ao seu time. O componente sem time herda o time do grupo.
3. Em *Usuários*, marque os times de cada pessoa e conceda um papel com escopo "Componentes dos times do usuário".

Essas pessoas só alteram status e só abrem incidentes, manutenções e comunicados dos componentes dos próprios times. Fora do escopo, a API responde 403 `outside_team_scope`.

**Senhas e bloqueio.** As senhas têm de 12 a 128 caracteres. Após 5 falhas seguidas, a conta fica bloqueada por 15 minutos; um administrador pode desbloqueá-la em *Usuários*. Senhas definidas por um administrador (criação ou redefinição) precisam ser trocadas no primeiro login.

**Recuperação** (ninguém consegue entrar, por exemplo o único administrador esqueceu a senha):

```bash
docker compose -f deploy/docker-compose.yml exec backend /api reset-password admin@empresa.com
```

O comando imprime uma senha provisória, desbloqueia e reativa a conta e encerra as sessões dela. A operação fica no histórico com a origem `cli`.

**Sessões.** O cookie `dsp_session` é `HttpOnly` e `SameSite=Strict`. Atrás de HTTPS, use `SESSION_COOKIE_SECURE=true`; o Compose local usa `false` porque serve HTTP em `127.0.0.1`. `SESSION_TTL` (padrão 12 h) limita a duração total da sessão, e `SESSION_IDLE_TIMEOUT` (padrão 2 h) a inatividade. Desativar um usuário, redefinir sua senha ou ele trocar a própria senha encerra as outras sessões dele.

**Login social (Google, Apple, Facebook).** Opcional, só para usuários já cadastrados: a pessoa entra com a conta do provedor que tem o mesmo e-mail do usuário ([ADR 0012](adr/0012-social-sign-in.md)). Exige `PUBLIC_BASE_URL` e HTTPS (a Apple não aceita HTTP). Cadastre no provedor o *redirect URI* `PUBLIC_BASE_URL/api/v1/auth/oauth/<google|apple|facebook>/callback`, que o backend também registra no log ao iniciar (`social sign-in enabled`).

- **Google:** em Google Cloud Console → APIs e serviços → Credenciais, crie um *ID do cliente OAuth* do tipo "Aplicativo da Web" e defina `OAUTH_GOOGLE_CLIENT_ID` e `OAUTH_GOOGLE_CLIENT_SECRET`.
- **Apple:** em Apple Developer → Certificates, IDs & Profiles, crie um *Services ID* com "Sign in with Apple" (domínio e *return URL*) e uma *Key* com "Sign in with Apple". Defina `OAUTH_APPLE_CLIENT_ID` (o Services ID), `OAUTH_APPLE_TEAM_ID`, `OAUTH_APPLE_KEY_ID` e `OAUTH_APPLE_PRIVATE_KEY_FILE` (caminho do `.p8` montado no contêiner, por exemplo como *secret*).
- **Facebook:** em Meta for Developers, crie um app com "Login do Facebook", cadastre a URI de redirecionamento OAuth válida e defina `OAUTH_FACEBOOK_APP_ID` e `OAUTH_FACEBOOK_APP_SECRET`.

Falhas aparecem na tela de login, e o motivo fica no log (`social login refused` ou `social login failed`, com `reason`). Na Apple, quem escolhe "Ocultar meu e-mail" não corresponde ao cadastro. Trocar o e-mail de um usuário desfaz os vínculos dele com os provedores.

**Controles complementares.** O Nginx limita login, login social e configuração inicial a 10 tentativas por minuto por IP, além dos limites da API, do corpo de 64 KiB e dos timeouts. `ADMIN_API_ENABLED=false` remove toda a administração (API pública somente leitura). O Compose continua publicando apenas em `127.0.0.1`.

## Inscrições (notificações)

As decisões estão no [ADR 0010](adr/0010-subscriptions-and-notification-outbox.md).

O botão **Inscrever-se** da página pública oferece estes canais:

- **E-mail, Slack e webhook:** exigem SMTP. Cada pedido é confirmado por e-mail (double opt-in).
- **RSS/Atom:** `/api/v1/feed.atom` e `/api/v1/feed.rss`, sempre disponíveis.

SMS não é oferecido, porque exige provedor pago.

**Configuração mínima** (em `deploy/.env`):

```bash
PUBLIC_BASE_URL=https://status.empresa.com   # usado nos links dos e-mails
SMTP_HOST=smtp.empresa.com
SMTP_PORT=587
SMTP_TLS=starttls          # tls para 465; none só para servidores de teste locais
SMTP_USERNAME=status@empresa.com
SMTP_PASSWORD=...          # segredo: nunca versione
SMTP_FROM="Status Empresa <status@empresa.com>"
```

**Entrega.**

- O worker (`NOTIFY_WORKER=true`, a cada `NOTIFY_INTERVAL`, padrão `10s`) envia as mensagens registradas na mesma transação de cada publicação. Ele pode rodar em várias réplicas.
- Falhas são repetidas com backoff. Webhook ou Slack com 15 falhas seguidas é desativado, e o motivo aparece em *Inscrições*.
- Webhooks em rede privada exigem `NOTIFY_ALLOW_PRIVATE_NETWORKS=true`.

**Administração** (permissão *Gerenciar inscrições*): em *Inscrições* você consulta os inscritos, desativa, reativa, exclui e envia testes. Também cria integrações internas de webhook ou Slack, ativas sem confirmação. O segredo de assinatura do webhook é exibido uma única vez.

**Validação no receptor do webhook:**

1. Calcule `HMAC-SHA256(segredo, X-Digolitus-Timestamp + "." + corpo)`.
2. Compare o resultado com `X-Digolitus-Signature` (`sha256=<hex>`).
3. Recuse timestamps antigos.
4. Deduplique por `X-Digolitus-Delivery`.

O Nginx limita `/api/v1/subscriptions` a 6 requisições por minuto por IP.

## Serviços de terceiros

O backend lê `https://<provedor>/api/v2/summary.json` a cada `EXTERNAL_STATUS_INTERVAL` (padrão `2m`, entre `30s` e `1h`). Para isso ele usa a rede `egress` do Compose; o PostgreSQL continua isolado. Em outra infraestrutura, libere saída HTTPS (443) apenas para o backend.

- `EXTERNAL_STATUS_SYNC=false` desliga a leitura periódica (o cadastro e o botão "Sincronizar agora" continuam funcionando).
- Conexões a endereços privados, loopback e link-local são recusadas. `EXTERNAL_STATUS_ALLOW_PRIVATE_NETWORKS=true` libera essas redes; use apenas para páginas internas conhecidas.
- Falhas de leitura ficam em `last_error` (visível em `/admin/third-party`) e no log (`external status: sync failed`). Sem leitura bem-sucedida por `max(3 × intervalo, 10 min)`, o provedor aparece como desatualizado e seus componentes como "Desconhecido".
- Decisão e controles: [ADR 0006](adr/0006-third-party-status.md).

## Atualizar a Swagger UI embutida

```bash
scripts/update-swagger-ui.sh v5.33.3   # informe a tag desejada; revise o diff e rode os testes
```

## Deploy

Ainda não existe deploy automatizado. Antes de criar um, é preciso decidir:

1. **Destino:** VPS com Docker Compose, Kubernetes, serviço gerenciado de containers etc.
2. **HTTPS:** obrigatório fora de `127.0.0.1`, com `SESSION_COOKIE_SECURE=true`. Opcionalmente, mantenha a instância pública com `ADMIN_API_ENABLED=false` e a administração em rede privada ou VPN.
3. **Credenciais:** de preferência identidade federada (OIDC do GitHub Actions) em vez de segredos de longa duração; banco gerenciado ou com backup.
4. **Promoção:** usar sempre a tag imutável `sha-<commit>` publicada pelo `publish.yml`, com GitHub Environments e aprovação manual para produção.
5. **Rollback:** reimplantar a tag `sha-` anterior. As migrações só avançam, então mudanças de esquema devem ser compatíveis com a versão anterior.

Para executar manualmente as imagens publicadas, veja `deploy/docker-compose.ghcr.yml`.
