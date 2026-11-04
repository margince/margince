-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A finished transcript reading keeps what it did as facts: the tasks per
-- assignee, the proposals per seat they wait on, the withheld reasons and the
-- failure code. The report is phrased from them when it is read, in the
-- reader's language. Readings from before keep only status_detail.
SET LOCAL lock_timeout = '3s';

ALTER TABLE transcript_read ADD COLUMN outcome jsonb;
