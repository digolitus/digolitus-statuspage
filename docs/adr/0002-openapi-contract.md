# ADR 0002 — Especificação OpenAPI versionada como contrato, validada por testes

- **Status:** aceito
- **Data:** 2026-10-18

## Contexto

A API é o contrato central do produto: o frontend e futuras integrações dependem dela. A documentação precisa refletir a implementação, e a divergência entre as duas deve ser detectada automaticamente.

## Decisão

- O contrato é `apps/backend/api/openapi.yaml` (OpenAPI 3.0.3), escrito à mão e revisado junto com as mudanças de código.
- O documento é embutido no binário e servido em `/api/openapi.yaml`.
- Testes garantem a correspondência, usando `kin-openapi` apenas em teste:
  1. a especificação é válida;
  2. o conjunto `método + caminho` registrado no roteador é idêntico ao documentado, nos dois sentidos;
  3. os testes de integração validam cada requisição e resposta reais contra os schemas. As respostas usam `additionalProperties: false`, então campos não documentados falham.
- A **Swagger UI** usa os arquivos oficiais `dist/` do projeto `swagger-api/swagger-ui`, vendorizados no backend (`internal/docs/swaggerui`, Apache-2.0) e servidos em `/swagger/` com CSP sem scripts inline. Não depende de CDN nem de npm. A atualização é feita com `scripts/update-swagger-ui.sh`.

## Alternativas consideradas

- **Gerar a especificação a partir do código (anotações/swag):** acopla a documentação a comentários e tende a produzir schemas pobres, sem `oneOf`, `nullable` ou exemplos.
- **Gerar o servidor a partir da especificação (oapi-codegen):** forte garantia de tipos, mas impõe estrutura de handlers e acrescenta geração de código ao fluxo. Pode ser reavaliado se a API crescer muito.
- **Swagger UI via CDN:** dependência externa em tempo de execução e CSP mais frouxa.
- **Swagger UI via pacote npm no frontend:** a disponibiliza só com o frontend; a versão no backend funciona também em desenvolvimento sem Docker.

## Consequências

- Mudanças de API exigem editar a especificação, senão os testes falham.
- Os tipos TypeScript do frontend são gerados a partir da especificação (ver [ADR 0005](0005-typescript-types-from-openapi.md)).
- Assets da Swagger UI (~1,8 MB) passam a fazer parte do repositório e do binário.
