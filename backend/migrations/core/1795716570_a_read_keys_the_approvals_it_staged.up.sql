-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A site read and a transcript read each list the approvals they staged. A
-- uuid[] cannot carry a foreign key, so each list becomes a table whose rows
-- key the approval, and position keeps the order the read staged them in.
--
-- The copy keeps an id only once and only while its approval exists: the key
-- cannot hold an id that names no approval.
SET LOCAL lock_timeout = '3s';

CREATE TABLE site_read_proposal (
    site_read_id uuid NOT NULL,
    approval_id uuid NOT NULL,
    position integer NOT NULL,
    CONSTRAINT site_read_proposal_pkey PRIMARY KEY (site_read_id, approval_id),
    CONSTRAINT site_read_proposal_position_key UNIQUE (site_read_id, position),
    CONSTRAINT site_read_proposal_position_check CHECK (position >= 1),
    CONSTRAINT site_read_proposal_site_read_fkey FOREIGN KEY (site_read_id) REFERENCES site_read(id) ON DELETE CASCADE,
    CONSTRAINT site_read_proposal_approval_fkey FOREIGN KEY (approval_id) REFERENCES approval(id) ON DELETE CASCADE
);
CREATE INDEX site_read_proposal_approval_ix ON site_read_proposal (approval_id);
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE site_read_proposal TO margince_app;

CREATE TABLE transcript_read_proposal (
    transcript_read_id uuid NOT NULL,
    approval_id uuid NOT NULL,
    position integer NOT NULL,
    CONSTRAINT transcript_read_proposal_pkey PRIMARY KEY (transcript_read_id, approval_id),
    CONSTRAINT transcript_read_proposal_position_key UNIQUE (transcript_read_id, position),
    CONSTRAINT transcript_read_proposal_position_check CHECK (position >= 1),
    CONSTRAINT transcript_read_proposal_transcript_read_fkey FOREIGN KEY (transcript_read_id) REFERENCES transcript_read(id) ON DELETE CASCADE,
    CONSTRAINT transcript_read_proposal_approval_fkey FOREIGN KEY (approval_id) REFERENCES approval(id) ON DELETE CASCADE
);
CREATE INDEX transcript_read_proposal_approval_ix ON transcript_read_proposal (approval_id);
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE transcript_read_proposal TO margince_app;

INSERT INTO site_read_proposal (site_read_id, approval_id, position)
SELECT r.id, staged.approval_id, min(staged.n)
  FROM site_read r
 CROSS JOIN LATERAL unnest(r.proposal_ids) WITH ORDINALITY AS staged(approval_id, n)
  JOIN approval a ON a.id = staged.approval_id
 GROUP BY r.id, staged.approval_id;

INSERT INTO transcript_read_proposal (transcript_read_id, approval_id, position)
SELECT r.id, staged.approval_id, min(staged.n)
  FROM transcript_read r
 CROSS JOIN LATERAL unnest(r.proposal_ids) WITH ORDINALITY AS staged(approval_id, n)
  JOIN approval a ON a.id = staged.approval_id
 GROUP BY r.id, staged.approval_id;

ALTER TABLE site_read DROP COLUMN proposal_ids;
ALTER TABLE transcript_read DROP COLUMN proposal_ids;
