-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Each read still holds its list in proposal_ids, so the keyed copies go.
SET LOCAL lock_timeout = '3s';

DROP TABLE site_read_proposal;
DROP TABLE transcript_read_proposal;
