// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The messages half of a subject-access package: what was captured, what was
// sent to the subject, and what is still waiting — scheduled, staged or
// drafted. Split from sar.go because these projections carry the file's
// hardest rule — which addresses may be disclosed to whom — and it is easier
// to check when it is not interleaved with the identity and provenance
// sections.

import (
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// sarMessagingSections gather both directions of the messaging boundary: what
// capture decided about mail arriving from the subject, and what this
// installation sent out about or to them.
func sarMessagingSections(pkg *SARPackage, contactID ids.ContactID, emails []string, leads, messages []ids.UUID) []sarSection {
	sections := []sarSection{
		sarPartySourceSection(pkg, emails),
		{&pkg.CaptureDispositions, `SELECT p.email, p.display_name, p.status, p.disposition_reason, p.created_at, p.resolved_at
		   FROM capture_pending_counterparty p
		   WHERE p.email IN (SELECT email FROM contact_email WHERE contact_id = $1)`, nil},
		sarSentSection(pkg, contactID, messages),
		// The messages nobody has sent yet. They hold the subject's address and
		// the body BEFORE any activity exists, so none of the three routes the
		// query above takes can reach them — and a message written to this
		// contact, sitting unsent, is data held about them that Art. 15 owes
		// them sight of.
		//
		// Same two-sided address rule as the sent projection, pulling the same
		// two ways: the row is FOUND on any addressee including a blind one, so
		// a bcc'd subject is not absent from their own export; and the blind
		// list is REDUCED to the asker, so they do not learn who else was
		// blind-copied. Exporting the whole bcc array would hand one subject
		// every other blind recipient's address, which is the disclosure a
		// blind copy exists to prevent.
		//
		// status and held_reason are disclosed because they are the honest
		// answer to "what is happening to this message" — waiting, withdrawn,
		// or stopped by a gate and waiting for a human. Showing the content
		// while hiding the state would tell the subject a message exists
		// without telling them whether it is still going to arrive.
		{&pkg.ScheduledMessages, `SELECT s.payload->>'subject' AS subject,
		      s.payload->>'body' AS body,
		      s.payload->>'html_body' AS html_body,
		      -- The To LINE, derived by subtracting cc and bcc from the merged
		      -- list. payload.recipients is the CONSENT superset — every To, Cc
		      -- AND Bcc address — which is the right shape for a gate and the
		      -- wrong one to disclose: handing it over verbatim would give this
		      -- subject every other blind recipient's address, which is exactly
		      -- what narrowing the bcc column below exists to prevent. The sent
		      -- projection above needs no such subtraction because
		      -- comms_outbound keeps the To line and the blind list apart.
		      --
		      -- Compared case-insensitively and trimmed, which is what
		      -- activities.normalizeAddress does on the send side. Matching raw
		      -- would leave a differently-cased blind address in the To line
		      -- here while the send itself correctly removed it — the one place
		      -- the two derivations must not disagree.
		      (SELECT coalesce(jsonb_agg(addr), '[]'::jsonb)
		         FROM jsonb_array_elements_text(coalesce(s.payload->'recipients', '[]'::jsonb)) AS addr
		        WHERE lower(btrim(addr)) NOT IN (
		                SELECT lower(btrim(c))
		                  FROM jsonb_array_elements_text(coalesce(s.payload->'cc', '[]'::jsonb)) AS c
		                UNION ALL
		                SELECT lower(btrim(b))
		                  FROM jsonb_array_elements_text(coalesce(s.payload->'bcc', '[]'::jsonb)) AS b)) AS recipients,
		      s.payload->'cc' AS cc,
		      (SELECT coalesce(jsonb_agg(addr), '[]'::jsonb)
		         FROM jsonb_array_elements_text(coalesce(s.payload->'bcc', '[]'::jsonb)) AS addr
		        WHERE lower(addr) IN (SELECT email FROM contact_email WHERE contact_id = $1)) AS bcc,
		      s.payload->>'consent_purpose' AS consent_purpose,
		      s.status, s.held_reason, s.scheduled_at, s.scheduled_tz, s.created_at
		   FROM scheduled_send s
		   WHERE EXISTS (
		           SELECT 1 FROM jsonb_array_elements_text(
		                          coalesce(s.payload->'recipients', '[]'::jsonb)
		                          || coalesce(s.payload->'cc', '[]'::jsonb)
		                          || coalesce(s.payload->'bcc', '[]'::jsonb)) AS addr
		           WHERE lower(addr) IN (SELECT email FROM contact_email WHERE contact_id = $1))`, nil},
		// The messages still waiting for a human to decide them. A staged
		// approval holds a whole composed message before any scheduled row or
		// activity exists, so neither section above can see it — and a subject
		// asking what this installation holds about them is owed a draft
		// somebody is about to send them just as much as one already sent.
		//
		// Matched on the payload as TEXT rather than on a shape, because there
		// is no shape to match: proposed_change is per-kind JSON this package
		// does not parse. The looseness errs toward including a proposal that
		// merely mentions the subject, which is the right direction for an
		// export — Art. 15 owes them what is held, and a staged message naming
		// them is held about them whatever kind wrote it.
		//
		// Loose about SHAPE is not loose about WHO, and this is the half that
		// bites: an address dropped into a substring match hands a THIRD PARTY's
		// composed message to whoever asked. subjectApprovalMatch anchors it,
		// and the export shares that predicate with the erasure so a subject
		// cannot be told two different things about which rows are theirs.
		//
		// The export then reaches PAST it, onto the quotations, and the
		// containment runs one way ON PURPOSE: everything the erasure destroys
		// is listed here, and the export lists more besides. A row this reaches
		// and the cascade leaves alone is a proposal read out of a record the
		// cascade is not entitled to destroy — held, floor-shielded, or shared
		// with somebody else — and the subject is owed sight of it either way.
		// The reverse containment is the one that must never hold: a row
		// destroyed and never listed tells a subject nothing was staged about
		// them and then destroys it on the strength of the same reading.
		//
		// The proposal is returned whole, for the same reason the sent
		// messages' recipient lists are: this assembly is admin-mediated, so
		// the disclosure is a human handing a package to a subject rather than
		// an endpoint answering one.
		//
		// Evidence rides along because it is the part of a staging held in the
		// subject's OWN words — the verbatim lines a claim was read out of, a
		// sentence they spoke in a meeting. The proposal is what this
		// installation concluded; the evidence is what it kept of them in order
		// to conclude it, which is the half an Art. 15 answer would be strangest
		// to omit.
		//
		// It is the one column here that is NARROWED rather than returned whole,
		// and the bcc lists above are narrowed for the same reason. Everything
		// else on this row is text this installation COMPOSED — a summary, a
		// proposal — and the loose arms match on exactly that. Evidence is not
		// composed: it is a raw line lifted out of some record, so a row matched
		// because a summary mentions the subject's address would otherwise hand
		// them a verbatim sentence out of a meeting they were never part of,
		// about contacts they have no relationship to. So the ROW is found by any
		// arm, and the QUOTATIONS are reduced to the ones that are the subject's
		// to see: read out of a record they are linked to, or naming them
		// outright. The rest belongs to another record.
		//
		// Per ITEM, not per row, because evidence is per-claim: a proposal
		// asserting two things cites two sources, and only one of them may be
		// theirs.
		{
			&pkg.StagedMessages, `SELECT id, kind, status, summary, proposed_change,
		      (SELECT coalesce(jsonb_agg(item), '[]'::jsonb)
		         FROM jsonb_array_elements(` + evidenceArray + `) AS item
		        WHERE item->>'source_id' IN (
		                SELECT a.id::text FROM activity a WHERE a.id = ANY($4::uuid[]) AND ` + sarContentDisclosed + `)
		           OR item->>'evidence_snippet' ~* ANY($3::text[])) AS evidence,
		      created_at, expires_at, decided_at
		   FROM approval
		   WHERE (` + subjectApprovalMatch + `)
		      OR evidence::text ~* ANY($3::text[])
		      OR ` + evidenceCitesSubjectActivity,
			[]any{contactID.UUID, leads, addressPatterns(emails), messages},
		},
	}
	return append(append(sections, sarMeetingSections(pkg, messages)...), sarDraftSection(pkg, contactID, emails, leads))
}

// sarDraftSection exports the drafts subjectDraftMatch names — the same rows
// the erasure deletes. It follows the scheduled projection's blind-copy rule:
// any addressee finds the row, and the bcc line is reduced to the subject's
// own address. A draft keeps To apart from Cc and Bcc, so its To line needs no
// subtraction.
func sarDraftSection(pkg *SARPackage, contactID ids.ContactID, emails []string, leads []ids.UUID) sarSection {
	return sarSection{
		&pkg.DraftMessages, `SELECT d.subject, d.body, d.html_body,
              d.editor_state->>'intent' AS drafting_purpose,
              d.editor_state->>'served_body' AS generated_body,
		      to_jsonb(d.to_addresses) AS recipients, to_jsonb(d.cc_addresses) AS cc,
		      (SELECT coalesce(jsonb_agg(addr), '[]'::jsonb)
		         FROM unnest(d.bcc_addresses) AS addr
		        WHERE lower(btrim(addr)) = ANY($3::text[])) AS bcc,
		      -- Named as the sent projection names them, for its reason: a draft may
		      -- carry any file its author could see. A file gone since keeps its id.
		      (SELECT coalesce(jsonb_agg(jsonb_build_object(
		                'attachment_id', chosen.id, 'filename', f.filename,
		                'content_type', f.content_type, 'byte_size', f.byte_size) ORDER BY chosen.ord), '[]'::jsonb)
		         FROM unnest(d.attachment_ids) WITH ORDINALITY AS chosen(id, ord)
		         LEFT JOIN attachment f ON f.id = chosen.id) AS attachments,
		      d.communication_context, d.scheduled_at, d.scheduled_tz,
		      d.anchor_type, d.created_at, d.updated_at
		   FROM mail_draft d
		   WHERE ` + subjectDraftMatch,
		[]any{contactID.UUID, leads, loweredAddresses(emails)},
	}
}

func sarMeetingSections(pkg *SARPackage, messages []ids.UUID) []sarSection {
	args := []any{messages}
	return []sarSection{
		{&pkg.MeetingProposals, `SELECT DISTINCT ` + sarActivityDisclosed("p.activity_id") + ` AS content_disclosed,
 CASE WHEN ` + sarActivityDisclosed("p.activity_id") + ` THEN p.request END AS request,p.expires_at,p.used_at,p.created_at
 FROM meeting_proposal p WHERE p.activity_id = ANY($1)`, args},
		{&pkg.MeetingInvitations, `SELECT DISTINCT ` + sarActivityDisclosed("i.activity_id") + ` AS content_disclosed,
 CASE WHEN ` + sarActivityDisclosed("i.activity_id") + ` THEN i.appointment-'PassportID'-'RequestID'-'EventID'-'CalendarID' END AS appointment,
 i.status,
 CASE WHEN ` + sarActivityDisclosed("i.activity_id") + ` THEN i.command END AS command,i.created_at,i.updated_at
 FROM meeting_invitation i WHERE i.activity_id = ANY($1)`, args},
	}
}

func sarPartySourceSection(pkg *SARPackage, emails []string) sarSection {
	return sarSection{&pkg.CapturePartySources, `SELECT ps.activity_id,ps.provider,ps.sent_by_owner,
 ARRAY(SELECT address FROM unnest(ps.recipients) address WHERE lower(address)=ANY(ARRAY(SELECT lower(btrim(subject_address)) FROM unnest($1::text[]) subject_address))) AS recipients,
 ARRAY(SELECT address FROM unnest(ps.policy_addresses) address WHERE lower(address)=ANY(ARRAY(SELECT lower(btrim(subject_address)) FROM unnest($1::text[]) subject_address))) AS policy_addresses
 FROM capture_party_source ps JOIN activity a ON a.id=ps.activity_id
 WHERE (ps.recipients && ARRAY(SELECT lower(btrim(subject_address)) FROM unnest($1::text[]) subject_address) OR ps.policy_addresses && ARRAY(SELECT lower(btrim(subject_address)) FROM unnest($1::text[]) subject_address)) AND ` + sarContentDisclosed, []any{emails}}
}

// sarSentSection exports the governed sends about or to the subject. Any
// addressee finds a send, a blind copy included. The bcc list and the bounce
// facts are reduced to the subject's own address, so no other blind recipient
// is disclosed. Content follows the audience rule of the send's activity.
func sarSentSection(pkg *SARPackage, contactID ids.ContactID, messages []ids.UUID) sarSection {
	return sarSection{&pkg.SentMessages, `SELECT ` + sarActivityDisclosed("o.activity_id") + ` AS content_disclosed,
		      CASE WHEN ` + sarActivityDisclosed("o.activity_id") + ` THEN o.subject END AS subject,
		      CASE WHEN ` + sarActivityDisclosed("o.activity_id") + ` THEN o.body END AS body,
		      CASE WHEN ` + sarActivityDisclosed("o.activity_id") + ` THEN o.html_body END AS html_body,
		      o.from_name,
		      CASE WHEN ` + sarActivityDisclosed("o.activity_id") + ` THEN o.attachments END AS attachments,
		      CASE WHEN ` + sarActivityDisclosed("o.activity_id") + ` THEN o.recipients END AS recipients,
		      CASE WHEN ` + sarActivityDisclosed("o.activity_id") + ` THEN o.cc END AS cc,
		      CASE WHEN ` + sarActivityDisclosed("o.activity_id") + ` THEN (SELECT coalesce(jsonb_agg(addr), '[]'::jsonb)
		         FROM jsonb_array_elements_text(coalesce(o.bcc, '[]'::jsonb)) AS addr
		        WHERE lower(addr) IN (SELECT email FROM contact_email WHERE contact_id = $1)) END AS bcc,
		      o.consent_purpose,
		      o.provider, o.channel_user_id, o.status, o.sent_at, o.created_at,
		      CASE WHEN lower(coalesce(o.bounce_recipient, '')) IN (
		             SELECT email FROM contact_email WHERE contact_id = $1)
		           THEN o.bounced_at END AS bounced_at,
		      CASE WHEN lower(coalesce(o.bounce_recipient, '')) IN (
		             SELECT email FROM contact_email WHERE contact_id = $1)
		           THEN o.bounce_kind END AS bounce_kind,
		      CASE WHEN lower(coalesce(o.bounce_recipient, '')) IN (
		             SELECT email FROM contact_email WHERE contact_id = $1)
		           THEN o.bounce_recipient END AS bounce_recipient
		   FROM comms_outbound o
		   WHERE o.activity_id = ANY($2)
		      OR EXISTS (
		           SELECT 1 FROM jsonb_array_elements_text(
		                          o.recipients || o.cc || coalesce(o.bcc, '[]'::jsonb)) AS addr
		           WHERE lower(addr) IN (SELECT email FROM contact_email WHERE contact_id = $1))`, []any{contactID.UUID, messages}}
}

// sarActivityDisclosed asks sarContentDisclosed of the activity a section row
// belongs to. Every section that lists a row through the subject's message set
// withholds content by this rule, as Activities does.
func sarActivityDisclosed(activityID string) string {
	return `EXISTS (SELECT 1 FROM activity a WHERE a.id = ` + activityID + ` AND ` + sarContentDisclosed + `)`
}
