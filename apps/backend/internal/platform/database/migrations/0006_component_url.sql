-- Optional link to the component's own page (documentation, product page,
-- public endpoint). Shown on the public page as a discreet external link.
ALTER TABLE components
    ADD COLUMN url text CONSTRAINT components_url_check
        CHECK (url IS NULL OR (char_length(url) <= 500 AND url ~ '^https?://'));
