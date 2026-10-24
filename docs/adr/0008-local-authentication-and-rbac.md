# ADR 0008 — Autenticação local e papéis com permissões (RBAC)

- **Status:** aceito (substitui o [ADR 0004](0004-admin-without-authentication.md))
- **Data:** 2026-10-18

## Contexto

A administração não tinha autenticação, e por isso só podia rodar em rede confiável. Agora ela precisa de contas pessoais e de papéis configuráveis. Exemplos:

- uma pessoa que comunica indisponibilidades, mas não altera o cadastro nem publica comunicados;
- uma pessoa que administra componentes, grupos e comunicados, mas não gerencia usuários.

O primeiro administrador deve ser criado na primeira execução, sem credenciais padrão.

## Decisão

**Autenticação**

- Contas locais com e-mail e senha. Senhas são guardadas com Argon2id (19 MiB, t=2, p=1, sal aleatório, formato PHC) e regeradas no login se os parâmetros mudarem. Políticas:
  - 12 a 128 caracteres, diferente do e-mail;
  - bloqueio de 15 min após 5 falhas seguidas;
  - mesma resposta para e-mail inexistente, senha errada e conta inativa ou bloqueada, com hash fictício para igualar o tempo de resposta.
- **Sessões no servidor** (tabela `sessions`). O cookie `dsp_session` é `HttpOnly`, `SameSite=Strict` e `Secure` (configurável apenas para HTTP local) e leva um token aleatório de 256 bits. O banco guarda apenas o SHA-256 do token.
  - Expiração absoluta: `SESSION_TTL`, padrão 12 h.
  - Expiração por inatividade: `SESSION_IDLE_TIMEOUT`, padrão 2 h.
  - Logout, troca de senha, redefinição e desativação encerram as sessões correspondentes.
- **CSRF:** além de `SameSite=Strict` e do corpo JSON, requisições que alteram dados são recusadas quando `Sec-Fetch-Site` indica outro site ou o `Origin` não corresponde ao `Host` (403 `cross_site_request`).
- **Primeira execução:** enquanto não existe usuário, o backend gera um token de uso único e o registra no log (`initial setup required`, campo `setup_token`). Apenas o hash do token fica no banco. Em `/admin/setup`, quem tem acesso ao log cria o primeiro administrador. A operação é serializada por advisory lock e não pode ser repetida (409 `setup_already_done`). Sem acesso ao log, `api setup-token` gera um novo token.
- **Senha provisória:** quando um administrador cria um usuário ou redefine sua senha, o usuário precisa escolher a própria senha antes de usar a API administrativa (403 `password_change_required`).
- **Recuperação:** `api reset-password <e-mail>`, executado no servidor, gera uma senha provisória, desbloqueia e reativa a conta e encerra suas sessões. É auditado com a origem `cli`. Não existe recuperação pela API.

**Autorização**

- O **catálogo de permissões** é fixo no código (`internal/auth/permissions.go`), porque cada permissão corresponde a rotas que o backend verifica. Os **papéis**, guardados no banco, concedem subconjuntos desse catálogo. Um usuário pode ter vários papéis, e suas permissões são a união deles. As permissões são lidas a cada requisição, então mudanças valem imediatamente.
- O papel de sistema **Administrador** concede todas as permissões, inclusive as que forem criadas no futuro. Ele não pode ser editado nem excluído e é atribuído ao primeiro usuário. O sistema também impede remover ou desativar o último administrador ativo e desativar a própria conta.
- **Ponto único de verificação:** `auth.Guard` envolve todas as rotas `/api/v1/admin/`, e a tabela `adminPolicy` (`internal/app/policy.go`) mapeia cada rota às permissões que a liberam. Rotas fora da tabela são negadas (*fail closed*), e um teste falha se alguma rota administrativa não estiver na tabela. Leituras administrativas valem para qualquer usuário autenticado; alterações exigem permissão.
- **Exceção:** quem tem só `components.status` consegue acessar a rota `PATCH /components/{id}`, mas o handler aceita apenas a mudança de status.
- **Sem escalada de privilégio:** só é possível criar ou editar papéis com permissões que você já tem, atribuir papéis cujas permissões você tem e administrar usuários cujas permissões você cobre. Por isso, só administradores concedem o papel Administrador.
- **Auditoria:** cada alteração registra o autor (`actor_id`, `actor_name`) além da origem.

## Alternativas consideradas

- **OIDC/SSO (Google, Entra ID, Keycloak):** é a escolha natural para organizações com provedor de identidade, mas depende de configuração externa e não atende uma instalação isolada. A interface `Principal` e o `Guard` permitem acrescentá-lo depois, mantendo os papéis locais. Google, Apple e Facebook foram acrescentados no [ADR 0012](0012-social-sign-in.md).
- **JWT sem estado:** não permite revogação imediata ao desativar usuários ou mudar papéis, e exigiria guardar o token em lugar acessível a scripts ou criar outra camada de controle. Sessões no servidor são mais simples e seguras para este volume.
- **Papéis fixos sem permissões granulares:** não atenderiam combinações como "comunica incidentes mas não comunicados".
- **Usuário e senha padrão (admin/admin):** é uma credencial conhecida em toda instalação nova. O token de uso único no log exige acesso ao servidor.

## Consequências

- Atrás de HTTPS, use `SESSION_COOKIE_SECURE=true`. O Compose local usa `false` porque serve HTTP em `127.0.0.1`.
- Ainda não existem 2FA, recuperação de senha por e-mail nem lista de sessões ativas. São extensões naturais, sem mudar o modelo.
- O primeiro acesso exige ler o log do backend, o que é uma etapa operacional, documentada em `docs/operations.md`.
