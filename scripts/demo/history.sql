-- Opcional, só para demonstração: depois de scripts/demo/seed.sh, distribui o
-- histórico de status pelos últimos 90 dias para as barras de disponibilidade
-- refletirem os incidentes resolvidos nas datas em que ocorreram.
-- Reescreve linhas append-only (ADR 0014): NUNCA rode em produção.
--
--   psql "$DATABASE_URL" -f scripts/demo/history.sql
--   docker compose -f deploy/docker-compose.yml exec -T db \
--     psql -U statuspage statuspage < scripts/demo/history.sql
BEGIN;

-- Componentes passam a existir (operacionais) desde 90 dias atrás.
UPDATE component_status_changes
SET changed_at = now() - interval '90 days'
WHERE cause_type IN ('baseline', 'component');

-- Mudanças causadas por incidentes resolvidos vão para o período real do impacto.
UPDATE component_status_changes s
SET changed_at = CASE WHEN s.status = 'operational' THEN i.resolved_at ELSE i.started_at END
FROM incidents i
WHERE s.cause_type = 'incident' AND s.cause_id = i.id AND i.resolved_at IS NOT NULL;

COMMIT;
