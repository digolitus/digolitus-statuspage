# Digolitus Status Page

Aplicação inicial do **Digolitus Status Page**, composta por um frontend React/TypeScript servido pelo Nginx e uma API independente desenvolvida em Go.

A primeira versão implementa uma página inicial funcional, consulta real ao backend e endpoints básicos de saúde. Não há banco de dados, autenticação, cadastro de serviços, métricas ou histórico de incidentes nesta versão.

## Arquitetura

```text
Navegador
   |
   v
Frontend/Nginx :8080
   |-- /                 -> arquivos estáticos React
   |-- /api/hello        -> backend:8080/api/hello
   |-- /healthz          -> backend:8080/healthz
   `-- /readyz           -> backend:8080/readyz
```

Serviços:

- `backend`: API HTTP em Go;
- `frontend`: aplicação React compilada pelo Vite e servida pelo Nginx;
- `default`: rede interna do Docker Compose.

## Estrutura

```text
.
├── apps
│   ├── backend
│   │   ├── cmd
│   │   │   ├── api
│   │   │   └── healthcheck
│   │   ├── internal
│   │   │   ├── config
│   │   │   └── handlers
│   │   ├── Dockerfile
│   │   ├── go.mod
│   │   └── ...
│   └── frontend
│       ├── public
│       ├── src
│       ├── Dockerfile
│       ├── nginx.conf
│       ├── package.json
│       └── ...
├── deploy
│   └── docker-compose.yml
├── .github
│   └── workflows
├── Makefile
└── README.md
```

## Pré-requisitos

Para executar localmente:

- Docker 24 ou superior;
- Docker Compose plugin;
- Go 1.23 ou superior, para executar validações sem Docker;
- Node.js 22 ou superior e npm 10 ou superior, para executar validações do frontend sem Docker;
- Trivy, opcionalmente, para análise de segurança local.

## Execução com Docker Compose

```bash
git clone https://github.com/digolitus/digolitus-statuspage.git
cd digolitus-statuspage
docker compose -f deploy/docker-compose.yml up --build
```

Acesse:

```text
http://localhost:8080
```

O backend não publica uma porta diretamente no host. O acesso ocorre pelo proxy reverso do Nginx.

## Endpoints

```bash
curl -i http://localhost:8080/api/hello
curl -i http://localhost:8080/healthz
curl -i http://localhost:8080/readyz
```

Resposta esperada de `/api/hello`:

```json
{
  "message": "Hello World from Digolitus Status Page",
  "service": "digolitus-statuspage-backend"
}
```

`/healthz` verifica se o processo está vivo e apto a responder.

`/readyz` verifica se a aplicação está pronta para receber requisições. Na versão inicial, como não existem dependências externas, ambos retornam HTTP 200 quando o servidor está operacional.

## Comandos Docker Compose

```bash
docker compose -f deploy/docker-compose.yml ps
docker compose -f deploy/docker-compose.yml logs -f
docker compose -f deploy/docker-compose.yml down
```

## Desenvolvimento sem Docker

### Backend

```bash
cd apps/backend
go test ./...
go vet ./...
go run ./cmd/api
```

O backend utiliza a porta `8080` por padrão. Para alterar:

```bash
PORT=9090 go run ./cmd/api
```

### Frontend

```bash
cd apps/frontend
npm install
npm run dev
```

Durante o desenvolvimento pelo Vite, o proxy configurado em `vite.config.ts` encaminha as chamadas para `http://localhost:8080`.

Scripts disponíveis:

```bash
npm ci
npm run lint
npm test -- --run
npm run build
```

## Makefile

```bash
make up
make down
make logs
make ps
make test
make lint
make build
make scan
make clean
```

O alvo `make scan` exige que o Trivy esteja instalado localmente.

## Construção manual das imagens

Backend:

```bash
docker build \
  -t digolitus-statuspage-backend:local \
  apps/backend
```

Frontend:

```bash
docker build \
  -t digolitus-statuspage-frontend:local \
  apps/frontend
```

## Segurança

O projeto contém verificações para:

- `go vet`;
- `gosec`;
- ESLint;
- Trivy no filesystem;
- Trivy em configurações;
- Trivy nas imagens Docker;
- detecção de segredos;
- vulnerabilidades HIGH e CRITICAL.

A política inicial bloqueia o pipeline quando vulnerabilidades HIGH ou CRITICAL são detectadas com correção disponível.

Exemplo:

```bash
trivy fs \
  --scanners vuln,secret,misconfig \
  --severity HIGH,CRITICAL \
  .
```

## GitHub Actions

Os workflows estão em `.github/workflows`:

- `ci.yml`: testes, lint, build, validação dos Dockerfiles e Trivy;
- `build-publish.yml`: build e publicação das imagens no GHCR;
- `deploy.yml`: workflow preparado para receber uma imagem previamente publicada, sem presumir infraestrutura de deploy.

O workflow de publicação não executa em Pull Requests e utiliza `GITHUB_TOKEN` somente no contexto de publicação.

## Imagens GHCR

Nomes previstos:

```text
ghcr.io/digolitus/digolitus-statuspage-backend
ghcr.io/digolitus/digolitus-statuspage-frontend
```

As imagens são identificadas por:

- SHA do commit;
- branch;
- tag de release;
- versão semântica para releases.

A tag `latest` não é utilizada como único mecanismo de versionamento.

Como o repositório é privado, o download exige autenticação no GHCR e permissão de leitura no pacote:

```bash
echo "$GITHUB_TOKEN" | docker login ghcr.io -u SEU_USUARIO --password-stdin
docker pull ghcr.io/digolitus/digolitus-statuspage-backend:<tag>
```

Não coloque tokens no repositório, no `.env`, na documentação ou na linha de comando compartilhada.

## Releases

Para publicar uma versão:

```bash
git tag v0.1.0
git push origin v0.1.0
```

O workflow de publicação utilizará a tag para gerar imagens versionadas.

## Adicionando novos módulos

Para adicionar um novo serviço:

1. Crie um diretório em `apps/<novo-modulo>`;
2. Adicione seu Dockerfile;
3. Adicione testes e lint;
4. Inclua o módulo nos workflows;
5. Inclua o módulo no job de publicação;
6. Atualize o Compose caso ele faça parte da execução local;
7. Atualize este README.

A estrutura atual evita compartilhar código de build entre aplicações diferentes, mantendo os ciclos de build e publicação independentes.

## Deploy

O destino de produção ainda não foi definido.

O workflow `deploy.yml` não declara sucesso sem executar uma implantação real. Ele valida a presença de um digest ou tag de imagem e encerra explicitamente informando que o adaptador de infraestrutura ainda precisa ser configurado.

Antes de usar em produção, será necessário definir:

- destino de deploy;
- ambiente GitHub;
- regras de aprovação;
- credenciais ou identidade federada;
- estratégia de rollback;
- monitoramento do deploy;
- política de promoção entre ambientes.

## Solução de problemas

### O frontend não inicia

Verifique:

```bash
docker compose -f deploy/docker-compose.yml logs frontend
docker compose -f deploy/docker-compose.yml ps
```

### O backend não fica saudável

Verifique:

```bash
docker compose -f deploy/docker-compose.yml logs backend
```

O backend deve escutar na porta interna `8080`.

### O `npm ci` falha

Remova dependências antigas e recrie o lockfile:

```bash
cd apps/frontend
rm -rf node_modules package-lock.json
npm install
npm ci
```

### O Trivy não está instalado

Instale seguindo a documentação oficial:

```text
https://aquasecurity.github.io/trivy/
```

## Estado da versão

Implementado:

- API Go;
- frontend React/TypeScript;
- proxy Nginx;
- Docker Compose;
- testes;
- workflows de CI;
- publicação preparada para GHCR;
- deploy desacoplado da infraestrutura.

Ainda não implementado:

- banco de dados;
- autenticação;
- cadastro de serviços;
- coleta de métricas;
- histórico de incidentes;
- deploy em ambiente remoto;
- monitoramento externo.
