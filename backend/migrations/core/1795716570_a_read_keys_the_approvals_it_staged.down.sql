-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Each read gets its approval list back as an array, in the order it staged them.
SET LOCAL lock_timeout = '3s';

ALTER TABLE site_read ADD COLUMN proposal_ids uuid[] DEFAULT '{}'::uuid[] NOT NULL;
ALTER TABLE transcript_read ADD COLUMN proposal_ids uuid[] DEFAULT '{}'::uuid[] NOT NULL;

UPDATE site_read r
   SET proposal_ids = staged.ids
  FROM (SELECT site_read_id, array_agg(approval_id ORDER BY position) AS ids
          FROM site_read_proposal GROUP BY site_read_id) staged
 WHERE staged.site_read_id = r.id;

UPDATE transcript_read r
   SET proposal_ids = staged.ids
  FROM (SELECT transcript_read_id, array_agg(approval_id ORDER BY position) AS ids
          FROM transcript_read_proposal GROUP BY transcript_read_id) staged
 WHERE staged.transcript_read_id = r.id;

DROP TABLE site_read_proposal;
DROP TABLE transcript_read_proposal;
