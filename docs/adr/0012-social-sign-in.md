# ADR 0012 — Login social (Google, Apple, Facebook) para usuários cadastrados

- **Status:** aceito (complementa o [ADR 0008](0008-local-authentication-and-rbac.md))
- **Data:** 2026-10-23

## Contexto

Operadores querem entrar na administração com uma conta que já usam (Google, Apple ou Facebook) em vez de guardar mais uma senha. O modelo de papéis e permissões do ADR 0008 continua valendo, e uma instalação isolada, sem acesso a esses provedores, precisa continuar funcionando.

## Decisão

- **Só para quem já tem usuário.** O login social não cria contas. Um administrador cadastra o usuário, e o provedor só prova que a pessoa é dona do e-mail. Assim, uma conta Google qualquer nunca entra sem convite, e os papéis continuam sendo atribuídos por quem pode.
- **Vínculo no primeiro acesso.** A identidade do provedor é vinculada ao usuário (tabela `user_identities`) quando o provedor confirma um e-mail igual ao do usuário: `email_verified` no Google e na Apple. O Facebook só devolve e-mails confirmados. Nos acessos seguintes, o usuário é reconhecido pelo ID estável do provedor (`sub`), mesmo que o e-mail mude lá.
- **Trocar o e-mail do usuário desfaz os vínculos**, porque o novo endereço pode ser de outra pessoa. Usuários inativos são recusados. O bloqueio por senhas erradas não se aplica, porque não há senha envolvida.
- **O login com senha continua.** Ele é necessário para o primeiro administrador, para a recuperação pela CLI e para instalações sem os provedores. Cada provedor só aparece na tela de login quando está configurado (`OAUTH_*`).
- **Fluxo authorization code**, com o backend como cliente confidencial. `GET /api/v1/auth/oauth/{provider}` grava o cookie `dsp_oauth` (10 min, uso único) com `state`, `nonce` e verificador PKCE, e redireciona ao provedor. O retorno em `/api/v1/auth/oauth/{provider}/callback` (GET, ou POST de formulário na Apple) confere o `state`, troca o código, cria a sessão normal (`dsp_session`, ADR 0008) e redireciona para `/admin`. Falhas voltam para `/admin?login_error=cancelled|not_allowed|failed`, e o motivo detalhado fica só no log.
- **Cookie do fluxo.** A Apple devolve o resultado por um POST vindo do site dela, que não leva cookies `SameSite=Lax`. Por isso, com HTTPS, `dsp_oauth` é `SameSite=None; Secure`. Ele é seguro porque é de uso único, vale 10 minutos e só é comparado com o `state`. Sem HTTPS, ele é `Lax`, o que basta para Google e Facebook. A Apple exige HTTPS de qualquer forma.
- **Sem dependências novas.** O `id_token` do Google e da Apple chega direto do endpoint de token, por TLS, então a assinatura não é verificada (OpenID Connect Core, seção 3.1.3.7, item 6). Emissor, audiência, expiração e `nonce` são verificados. O *client secret* da Apple é um JWT ES256 assinado a cada troca com a chave `.p8`. O Facebook usa a Graph API com `appsecret_proof`.
- O `redirect_uri` é `PUBLIC_BASE_URL` + `/api/v1/auth/oauth/{provider}/callback`, e o backend registra no log, na inicialização, a URL de cada provedor habilitado.

## Alternativas consideradas

- **Criar contas automaticamente**, sem papéis ou por domínio permitido: é prático em organizações grandes, mas abre a administração para qualquer conta do domínio, ou de qualquer domínio se a configuração estiver errada. Pode ser acrescentado depois como opção explícita.
- **Biblioteca OIDC (go-oidc, x/oauth2)**, com verificação de assinatura por JWKS: é necessária quando o token não vem direto do provedor, o que não é o caso aqui. Acrescentaria dependências e busca de chaves sem ganho de segurança neste fluxo.
- **Desativar o login por senha**: deixaria instalações sem acesso aos provedores, e a recuperação, sem saída.

## Consequências

- Na Apple, o usuário que escolher "Ocultar meu e-mail" recebe um endereço `privaterelay.appleid.com`, que não corresponde ao cadastro. Ele deve compartilhar o e-mail real, ou o administrador deve cadastrar esse endereço.
- Usuários criados com senha provisória ainda precisam trocá-la antes de usar a API administrativa (`password_change_required`), mesmo quando entram pelo provedor.
- A versão da Graph API do Facebook (`facebookGraphVersion`) precisa ser atualizada antes de a Meta desativá-la, cerca de dois anos após o lançamento.
- Ainda não existe tela para ver ou remover vínculos. Trocar o e-mail do usuário ou desativá-lo são as formas de cortar o acesso social.
