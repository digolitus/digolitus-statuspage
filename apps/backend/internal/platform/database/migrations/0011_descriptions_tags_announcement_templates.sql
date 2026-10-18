-- Component descriptions shown on demand, team tags and announcement
-- templates.

-- Descriptions become opt-in on the public page (an "i" next to the name).
-- Existing descriptions were already public, so they stay visible.
ALTER TABLE components ADD COLUMN show_description boolean NOT NULL DEFAULT false;
UPDATE components SET show_description = true WHERE description <> '';

-- Free-form labels for teams (e.g. "pagamentos", "on-call 24x7"); stored
-- normalized (trimmed, unique, at most 20) by the application.
ALTER TABLE teams ADD COLUMN tags text[] NOT NULL DEFAULT '{}'
    CONSTRAINT teams_tags_limit CHECK (cardinality(tags) <= 20);

-- Announcement templates pre-fill the announcement editor; announcements
-- copy the text, so editing a template never changes published history.
CREATE TABLE announcement_templates (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    name        text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    description text        NOT NULL DEFAULT '' CHECK (char_length(description) <= 1000),
    title       text        NOT NULL CHECK (char_length(title) BETWEEN 1 AND 200),
    body        text        NOT NULL CHECK (char_length(body) BETWEEN 1 AND 10000),
    importance  text        NOT NULL DEFAULT 'info' CHECK (importance IN ('info', 'important')),
    -- SET NULL: deleting a category never breaks a template.
    category_id uuid        CONSTRAINT announcement_templates_category_id_fkey REFERENCES announcement_categories (id) ON DELETE SET NULL,
    guidance    text        NOT NULL DEFAULT '' CHECK (char_length(guidance) <= 2000),
    is_active   boolean     NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX announcement_templates_name_key ON announcement_templates (lower(name));

INSERT INTO announcement_templates (name, description, title, body, importance, guidance) VALUES
('Nova funcionalidade', 'Lançamento de recurso ou melhoria.',
 'Novo: [nome da funcionalidade]',
 E'Já está disponível **[nome da funcionalidade]**.\n\n## O que muda\n- [benefício principal]\n- [outro benefício]\n\n## Como usar\n[instruções curtas ou link para a documentação]',
 'info', 'Comece pelo benefício para quem usa. Inclua o link da documentação.'),
('Mudança com ação necessária', 'Algo deixa de funcionar ou muda e exige ação de quem usa.',
 'Ação necessária até [data]: [mudança]',
 E'A partir de **[data]**, [o que muda].\n\n## O que você precisa fazer\n1. [passo]\n2. [passo]\n\n## Por que\n[motivo]\n\nDúvidas: [canal de suporte]',
 'important', 'Diga a data e a ação logo no início. Marque como importante.'),
('Descontinuação', 'Fim de suporte de uma versão, API ou recurso.',
 'Descontinuação de [recurso] em [data]',
 E'O recurso **[recurso]** será descontinuado em **[data]**.\n\n## Alternativa\n[o que usar no lugar]\n\n## Prazo\n- [data]: [marco]\n- [data]: desligamento',
 'important', 'Informe a alternativa e o calendário completo.');

ALTER TABLE audit_events DROP CONSTRAINT audit_events_resource_type_check;
ALTER TABLE audit_events ADD CONSTRAINT audit_events_resource_type_check
    CHECK (resource_type IN ('component', 'component_group', 'incident', 'maintenance', 'incident_template',
                             'maintenance_template', 'external_source', 'announcement', 'announcement_category', 'team',
                             'user', 'role', 'subscription', 'announcement_template'));
