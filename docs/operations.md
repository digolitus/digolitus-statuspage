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

## Administração sem autenticação

**Risco:** nesta versão, qualquer cliente que alcance `/admin` ou `/api/v1/admin/` pode criar, alterar e encerrar incidentes, manutenções, componentes e templates. O histórico registra a origem `admin_api`, mas não identifica pessoas.

Controles existentes:

- O Compose publica apenas o frontend, e somente em `127.0.0.1`. Backend e PostgreSQL ficam numa rede interna sem portas no host e sem acesso à internet.
- Limitação de taxa no Nginx (5 req/s por IP com rajada de 20 nas rotas admin; 20 req/s no restante da API), limite de corpo de 64 KiB e timeouts.
- `ADMIN_API_ENABLED=false` remove todas as rotas administrativas do backend, deixando uma API pública somente leitura.

O que **não** é controle de acesso: a Swagger UI, CORS, a URL `/admin` "não divulgada" e a limitação de taxa.

**Não publique esta stack em rede não confiável** (não troque `127.0.0.1` por `0.0.0.0`, não coloque atrás de um proxy público) sem antes implementar autenticação e autorização. Para um ambiente remoto somente leitura, use `ADMIN_API_ENABLED=false`. Veja o [ADR 0004](adr/0004-admin-without-authentication.md).

## Serviços de terceiros

O backend lê `https://<provedor>/api/v2/summary.json` a cada `EXTERNAL_STATUS_INTERVAL` (padrão `2m`, entre `30s` e `1h`). Para isso ele usa a rede `egress` do Compose; o PostgreSQL continua isolado. Em outra infraestrutura, libere saída HTTPS (443) apenas para o backend.

- `EXTERNAL_STATUS_SYNC=false` desliga a leitura periódica (o cadastro e o botão "Sincronizar agora" continuam funcionando).
- Conexões a endereços privados, loopback e link-local são recusadas. `EXTERNAL_STATUS_ALLOW_PRIVATE_NETWORKS=true` libera essas redes; use apenas para páginas internas conhecidas, pois a administração não tem autenticação.
- Falhas de leitura ficam em `last_error` (visível em `/admin/third-party`) e no log (`external status: sync failed`). Sem leitura bem-sucedida por `max(3 × intervalo, 10 min)`, o provedor aparece como desatualizado e seus componentes como "Desconhecido".
- Decisão e controles: [ADR 0006](adr/0006-third-party-status.md).

## Atualizar a Swagger UI embutida

```bash
scripts/update-swagger-ui.sh v5.33.3   # informe a tag desejada; revise o diff e rode os testes
```

## Deploy

Ainda não existe deploy automatizado. Antes de criar um, é preciso decidir:

1. **Destino:** VPS com Docker Compose, Kubernetes, serviço gerenciado de containers etc.
2. **Proteção da administração:** autenticação ([ADR 0004](adr/0004-admin-without-authentication.md)) ou instância pública com `ADMIN_API_ENABLED=false` e a administração só em rede privada/VPN.
3. **Credenciais:** de preferência identidade federada (OIDC do GitHub Actions) em vez de segredos de longa duração; banco gerenciado ou com backup.
4. **Promoção:** usar sempre a tag imutável `sha-<commit>` publicada pelo `publish.yml`, com GitHub Environments e aprovação manual para produção.
5. **Rollback:** reimplantar a tag `sha-` anterior. As migrações só avançam, então mudanças de esquema devem ser compatíveis com a versão anterior.

Para executar manualmente as imagens publicadas, veja `deploy/docker-compose.ghcr.yml`.
