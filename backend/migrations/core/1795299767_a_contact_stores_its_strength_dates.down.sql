-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

DROP TRIGGER IF EXISTS contact_strength_dates_archive ON contact;
DROP TRIGGER IF EXISTS activity_strength_dates ON activity;
DROP TRIGGER IF EXISTS activity_link_strength_dates_added ON activity_link;
DROP TRIGGER IF EXISTS activity_link_strength_dates_removed ON activity_link;
DROP TRIGGER IF EXISTS activity_link_strength_dates_moved ON activity_link;
DROP TRIGGER IF EXISTS activity_participant_strength_dates_added ON activity_participant;
DROP TRIGGER IF EXISTS activity_participant_strength_dates_removed ON activity_participant;
DROP TRIGGER IF EXISTS activity_participant_strength_dates_moved ON activity_participant;
DROP TRIGGER IF EXISTS contact_email_strength_dates_added ON contact_email;
DROP TRIGGER IF EXISTS contact_email_strength_dates_removed ON contact_email;
DROP TRIGGER IF EXISTS contact_email_strength_dates_moved ON contact_email;
DROP TRIGGER IF EXISTS capture_import_strength_dates_added ON capture_import;
DROP TRIGGER IF EXISTS capture_import_strength_dates_removed ON capture_import;
DROP TRIGGER IF EXISTS capture_import_strength_dates_moved ON capture_import;
DROP TRIGGER IF EXISTS capture_thread_verdict_strength_dates_added ON capture_thread_verdict;
DROP TRIGGER IF EXISTS capture_thread_verdict_strength_dates_removed ON capture_thread_verdict;
DROP TRIGGER IF EXISTS capture_thread_verdict_strength_dates_moved ON capture_thread_verdict;

DROP FUNCTION IF EXISTS trg_contact_strength_dates_archive();
DROP FUNCTION IF EXISTS trg_activity_strength_dates();
DROP FUNCTION IF EXISTS trg_activity_link_strength_dates_added();
DROP FUNCTION IF EXISTS trg_activity_link_strength_dates_removed();
DROP FUNCTION IF EXISTS trg_activity_link_strength_dates_moved();
DROP FUNCTION IF EXISTS trg_activity_participant_strength_dates_added();
DROP FUNCTION IF EXISTS trg_activity_participant_strength_dates_removed();
DROP FUNCTION IF EXISTS trg_activity_participant_strength_dates_moved();
DROP FUNCTION IF EXISTS trg_contact_email_strength_dates_added();
DROP FUNCTION IF EXISTS trg_contact_email_strength_dates_removed();
DROP FUNCTION IF EXISTS trg_contact_email_strength_dates_moved();
DROP FUNCTION IF EXISTS trg_capture_import_strength_dates_added();
DROP FUNCTION IF EXISTS trg_capture_import_strength_dates_removed();
DROP FUNCTION IF EXISTS trg_capture_import_strength_dates_moved();
DROP FUNCTION IF EXISTS trg_capture_thread_verdict_strength_dates_added();
DROP FUNCTION IF EXISTS trg_capture_thread_verdict_strength_dates_removed();
DROP FUNCTION IF EXISTS trg_capture_thread_verdict_strength_dates_moved();
DROP FUNCTION IF EXISTS refresh_strength_dates_of_addresses(text[], uuid[]);
DROP FUNCTION IF EXISTS touch_strength_dates_of_holds(jsonb);
DROP FUNCTION IF EXISTS touch_strength_dates_of_activities(uuid[]);
DROP FUNCTION IF EXISTS touch_contact_strength_dates(uuid[], uuid[], timestamptz[]);
DROP FUNCTION IF EXISTS refresh_contact_strength_dates(uuid[]);
DROP FUNCTION IF EXISTS unfold_contact_strength_dates();
DROP FUNCTION IF EXISTS contact_strength_dates_among(uuid, uuid[]);
DROP FUNCTION IF EXISTS contact_strength_dates(uuid);

DROP INDEX IF EXISTS idx_contact_strength_unfolded;

ALTER TABLE contact
    DROP COLUMN IF EXISTS strength_folded,
    DROP COLUMN IF EXISTS strength_last_outbound_day,
    DROP COLUMN IF EXISTS strength_last_outbound_at,
    DROP COLUMN IF EXISTS strength_last_inbound_day,
    DROP COLUMN IF EXISTS strength_last_inbound_at,
    DROP COLUMN IF EXISTS strength_last_engaged_at,
    DROP COLUMN IF EXISTS strength_last_interaction_at;
