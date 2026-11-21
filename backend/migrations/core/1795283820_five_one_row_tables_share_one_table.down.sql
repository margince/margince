-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The five tables come back, each with the row the shared table held for it.
SET LOCAL lock_timeout = '3s';

CREATE TABLE activity_clock_refold_mark (
    singleton boolean DEFAULT true NOT NULL,
    passed_up_to timestamptz NOT NULL,
    passed_id uuid NOT NULL,
    CONSTRAINT activity_clock_refold_mark_pkey PRIMARY KEY (singleton),
    CONSTRAINT activity_clock_refold_mark_singleton_check CHECK (singleton)
);
INSERT INTO activity_clock_refold_mark (passed_up_to, passed_id)
SELECT (value->>'passed_up_to')::timestamptz, (value->>'passed_id')::uuid
  FROM installation_state WHERE key = 'activity_clock_refold_mark';

CREATE TABLE activity_waiting_horizon (
    singleton boolean DEFAULT true NOT NULL,
    horizon_days integer NOT NULL,
    measured_at timestamptz NOT NULL,
    CONSTRAINT activity_waiting_horizon_pkey PRIMARY KEY (singleton),
    CONSTRAINT activity_waiting_horizon_singleton_check CHECK (singleton),
    CONSTRAINT activity_waiting_horizon_days_check CHECK (horizon_days > 0)
);
INSERT INTO activity_waiting_horizon (horizon_days, measured_at)
SELECT (value->>'horizon_days')::integer, (value->>'measured_at')::timestamptz
  FROM installation_state WHERE key = 'activity_waiting_horizon';

CREATE TABLE capture_answered_sweep_cursor (
    singleton boolean DEFAULT true NOT NULL,
    after_contact_id uuid,
    updated_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT capture_answered_sweep_cursor_pkey PRIMARY KEY (singleton),
    CONSTRAINT capture_answered_sweep_cursor_one_row CHECK (singleton)
);
INSERT INTO capture_answered_sweep_cursor (after_contact_id, updated_at)
SELECT (value->>'after_contact_id')::uuid, updated_at
  FROM installation_state WHERE key = 'capture_answered_sweep_cursor';

CREATE TABLE capture_party_admission_cursor (
    singleton boolean DEFAULT true NOT NULL,
    after_activity uuid NOT NULL,
    after_user uuid NOT NULL,
    ceiling uuid NOT NULL,
    CONSTRAINT capture_party_admission_cursor_pkey PRIMARY KEY (singleton),
    CONSTRAINT capture_party_admission_cursor_singleton_check CHECK (singleton)
);
INSERT INTO capture_party_admission_cursor (after_activity, after_user, ceiling)
SELECT (value->>'after_activity')::uuid, (value->>'after_user')::uuid, (value->>'ceiling')::uuid
  FROM installation_state WHERE key = 'capture_party_admission_cursor';

CREATE TABLE deal_authorship_sweep_cursor (
    singleton boolean DEFAULT true NOT NULL,
    after_evidence_id uuid,
    updated_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT deal_authorship_sweep_cursor_pkey PRIMARY KEY (singleton),
    CONSTRAINT deal_authorship_sweep_cursor_one_row CHECK (singleton)
);
INSERT INTO deal_authorship_sweep_cursor (after_evidence_id, updated_at)
SELECT (value->>'after_evidence_id')::uuid, updated_at
  FROM installation_state WHERE key = 'deal_authorship_sweep_cursor';

GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE
    activity_clock_refold_mark, activity_waiting_horizon, capture_answered_sweep_cursor,
    capture_party_admission_cursor, deal_authorship_sweep_cursor TO margince_app;

DROP TABLE installation_state;
