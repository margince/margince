-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A sent report edition keeps the answer each question card gave its sender,
-- with every record behind each answer row. A reader is shown a row only while
-- they may read every one of those records, and retention, legal holds, subject
-- access and erasure reach the answers through the same records.
SET LOCAL lock_timeout = '3s';

CREATE TABLE report_edition_question (
    edition_id uuid NOT NULL REFERENCES report_edition(id) ON DELETE CASCADE,
    card_id text NOT NULL,
    entity text NOT NULL,
    -- Null when the answer was not kept; reason then says why.
    answer jsonb,
    reason text,
    PRIMARY KEY (edition_id, card_id),
    CONSTRAINT report_edition_question_kept_or_reasoned CHECK ((answer IS NULL) <> (reason IS NULL)),
    CONSTRAINT report_edition_question_reason_known CHECK (reason IN ('retention_unconfigured', 'too_many_records', 'unanswered'))
);

CREATE TABLE report_edition_question_source (
    edition_id uuid NOT NULL,
    card_id text NOT NULL,
    row_index integer NOT NULL,
    source_type text NOT NULL,
    source_id uuid NOT NULL,
    PRIMARY KEY (edition_id, card_id, row_index, source_id),
    FOREIGN KEY (edition_id, card_id) REFERENCES report_edition_question(edition_id, card_id) ON DELETE CASCADE
);
CREATE INDEX report_question_source ON report_edition_question_source(source_type, source_id);
