-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A second managed source, `gdocs`: the corpus the Google Docs extension files
-- into. A document in it carries the id its source gave it (`external_id`, the
-- Drive file id) so a changed source replaces the document instead of filing a
-- second one, and a small bag of facts read from the source (`meta`: owner,
-- department, effective date and so on) that a citation can show.
--
-- Both CHECKs are replaced as NOT VALID; the next migration validates them, in a
-- transaction of its own. Every stored row already passes: the old set was a
-- subset of the new one.
SET LOCAL lock_timeout = '3s';

ALTER TABLE knowledge_document ADD COLUMN external_id text;
ALTER TABLE knowledge_document ADD COLUMN meta jsonb;

ALTER TABLE knowledge_corpus DROP CONSTRAINT knowledge_corpus_managed_source_known;
ALTER TABLE knowledge_corpus ADD CONSTRAINT knowledge_corpus_managed_source_known
    CHECK (managed_source IS NULL OR managed_source IN ('handbook', 'gdocs')) NOT VALID;

ALTER TABLE knowledge_document DROP CONSTRAINT knowledge_document_managed_source_known;
ALTER TABLE knowledge_document ADD CONSTRAINT knowledge_document_managed_source_known
    CHECK (managed_source IS NULL OR managed_source IN ('handbook', 'gdocs')) NOT VALID;

-- An external id means something only inside a managed corpus, and the meta is
-- an object, never a bare scalar or array.
ALTER TABLE knowledge_document ADD CONSTRAINT knowledge_document_external_id_shape
    CHECK ((external_id IS NULL OR managed_source IS NOT NULL)
       AND (meta IS NULL OR jsonb_typeof(meta) = 'object')) NOT VALID;

-- One live document per source id per corpus, held by the schema: the upsert
-- reads before it writes, and two syncs can both run that read first.
CREATE UNIQUE INDEX knowledge_document_one_per_external_id
    ON knowledge_document (corpus_id, external_id)
    WHERE external_id IS NOT NULL AND archived_at IS NULL;
