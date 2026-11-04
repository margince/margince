-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- How a logged call ended, from one fixed list: reached, not_reached,
-- voicemail, call_back_later, put_through. Optional, and only on a call. Every
-- call logged before this column reads NULL, which counts as contact as every
-- call did.
--
-- NOT VALID, validated by the next migration in its own transaction, so the
-- scan holds SHARE UPDATE EXCLUSIVE rather than this ALTER's lock.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity
    ADD COLUMN call_outcome text,
    ADD CONSTRAINT activity_call_outcome_check
        CHECK (call_outcome IS NULL OR call_outcome IN ('reached', 'not_reached', 'voicemail', 'call_back_later', 'put_through')) NOT VALID,
    ADD CONSTRAINT activity_call_outcome_kind_check
        CHECK (call_outcome IS NULL OR kind = 'call') NOT VALID;
