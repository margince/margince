-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The record seam serves products and offer templates, so a passport keeps the
-- catalog on MCP as it does on REST. The three CHECKs bound to the seam's
-- vocabulary widen to carry both. Recorded NOT VALID here, without a scan;
-- 1792059406 validates them and retires the narrower ones.
SET LOCAL lock_timeout = '3s';

ALTER TABLE attachment ADD CONSTRAINT attachment_entity_type_check_v2
    CHECK (entity_type IN ('contact', 'company', 'deal', 'lead', 'activity', 'project', 'relationship', 'partner', 'product', 'offer_template')) NOT VALID;
ALTER TABLE embedding ADD CONSTRAINT embedding_entity_type_check_v2
    CHECK (entity_type IN ('contact', 'company', 'deal', 'lead', 'activity', 'project', 'relationship', 'partner', 'product', 'offer_template')) NOT VALID;
ALTER TABLE field_provenance ADD CONSTRAINT field_provenance_object_type_check_v2
    CHECK (object_type IN ('contact', 'company', 'deal', 'lead', 'activity', 'project', 'relationship', 'partner', 'product', 'offer_template')) NOT VALID;
