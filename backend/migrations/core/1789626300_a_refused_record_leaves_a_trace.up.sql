-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A connector whose every record the core refuses must stop looking like a
-- quiet one.
--
-- A unit moves its cursor past a record the grammar cannot express — stopping
-- on one malformed message parks the whole connection — so the drop leaves the
-- unit's own logs and nothing else. An installation reading only the CRM then
-- cannot tell a provider format change from a feed with nothing on it. The unit
-- counting its own drops does not help: the evidence lives where the core
-- cannot see it, which is the whole complaint.
--
-- COUNTS AND A CLASS. Never the record, and never the core's own sentence about
-- it: that sentence quotes the record back — a participant's account id, a
-- provider name, a role the unit invented — and storing it would give the
-- extension tier a retention and erasure question about untrusted third-party
-- content that the tier was deliberately built without. The class is the core's
-- own closed vocabulary (extension.RecordRefusal), and it is the granularity an
-- operator acts on: "every record from this connector fails its participants"
-- names the mapping to fix.
--
-- One row per unit per class per day, so a connector refusing a million records
-- costs one row rather than a million. The day is the installation's own date
-- in UTC; an operator reading a week of counts is not asking a question that
-- turns on which side of midnight a record landed.
CREATE TABLE extension_ingest_refusal (
    unit text NOT NULL,
    refusal text NOT NULL,
    day date NOT NULL,
    refused bigint NOT NULL,
    first_at timestamptz NOT NULL,
    last_at timestamptz NOT NULL,
    CONSTRAINT extension_ingest_refusal_pkey PRIMARY KEY (unit, refusal, day),
    -- A count is the whole content of the row; a zero would be a row saying
    -- nothing happened, which is what an absent row already says.
    CONSTRAINT extension_ingest_refusal_counted CHECK (refused > 0),
    -- The core's own closed vocabulary (extension.RecordRefusal), one value per
    -- check the ingress grammar runs. Spelled here as well as in Go so a class
    -- the code learned and the schema did not fails the write rather than
    -- landing a value no reader of this table has a name for.
    CONSTRAINT extension_ingest_refusal_class CHECK (
        refusal IN ('key', 'activity', 'addresses', 'counterparty', 'participants', 'size'))
);

-- The read is "what has this installation refused lately", newest first.
CREATE INDEX extension_ingest_refusal_day_idx ON extension_ingest_refusal (day DESC, unit);
