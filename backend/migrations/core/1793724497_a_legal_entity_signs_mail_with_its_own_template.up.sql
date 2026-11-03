-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A further legal entity can carry its own mail signature template. A sender
-- assigned to it signs with that template; NULL falls back to the
-- installation's template, as before.
--
-- The default entity signs with the installation's template, which keeps its
-- own editor, so it stores none here. The checks are added NOT VALID and
-- validated by the file beside this one.
SET LOCAL lock_timeout = '3s';

ALTER TABLE legal_entity
    ADD COLUMN signature_template text,
    ADD CONSTRAINT legal_entity_default_signs_with_the_installation
        CHECK (NOT is_default OR signature_template IS NULL) NOT VALID,
    ADD CONSTRAINT legal_entity_signature_template_bounded
        CHECK (signature_template IS NULL OR length(signature_template) <= 4000) NOT VALID;
