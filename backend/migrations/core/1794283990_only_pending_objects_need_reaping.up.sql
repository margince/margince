-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

CREATE INDEX stored_object_intent_pending_idx
ON stored_object_intent (coalesce(reaping_since, recorded_at), storage_key)
WHERE reaped_at IS NULL;
