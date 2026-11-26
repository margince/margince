-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Validates the three CHECKs the previous migration added NOT VALID, in a
-- transaction of its own so the scan runs under SHARE UPDATE EXCLUSIVE.
SET LOCAL lock_timeout = '3s';

ALTER TABLE knowledge_corpus VALIDATE CONSTRAINT knowledge_corpus_managed_source_known;
ALTER TABLE knowledge_document VALIDATE CONSTRAINT knowledge_document_managed_source_known;
ALTER TABLE knowledge_document VALIDATE CONSTRAINT knowledge_document_external_id_shape;
