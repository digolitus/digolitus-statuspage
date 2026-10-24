-- Social sign-in (Google, Apple, Facebook) for existing users.
--
-- A provider account is linked to a user the first time it signs in with a
-- verified e-mail equal to the user's; later sign-ins are matched by the
-- provider's stable subject, even if the e-mail changes at the provider.
-- Changing the user's e-mail in the admin area removes the links.
CREATE TABLE user_identities (
    provider     text        NOT NULL CHECK (provider IN ('google', 'apple', 'facebook')),
    subject      text        NOT NULL CHECK (char_length(subject) BETWEEN 1 AND 255),
    user_id      uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- E-mail reported by the provider when the link was made (for audit).
    email        text        NOT NULL DEFAULT '' CHECK (char_length(email) <= 254),
    created_at   timestamptz NOT NULL,
    last_used_at timestamptz NOT NULL,
    PRIMARY KEY (provider, subject)
);

CREATE INDEX user_identities_user_idx ON user_identities (user_id);
