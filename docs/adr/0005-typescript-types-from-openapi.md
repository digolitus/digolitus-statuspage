# ADR 0005 — Tipos TypeScript gerados a partir do OpenAPI por um gerador em Go

- **Status:** aceito
- **Data:** 2026-10-18

## Contexto

Os tipos da API no frontend eram escritos à mão e podiam divergir do contrato (`apps/backend/api/openapi.yaml`). Na introdução dos grupos de componentes, por exemplo, a geração revelou que o rótulo do novo tipo de recurso de auditoria faltava na interface.

A ferramenta usual, `openapi-typescript`, é um pacote npm. No ambiente em que esta mudança foi feita, o registry npm estava inacessível, e a dependência teria que ser instalada e auditada no frontend.

## Decisão

- Um gerador pequeno em Go (`apps/backend/internal/tsgen`, comando `cmd/openapi-ts`) lê o contrato com `kin-openapi`, que já é dependência do backend, e escreve `apps/frontend/src/api/schema.gen.ts`.
- `apps/frontend/src/api/types.ts` apenas dá nomes amigáveis aos tipos gerados. Formatos da API não são declarados à mão.
- O teste `TestGeneratedTypesAreUpToDate` falha quando o arquivo gerado não corresponde ao contrato. O CI executa esse teste sempre que o backend ou o arquivo gerado mudam.
- Regeneração: `make generate-types`.

## Alternativas consideradas

- **`openapi-typescript` no frontend:** padrão de mercado e mais completo (gera também operações e caminhos). Exigiria npm para a geração e um passo de verificação no job de frontend. É a escolha natural se o contrato passar a usar recursos que o gerador próprio não cobre.
- **Gerar o cliente inteiro (orval, openapi-fetch):** mais acoplamento e dependências do que o cliente atual, que tem cerca de 100 linhas, justifica.

## Consequências

- O gerador cobre o subconjunto do OpenAPI 3.0 usado pelo contrato (`$ref`, objetos, arrays, enums, `nullable`, `allOf`/`oneOf`/`anyOf`) e falha explicitamente diante de construções não suportadas.
- É código próprio a manter, cerca de 200 linhas com testes.
