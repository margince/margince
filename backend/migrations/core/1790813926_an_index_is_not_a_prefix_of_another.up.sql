-- Fifteen indexes held a strict PREFIX of the columns of another index on the
-- same table, with the same predicate and modifiers. A B-tree on (a, b) answers
-- every lookup on (a), so the narrow one is read by nothing the wide one cannot
-- serve and costs a second index entry on every insert and update of the row.
--
-- Three exclusions, and each is a way the rule would otherwise take something
-- load-bearing:
--
-- A PRIMARY KEY also prefixes a wider unique on six tables (deal_room_document,
-- deal_room_participant, passport, provider_run, sdr_handoff_reason, stage). A
-- primary key is not a read path — it is the table's identity and what its
-- inbound foreign keys reference.
--
-- An index BACKING a unique constraint goes with the constraint if dropped, so
-- it is never the redundant half of a pair.
--
-- A PARTIAL index covers nothing outside its predicate, so it cannot stand in
-- for a plain one. activity_retention_evidence(activity_id) and
-- preference_token(contact_id) look covered by a wider unique and are not: both
-- of those are partial, and both columns carry an ON DELETE CASCADE whose delete
-- has to find every child row, including the ones the predicate leaves out.
SET LOCAL lock_timeout = '3s';

DROP INDEX idx_ai_feedback_subject;
DROP INDEX capture_counterparty_hold_user_idx;
DROP INDEX idx_contact_profile_field;
DROP INDEX idx_contact_social_contact;
DROP INDEX idx_dsh_deal;
DROP INDEX idx_forecast_contribution_snapshot;
DROP INDEX idx_list_member_list;
DROP INDEX idx_notice_recipient_user;
DROP INDEX idx_record_grant_record;
DROP INDEX idx_taggable_tag;
DROP INDEX idx_team_membership_team;
DROP INDEX voice_corpus_source_profile_fk;
DROP INDEX voice_profile_delta_profile_fk;
DROP INDEX voice_profile_version_profile_fk;
DROP INDEX idx_weekly_review_driver_bar;
