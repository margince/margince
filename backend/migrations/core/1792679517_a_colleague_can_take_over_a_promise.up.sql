-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '5s';

-- Who took over a promise nobody was asked to keep, and when. Set together with
-- task_activity_id by the one writer, contacts.RecordPromiseTakenTx. A member
-- who is deleted later clears the name and leaves the time.
ALTER TABLE conversation_claim
    ADD COLUMN taken_by_user_id uuid,
    ADD COLUMN taken_at timestamptz;

-- NOT VALID: the table holds history, and the scan runs in the next migration
-- under a lock readers and writers pass. Every existing row is NULL here.
ALTER TABLE conversation_claim
    ADD CONSTRAINT conversation_claim_taker_fkey
        FOREIGN KEY (taken_by_user_id) REFERENCES app_user(id) ON DELETE SET NULL NOT VALID;
