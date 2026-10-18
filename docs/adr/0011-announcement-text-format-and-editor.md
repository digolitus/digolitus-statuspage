# ADR 0011 — Formato do texto dos comunicados e editor visual

- **Status:** aceito (amplia o [ADR 0007](0007-announcements.md))
- **Data:** 2026-10-18

## Contexto

O editor de comunicados era um textarea com três botões (negrito, lista, link). A pedido dos operadores, ele precisa de:

- uma barra de formatação completa: negrito, itálico, sublinhado, tamanho, fonte, listas, link e outros recursos;
- um modo markdown;
- pré-visualização.

O texto continua sendo publicado numa página pública, então não pode abrir espaço para HTML arbitrário (XSS) nem para estilos que quebrem a identidade visual.

## Decisão

- **O armazenamento continua em markdown**, num dialeto pequeno e fechado (`apps/frontend/src/lib/markdown.ts`).
  - **Blocos:**
    - `## título`, `### subtítulo` e `-# texto pequeno`;
    - listas com `-` e `1.`;
    - `> citação` e `---`.
  - **Formatação inline:**
    - `**negrito**`, `*itálico*`, `++sublinhado++`, `~~tachado~~` e `==destaque==`;
    - `` `monoespaçado` ``, links http(s) e escapes com `\`.
  - Textos já publicados continuam válidos (o dialeto anterior é um subconjunto) e a API não muda.
- **Um único parser** gera uma árvore usada por todos:
  - a renderização React da página pública e da pré-visualização, que só produz elementos React e só cria links http(s);
  - o HTML do modo visual;
  - a conversão de volta do DOM do editor para markdown.
- **O editor tem três modos.**
  - **Visual:** `contentEditable` com os comandos nativos do navegador. Colar insere texto puro e arrastar conteúdo para dentro é bloqueado.
  - **Markdown:** textarea com a mesma barra e atalhos Ctrl+B/I/U/K.
  - **Pré-visualizar.**
  - O valor do formulário é sempre o markdown, e o modo escolhido fica salvo no navegador.
- **Opções de tamanho e fonte são limitadas.**
  - "Tamanho" é título, subtítulo, normal ou pequeno.
  - "Fonte" é padrão ou monoespaçada.
  - Não há cor, família de fonte nem tamanho livres. Isso mantém a página pública consistente e o formato portátil (e-mail, feeds, Slack).
- Nenhuma dependência nova: um editor rico de terceiros traria peso e superfície de ataque maiores que o necessário para este dialeto.

## Consequências

- O mesmo texto vale para página pública, feeds e notificações, sem sanitização de HTML.
- `document.execCommand` está obsoleto, mas continua suportado pelos navegadores atuais. Se algum dia for removido, só o modo visual precisa ser trocado (por exemplo, por manipulação de `Range`), porque formato, renderização e API não mudam.
- Os templates de comunicado (`/api/v1/admin/announcement-templates`) usam o mesmo formato e o mesmo editor.
