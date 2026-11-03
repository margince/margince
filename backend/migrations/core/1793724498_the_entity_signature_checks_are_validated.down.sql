-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Put the checks back the way the file beside this one found them: present,
-- unvalidated. Postgres has no ALTER that un-validates, so each is dropped and
-- re-added NOT VALID, which takes ACCESS EXCLUSIVE without a scan.
SET LOCAL lock_timeout = '3s';

ALTER TABLE legal_entity
    DROP CONSTRAINT legal_entity_default_signs_with_the_installation,
    DROP CONSTRAINT legal_entity_signature_template_bounded,
    ADD CONSTRAINT legal_entity_default_signs_with_the_installation
        CHECK (NOT is_default OR signature_template IS NULL) NOT VALID,
    ADD CONSTRAINT legal_entity_signature_template_bounded
        CHECK (signature_template IS NULL OR length(signature_template) <= 4000) NOT VALID;
