-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Five tables each held one row of bookkeeping for a background pass: a mark,
-- a measured number or the place a sweep stopped. They become five rows of one
-- table, keyed by the name the table had. The writer of a row decides the
-- shape of its value.
--
-- A workspace reset clears this table as it cleared the five, so a pass
-- starts over after one. State that a reset keeps does not belong here.
SET LOCAL lock_timeout = '3s';

CREATE TABLE installation_state (
    key text NOT NULL,
    value jsonb NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT installation_state_pkey PRIMARY KEY (key),
    CONSTRAINT installation_state_key_present CHECK (btrim(key) <> ''),
    CONSTRAINT installation_state_value_is_an_object CHECK (jsonb_typeof(value) = 'object')
);

GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE installation_state TO margince_app;

CREATE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."installation_state" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"key":20000}');
CREATE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."installation_state" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"key":20000}');

INSERT INTO installation_state (key, value)
SELECT 'activity_clock_refold_mark',
       jsonb_build_object('passed_up_to', passed_up_to, 'passed_id', passed_id)
  FROM activity_clock_refold_mark;

INSERT INTO installation_state (key, value)
SELECT 'activity_waiting_horizon',
       jsonb_build_object('horizon_days', horizon_days, 'measured_at', measured_at)
  FROM activity_waiting_horizon;

INSERT INTO installation_state (key, value, updated_at)
SELECT 'capture_answered_sweep_cursor',
       jsonb_build_object('after_contact_id', after_contact_id), updated_at
  FROM capture_answered_sweep_cursor;

INSERT INTO installation_state (key, value)
SELECT 'capture_party_admission_cursor',
       jsonb_build_object('after_activity', after_activity, 'after_user', after_user, 'ceiling', ceiling)
  FROM capture_party_admission_cursor;

INSERT INTO installation_state (key, value, updated_at)
SELECT 'deal_authorship_sweep_cursor',
       jsonb_build_object('after_evidence_id', after_evidence_id), updated_at
  FROM deal_authorship_sweep_cursor;

DROP TABLE activity_clock_refold_mark;
DROP TABLE activity_waiting_horizon;
DROP TABLE capture_answered_sweep_cursor;
DROP TABLE capture_party_admission_cursor;
DROP TABLE deal_authorship_sweep_cursor;
