// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The Art. 15 gather list: WHICH tables hold data about a subject, and the
// query that reads each. Apart from sar.go, which is how the export RUNS —
// the permission gate, the transaction, the row scan. The split is by the
// question each answers, and this half is the compliance-critical one: adding
// or dropping a source here changes what the export owes the data subject.

import (
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// sarSections is the Art. 15 gather list: the exact set of tables that hold
// data about the subject, each bound to the package field it populates. The
// query set is compliance-critical — adding or dropping a source changes what
// the export owes the data subject. It is assembled chapter by chapter, and the
// order the chapters concatenate in is the order the export runs them in.
func sarSections(pkg *SARPackage, personID ids.PersonID, emails []string, leads []ids.UUID) []sarSection {
	sections := sarIdentitySections(pkg)
	sections = append(sections, sarRecordSections(pkg)...)
	sections = append(sections, sarMessagingSections(pkg, personID, emails, leads)...)
	sections = append(sections, sarConsentSections(pkg)...)
	return append(sections, sarProvenanceSections(pkg)...)
}

// sarIdentitySections gather how the subject is identified and where they are
// named: their own addresses, numbers and channel accounts, the conversations
// they were recorded as a party to, and the imported networks they appear in.
func sarIdentitySections(pkg *SARPackage) []sarSection {
	return []sarSection{
		// The three identifier sections export ARCHIVED rows alongside live
		// ones: Art. 15 owes what is HELD, and a retired address, number or
		// channel binding is still a record of how the subject was reached, and
		// of which account wrote as them. Each therefore carries archived_at.
		// Without it every identifier in the export reads as current, so the
		// subject cannot tell a retirement that happened from one that did not —
		// in the very package they would check it in.
		{&pkg.Emails, `SELECT email, email_type, is_primary, archived_at FROM person_email WHERE person_id = $1`, nil},
		{&pkg.Phones, `SELECT phone, phone_type, archived_at FROM person_phone WHERE person_id = $1`, nil},
		{&pkg.ChannelIdentities, `SELECT provider, channel_user_id, username, blocked_at, source, created_at, archived_at
		   FROM person_channel_identity WHERE person_id = $1`, nil},
		{&pkg.InteractionParticipation, `SELECT ap.activity_id, ap.role, ap.address, ap.created_at,
		       a.kind, a.occurred_at, a.direction
		   FROM activity_participant ap
		   JOIN activity a ON a.id = ap.activity_id
		  WHERE ap.person_id = $1
		     OR (ap.address IS NOT NULL AND ap.address IN (
		         SELECT lower(email) FROM person_email WHERE person_id = $1))`, nil},
		// The same reach erasure uses: matched, or carrying their address, or
		// bearing their name at an employer they actually work for. Art. 15
		// owes what is HELD, and an unmatched ghost holds their name and
		// employer just as surely as a confirmed one does.
		{&pkg.LinkedInConnections, `SELECT full_name, position, company_name, connected_on,
		       email, profile_url, match_status, source, synced_at
		   FROM linkedin_connection g
		  WHERE g.matched_person_id = $1
		     OR (g.email IS NOT NULL AND g.email IN (
		         SELECT lower(email) FROM person_email WHERE person_id = $1))
		     -- The profile URL is an identifier the subject is reachable by,
		     -- and it is held about them whether or not the matcher ever
		     -- linked the row. A package that omitted it would answer "what do
		     -- you hold about me" with less than is held.
		     OR (g.profile_url IS NOT NULL AND g.profile_url IN (
		         SELECT handle FROM person_social
		          WHERE person_id = $1 AND platform = 'linkedin'))
		     OR (g.normalized_company IS NOT NULL
		         AND g.normalized_name = (SELECT lower(f_unaccent(full_name)) FROM person WHERE id = $1)
		         AND EXISTS (
		             SELECT 1 FROM relationship r
		              WHERE r.person_id = $1 AND r.kind = 'employment'
		                AND r.archived_at IS NULL
		                AND r.organization_id = g.matched_org_id))`, nil},
	}
}

// sarRecordSections gather the business records the subject appears in: who
// they are connected to, the deals they hold a stake in, the leads they came
// from, and their timeline with the files hanging off it.
func sarRecordSections(pkg *SARPackage) []sarSection {
	return []sarSection{
		{&pkg.Relationships, `SELECT kind, organization_id, deal_id, role, started_at, ended_at
		   FROM relationship WHERE person_id = $1 AND archived_at IS NULL`, nil},
		{&pkg.Deals, `SELECT d.id, d.name, d.status, d.amount_minor, d.currency
		   FROM deal d JOIN relationship r ON r.deal_id = d.id
		   WHERE r.kind = 'deal_stakeholder' AND r.person_id = $1 AND r.archived_at IS NULL`, nil},
		{&pkg.Leads, `SELECT l.id, l.full_name, l.email, l.title, l.company_name, l.status, l.created_at
		   FROM lead l
		   WHERE l.promoted_person_id = $1
		      OR l.id IN (SELECT converted_from_lead_id FROM person WHERE id = $1 AND converted_from_lead_id IS NOT NULL)
		      OR (l.email IS NOT NULL AND EXISTS (
		            SELECT 1 FROM person_email pe WHERE pe.person_id = $1 AND pe.email = lower(l.email)))`, nil},
		// Art. 15 reaches a record the statutory floor is HOLDING about this
		// subject, and does so deliberately: the restriction bars further
		// PROCESSING of the record, not the subject's own access to what is
		// held about them (A165/ADR-0114 — the subject's Art. 15 access is
		// named among what does not change). Every other reader in this tree
		// excludes `restricted_at IS NOT NULL`; this is the one that must not,
		// and saying so here is what keeps the omission a decision rather than
		// an oversight the next reader "fixes".
		//
		// The AUDIENCE is a different obligation from the statutory hold above,
		// and it goes the other way. A restriction bars processing of a record
		// the workspace holds ABOUT the subject, and Art. 15 pierces it. A
		// limited audience says the message's content belongs to the mailbox
		// that captured it and to the humans on it — the mailbox owner's own
		// correspondence, quoted third parties included. Handing that text to
		// whoever operates the SAR would disclose a colleague's private mail to
		// a person the message's audience excludes, in a package the subject
		// then holds a copy of. So a limited message is DISCLOSED, with the
		// fields that prove it exists — id, kind, when, where it came from, and
		// which mailboxes hold it — and its subject and body are withheld
		// pending a release by one of those mailbox owners.
		{&pkg.Activities, `SELECT a.id, a.kind, a.occurred_at, a.source_system,
		       a.audience = 'workspace' AS content_disclosed,
		       CASE WHEN a.audience = 'workspace' THEN a.subject END AS subject,
		       CASE WHEN a.audience = 'workspace' THEN a.body END AS body,
		       CASE WHEN a.audience = 'workspace' THEN NULL
		            ELSE substring(a.captured_by from '([0-9a-f-]{36})$')
		       END AS withheld_from_mailbox_of
		   FROM activity a JOIN activity_link l ON l.activity_id = a.id
		   WHERE l.person_id = $1`, nil},
		// A filename is content: `Aufhebungsvertrag_Mueller.pdf` states what the
		// message is about. An attachment of a limited message is therefore
		// listed by id and size with its name withheld, under the same rule as
		// the body above.
		{&pkg.Attachments, `SELECT at.id, at.entity_type, at.entity_id,
		      at.content_type, at.byte_size, at.created_at,
		      CASE WHEN at.entity_type <> 'activity' THEN at.filename
		           WHEN EXISTS (SELECT 1 FROM activity a
		                         WHERE a.id = at.entity_id AND a.audience = 'workspace')
		           THEN at.filename
		      END AS filename
		   FROM attachment at
		   WHERE (at.entity_type = 'person' AND at.entity_id = $1)
		      OR (at.entity_type = 'activity' AND at.entity_id IN (
		            SELECT l.activity_id FROM activity_link l WHERE l.person_id = $1))`, nil},
	}
}

// sarConsentSections gather the per-purpose consent state and the proof log
// behind it — what the subject agreed to, and every change of mind on record.
func sarConsentSections(pkg *SARPackage) []sarSection {
	return []sarSection{
		{&pkg.Consent, `SELECT cp.key AS purpose, pc.state, pc.lawful_basis, pc.captured_at
		   FROM person_consent pc JOIN consent_purpose cp ON cp.id = pc.purpose_id
		   WHERE pc.person_id = $1`, nil},
		{&pkg.ConsentEvents, `SELECT cp.key AS purpose, ce.new_state, ce.source, ce.captured_at
		   FROM consent_event ce JOIN consent_purpose cp ON cp.id = ce.purpose_id
		   WHERE ce.person_id = $1`, nil},
		// The token row itself is deliberately NOT read: it is a live
		// credential, and the subject already holds their own copy in the mail
		// that delivered it. Registered sarForbidden so a future section over
		// it fails the coverage gate rather than shipping.
		{&pkg.ConfirmSubmissions, `SELECT kind, field, proposed_value, submitted_at, resolution, resolved_at
		   FROM person_confirm_submission
		   WHERE person_id = $1`, nil},
	}
}

// sarProvenanceSections gather where the held data CAME FROM: the raw provider
// payloads the subject was captured out of, and the per-field record of who
// captured what from where.
func sarProvenanceSections(pkg *SARPackage) []sarSection {
	return []sarSection{
		// Reached two ways, like the erasure purge this mirrors (erasure.go's
		// purgeDerivedTraces): by email, ILIKE against the stored address, and
		// by channel identity, a typed JSONB path equality rather than a
		// substring match — a Telegram-only subject carries no email at all,
		// so the email arm alone would silently omit their entire channel
		// history from the export, and their sender id is a bare digit run
		// that a substring match would also match against other rows' message
		// ids, timestamps and other people's ids. The two payload shapes
		// matched (message.from.id, my_chat_member.chat.id) are the same two
		// capture/telegram's Normalize and ParseMembership read the customer's
		// id from — both update kinds land in raw_capture. The membership arm
		// reads the chat and not new_chat_member.user, which is the BOT
		// (capture/telegram/membership.go): keyed on that, a subject who only
		// ever blocked the bot would be handed an export missing the one
		// record the installation holds about them.
		//
		// The payload of a LIMITED message is withheld, and this is where the
		// withholding has to bite hardest: it is the provider original, so it
		// carries the message's full text and every header regardless of what
		// the activity row above discloses. Matching is by substring anywhere in
		// the payload, so a subject merely quoted in a colleague's held thread
		// matches it — and the export would then hand over that whole thread.
		// The row is still LISTED, because Art. 15 owes the fact that a raw
		// original is held, and its content waits for a release.
		{&pkg.RawCapture, `SELECT rc.source_system, rc.source_id, rc.received_at,
		       NOT EXISTS (SELECT 1 FROM activity a
		                    WHERE a.source_system = rc.source_system
		                      AND a.source_id = rc.source_id
		                      AND a.audience <> 'workspace') AS content_disclosed,
		       CASE WHEN NOT EXISTS (SELECT 1 FROM activity a
		                              WHERE a.source_system = rc.source_system
		                                AND a.source_id = rc.source_id
		                                AND a.audience <> 'workspace')
		            THEN rc.payload END AS payload
		   FROM raw_capture rc
		   WHERE EXISTS (SELECT 1 FROM person_email pe WHERE pe.person_id = $1
		                 AND rc.payload::text ILIKE
		                     '%' || replace(replace(replace(pe.email, '\', '\\'), '%', '\%'), '_', '\_') || '%' ESCAPE '\')
		      OR EXISTS (SELECT 1 FROM person_channel_identity pci WHERE pci.person_id = $1
		                 AND rc.source_system = pci.provider
		                 AND (rc.payload->'message'->'from'->>'id' = pci.channel_user_id
		                      OR rc.payload->'my_chat_member'->'chat'->>'id' = pci.channel_user_id))`, nil},
		{&pkg.FieldOrigins, `SELECT fp.field_name, fp.source, fp.captured_by, fp.captured_at, fp.confidence, fp.evidence_ref
		   FROM field_provenance fp
		   WHERE fp.object_type = 'person' AND fp.object_id = $1`, nil},
		// The STORED value, not the one the 360 renders. person360's
		// readProfileFields overlays whatever verdict a human recorded, because
		// a page showing the machine's claim as fact would be showing a claim
		// its reader already overrode. An export is the other obligation: it
		// owes the subject what this installation HOLDS, and it holds both the
		// enriched value and the correction — which travels beside it in
		// Corrections below, as its own section. Merging them here would hand
		// the subject one value and conceal that the override exists at all.
		{&pkg.EnrichedFields, `SELECT ppf.field, ppf.value, ppf.evidence_snippet, ppf.source_ref,
		          ppf.confidence, ppf.source, ppf.captured_by, ppf.updated_at
		   FROM person_profile_field ppf
		   WHERE ppf.person_id = $1`, nil},
		// claim_key is exported as it stands: it is a hash of the claim's
		// path, so it names WHICH claim was decided without carrying the
		// asserted value, which the ledger never stores in the first place.
		{&pkg.Corrections, `SELECT af.claim_kind, af.claim_key, af.verdict, af.corrected_value, af.note,
		          af.captured_by, af.created_at, af.updated_at
		   FROM ai_feedback af
		   WHERE af.subject_type = 'person' AND af.subject_id = $1`, nil},
		// validation_status is deliberately absent: no writer populates that
		// column, and the per-value validation the provider reports lives
		// INSIDE value_json, which this exports whole. A column exported as
		// null on every row would tell the subject their address was never
		// validated, which is not what the platform knows.
		{&pkg.ProviderClaims, `SELECT ppc.provider, ppc.claim_key, ppc.value_json, ppc.confidence,
		          ppc.source, ppc.captured_by, ppc.retrieved_at
		   FROM person_provider_claim ppc
		   WHERE ppc.person_id = $1`, nil},
		// The run history carries no credential and no vault reference: the
		// closed safe status code is a product reason, never a provider body.
		{&pkg.ProviderRuns, `SELECT pr.provider, pr.trigger, pr.state, pr.skip_reason,
		          pr.requested_categories, pr.claims_unwritten, pr.last_safe_status_code,
		          pr.submitted_at, pr.completed_at, pr.created_at
		   FROM provider_run pr
		   WHERE pr.person_id = $1`, nil},
	}
}
