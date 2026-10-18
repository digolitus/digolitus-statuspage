# ADR 0004 — Administração sem autenticação nesta versão

- **Status:** substituído pelo [ADR 0008](0008-local-authentication-and-rbac.md)
- **Data:** 2026-10-18

## Contexto

O escopo desta etapa exclui login, usuários e sessões. Ainda assim, a API administrativa altera todos os dados publicados. O requisito explícito é não simular autenticação, não criar credenciais padrão e manter a administração restrita ao ambiente local.

## Decisão

- As rotas de escrita ficam todas sob `/api/v1/admin/`, e a interface sob `/admin`.
- No backend, todas as rotas administrativas passam por um único wrapper (`internal/app/app.go`). É o ponto onde a autenticação e a autorização serão aplicadas.
- O histórico operacional registra a origem `admin_api` e o `X-Request-ID`, sem inventar identidade de usuário.
- O Compose publica apenas o Nginx, e somente em `127.0.0.1`. Backend e banco ficam numa rede interna, sem portas no host.
- O Nginx aplica limitação de taxa mais restrita às rotas admin, limite de corpo e timeouts. Isso é mitigação de abuso, não controle de acesso.
- `ADMIN_API_ENABLED=false` (padrão `true` para uso local) remove as rotas administrativas, permitindo uma instância pública somente leitura.
- A interface e a Swagger UI exibem permanentemente o aviso de ausência de autenticação.

## Alternativas rejeitadas

- **Segredo fixo no frontend ou token estático:** não é autenticação, vaza no bundle e dá falsa sensação de segurança.
- **Basic auth no Nginx com credencial padrão:** credencial padrão versionada e sem identidade útil para auditoria.

## Consequências e riscos

- Qualquer cliente na mesma máquina, ou em rede que alcance a porta publicada, pode alterar dados. Não há como atribuir alterações a pessoas.
- **Bloqueante para qualquer exposição pública com administração habilitada.**

## Próximo passo recomendado

Introduzir autenticação, preferencialmente OIDC por meio de um proxy de identidade ou middleware no wrapper admin, e autorização por papel (leitura vs. operação). O principal autenticado deve ser gravado como origem nos eventos de auditoria. Os módulos de domínio não precisam mudar: recebem a origem pelo contexto.
