-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

-- A gdocs row cannot survive the narrower CHECK, so it goes first. Its chunks
-- go with it by the foreign key.
DELETE FROM knowledge_document WHERE managed_source = 'gdocs';
DELETE FROM knowledge_corpus WHERE managed_source = 'gdocs';

DROP INDEX knowledge_document_one_per_external_id;
ALTER TABLE knowledge_document DROP CONSTRAINT knowledge_document_external_id_shape;

ALTER TABLE knowledge_document DROP CONSTRAINT knowledge_document_managed_source_known;
ALTER TABLE knowledge_document ADD CONSTRAINT knowledge_document_managed_source_known
    CHECK (managed_source IS NULL OR managed_source IN ('handbook')) NOT VALID;

ALTER TABLE knowledge_corpus DROP CONSTRAINT knowledge_corpus_managed_source_known;
ALTER TABLE knowledge_corpus ADD CONSTRAINT knowledge_corpus_managed_source_known
    CHECK (managed_source IS NULL OR managed_source IN ('handbook')) NOT VALID;

ALTER TABLE knowledge_document DROP COLUMN meta;
ALTER TABLE knowledge_document DROP COLUMN external_id;
