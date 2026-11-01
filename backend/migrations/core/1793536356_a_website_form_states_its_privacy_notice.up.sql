-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The privacy notice a form shows under it at the point of collection, and the
-- policy it links to. Both are optional; a form saved before this has neither.
SET LOCAL lock_timeout = '3s';

ALTER TABLE web_form
    ADD COLUMN privacy_notice text,
    ADD COLUMN privacy_notice_url text,
    ADD CONSTRAINT web_form_privacy_notice_bounded
        CHECK (privacy_notice IS NULL OR length(privacy_notice) <= 500) NOT VALID,
    ADD CONSTRAINT web_form_privacy_notice_url_https
        CHECK (privacy_notice_url IS NULL OR (privacy_notice_url LIKE 'https://%' AND length(privacy_notice_url) <= 2000)) NOT VALID;
