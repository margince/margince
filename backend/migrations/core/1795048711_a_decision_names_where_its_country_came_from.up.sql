-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Where the recipient's country a send decision followed came from: the
-- contact's address, the employer's, the lead's company, or the sender, who
-- stated or cleared it for this one message. NULL on a row written before this
-- column, or one whose recipient no record or sender gave a country.
SET LOCAL lock_timeout = '3s';

ALTER TABLE communication_decision ADD COLUMN IF NOT EXISTS country_source text;
