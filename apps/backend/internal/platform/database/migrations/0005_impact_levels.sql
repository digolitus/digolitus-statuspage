-- Incident impact and maintenance expected impact now share the component
-- status vocabulary: operational, degraded_performance, partial_outage,
-- major_outage. "operational" is an incident that does not degrade the
-- affected components (informational).
--
-- Mapping of existing data: minor → degraded_performance,
-- major → partial_outage, critical → major_outage, none → operational.
-- Audit events keep the values recorded at the time.

ALTER TABLE incidents DROP CONSTRAINT incidents_impact_check;
UPDATE incidents SET impact = CASE impact
    WHEN 'minor' THEN 'degraded_performance'
    WHEN 'major' THEN 'partial_outage'
    WHEN 'critical' THEN 'major_outage'
    ELSE impact END;
ALTER TABLE incidents ADD CONSTRAINT incidents_impact_check
    CHECK (impact IN ('operational', 'degraded_performance', 'partial_outage', 'major_outage'));

ALTER TABLE incident_templates DROP CONSTRAINT incident_templates_impact_check;
UPDATE incident_templates SET impact = CASE impact
    WHEN 'minor' THEN 'degraded_performance'
    WHEN 'major' THEN 'partial_outage'
    WHEN 'critical' THEN 'major_outage'
    ELSE impact END;
ALTER TABLE incident_templates ADD CONSTRAINT incident_templates_impact_check
    CHECK (impact IN ('operational', 'degraded_performance', 'partial_outage', 'major_outage'));

ALTER TABLE maintenances DROP CONSTRAINT maintenances_expected_impact_check;
UPDATE maintenances SET expected_impact = 'operational' WHERE expected_impact = 'none';
ALTER TABLE maintenances ADD CONSTRAINT maintenances_expected_impact_check
    CHECK (expected_impact IN ('operational', 'degraded_performance', 'partial_outage', 'major_outage'));

ALTER TABLE maintenance_templates DROP CONSTRAINT maintenance_templates_expected_impact_check;
UPDATE maintenance_templates SET expected_impact = 'operational' WHERE expected_impact = 'none';
ALTER TABLE maintenance_templates ADD CONSTRAINT maintenance_templates_expected_impact_check
    CHECK (expected_impact IN ('operational', 'degraded_performance', 'partial_outage', 'major_outage'));

-- Starter incident templates follow the new vocabulary, but only while they
-- are still exactly as seeded (operators may have edited them) and the new
-- name is free.
UPDATE incident_templates t SET name = v.new_name, title = v.new_title
FROM (VALUES
    ('Degradação de desempenho', 'Degradação de desempenho em [componente]', 'Performance degradada', 'Performance degradada em [componente]'),
    ('Indisponibilidade parcial', 'Indisponibilidade parcial em [componente]', 'Interrupção parcial', 'Interrupção parcial em [componente]'),
    ('Indisponibilidade total', 'Indisponibilidade de [componente]', 'Interrupção grave', 'Interrupção grave em [componente]')
) AS v (old_name, old_title, new_name, new_title)
WHERE t.name = v.old_name AND t.title = v.old_title
  AND NOT EXISTS (SELECT 1 FROM incident_templates o WHERE lower(o.name) = lower(v.new_name));
