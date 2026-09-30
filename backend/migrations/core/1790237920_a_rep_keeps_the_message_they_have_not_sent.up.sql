-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The message a rep has started and not sent, kept so that closing the composer
-- does not lose it.
--
-- NOT AN ACTIVITY and not a scheduled send. Both of those are facts about a
-- message that left the composer; this is the composer's own state, private to
-- the one seat that typed it. Nothing reads it but its author, and nothing on a
-- timeline, a count or a report ever sees it.
--
-- ONE PER AUTHOR AND ANCHOR. The anchor is what the composer opened against: the
-- activity being answered, or the record a new message starts from. A second
-- draft for the same place would make "which one did I mean" a question the rep
-- has to answer, so the unique key is the upsert's conflict target.
--
-- The anchor is a type and an id rather than a foreign key per type. Nothing
-- joins through it — the read probes the anchor's row scope by table name, and a
-- draft whose anchor has gone is answered as not found by that same probe.
--
-- The author cascades rather than restricts, unlike scheduled_send.scheduled_by:
-- a scheduled send records who authorized a message that will go out, and a
-- draft authorized nothing, so it has no reason to outlive the seat that wrote it.
CREATE TABLE mail_draft (
    id uuid NOT NULL,
    author_id uuid NOT NULL,
    anchor_type text NOT NULL,
    anchor_id uuid NOT NULL,
    -- Addresses as the rep typed them, which a draft may hold half-typed. They
    -- are validated when the message is sent, never here.
    to_addresses text[] DEFAULT '{}'::text[] NOT NULL,
    cc_addresses text[] DEFAULT '{}'::text[] NOT NULL,
    bcc_addresses text[] DEFAULT '{}'::text[] NOT NULL,
    subject text DEFAULT ''::text NOT NULL,
    body text DEFAULT ''::text NOT NULL,
    html_body text,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT mail_draft_pkey PRIMARY KEY (id),
    CONSTRAINT mail_draft_author_id_fkey FOREIGN KEY (author_id) REFERENCES app_user(id) ON DELETE CASCADE,
    CONSTRAINT mail_draft_anchor_type CHECK (anchor_type IN ('activity', 'contact', 'company', 'deal', 'lead', 'project')),
    CONSTRAINT mail_draft_one_per_anchor UNIQUE (author_id, anchor_type, anchor_id)
);
