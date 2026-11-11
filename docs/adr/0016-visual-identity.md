# ADR 0016 — Identidade visual configurável

- **Status:** aceito
- **Data:** 2026-11-11

## Contexto

O nome "Digolitus Status Page", a marca "D", as cores e o rodapé estavam fixos no frontend. O pedido é configurar pela administração o título, o subtítulo do cabeçalho, os textos da tela de login e do rodapé, o logotipo, o favicon, a fonte e a paleta de cores da página pública.

A página é servida com uma CSP restrita (`img-src 'self' data:`, `font-src 'self'`, `style-src 'self'`). Imagens e fontes de outros domínios não carregam, e estilos inline não são permitidos.

## Decisão

- **Um documento validado.** `branding_settings` guarda uma única linha com o documento em `jsonb`; o módulo `branding` valida tudo (tamanhos, caracteres de controle, cores `#rrggbb`, links `http(s)`). Sem linha salva valem os padrões, com `SITE_NAME` como título. Campos novos no futuro entram com valor padrão sem migração.
- **Imagens por upload ou URL liberada.** Cada imagem (logotipo, favicon) tem origem `none`, `upload` ou `url`.
  - Upload: guardado em `branding_images` (bytea) e servido por `GET /api/v1/branding/images/{kind}` com ETag, cache imutável quando a URL traz a versão (`?v=<hash>`), `nosniff` e `CSP: default-src 'none'; sandbox`. O tipo é detectado pelo conteúdo; SVG não é aceito (pode conter script). Logotipo: PNG, JPG ou WebP até 512 KB; favicon: PNG ou ICO até 128 KB.
  - URL: só `https://` em host listado em `BRANDING_IMAGE_HOSTS`. A mesma variável alimenta a CSP do Nginx (`img-src`), gerada na subida do container. A lista fica na configuração do servidor e não no admin: ela amplia a política de segurança, e uma conta administrativa comprometida não deve conseguir fazer isso.
- **Fontes empacotadas.** Lista fechada de seis fontes livres (`@fontsource-variable`) mais a fonte do sistema. O navegador só baixa a fonte em uso. Fonte por upload ou de serviço externo fica fora.
- **Quatro cores de marca, status fixos.** Cor principal, fundo e texto do cabeçalho, fundo da página. O backend recusa contraste abaixo de 4,5:1 (WCAG AA) para botões, cabeçalho e texto secundário; o admin mostra o mesmo cálculo antes de salvar. As cores de status não são configuráveis porque têm significado fixo.
- **Escopo.** Vale para a página pública e a tela de login. O admin mantém o visual padrão. Título e favicon também valem para a aba do navegador. E-mails e feeds usam o título configurado (cache de 30 s no processo).
- **Aplicação no frontend.** `GET /api/v1/branding` é lido uma vez; o último valor fica em `localStorage` para evitar o "piscar" do visual padrão. As cores viram variáveis CSS escritas via CSSOM, permitido pela CSP.
- **Acesso e auditoria.** Leitura no admin para qualquer usuário; alteração exige `branding.manage`. Cada alteração, envio ou remoção de imagem gera evento `branding` na auditoria.

## Alternativas consideradas

- **Só URL externa:** dependeria de outro site no ar e exigiria liberar domínios arbitrários na CSP.
- **Lista de domínios editável no admin:** exigiria gerar a CSP dinamicamente pelo backend e daria a um administrador o poder de afrouxar a política de segurança.
- **Google Fonts:** exige liberar domínios externos e expõe visitantes a terceiros.
- **CSS livre:** abre brecha de segurança e quebra a cada atualização da interface.

## Consequências

- Mudar `BRANDING_IMAGE_HOSTS` exige reiniciar backend e frontend com a mesma lista; um host removido faz a URL ser ignorada (a página volta à inicial/padrão).
- Modo escuro da página pública e fonte personalizada ficam para uma próxima etapa.
