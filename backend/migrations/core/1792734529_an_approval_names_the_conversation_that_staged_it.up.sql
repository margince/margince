-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '5s';

-- The colleague whose own live conversation with an AI client staged this
-- approval, NULL for every other staging. The approval email skips a decider who
-- is that colleague: the conversation already shows them the request.
ALTER TABLE approval ADD COLUMN staged_in_conversation_of uuid;

-- NOT VALID: the table holds history, and the scan runs in the next migration
-- under a lock readers and writers pass. Every existing row is NULL here.
ALTER TABLE approval
    ADD CONSTRAINT approval_staged_in_conversation_of_fkey
        FOREIGN KEY (staged_in_conversation_of) REFERENCES app_user(id) ON DELETE SET NULL NOT VALID;
