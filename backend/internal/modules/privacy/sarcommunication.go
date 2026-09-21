// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The outbound half of an Art. 15 export: why each message to this subject was
// permitted, what stood behind it, what they asked to stop, and the times a
// named contact decided a message went out despite a refusal.
//
// Its own file because it is the half that answers "what did you do with my
// data and why" — every other section says what is HELD, and these say what was
// DONE.

import "github.com/margince/margince/backend/internal/shared/kernel/ids"

// sarCommunicationSections gather the outbound record: why each message was
// permitted, the non-consent basis behind it, and what the subject asked to
// stop.
//
// This is the part of the package that answers "what did you do with my data
// and why" for every message actually sent. The decision rows carry ids and a
// verdict, never the message: the content fingerprint is deliberately not read
// back, because it identifies a body the export already discloses elsewhere and
// hashing it again tells the subject nothing.
// The lead arm is not optional here. A subject captured as a lead and promoted
// later has decisions, bases and suppressions carrying the LEAD id — the lead
// row survives an erasure as an anonymized shell, so those rows are still the
// subject's own history. A contact-keyed section would silently withhold the
// earliest part of their record, which is the half they are least likely to
// know about and most likely to be asking after.
func sarCommunicationSections(
	pkg *SARPackage, emails []string, leads, identities []ids.UUID,
) []sarSection {
	return []sarSection{
		// EVERY IDENTITY here too, for the reason the bases and suppressions
		// below take it: a decision taken about a record that was later merged
		// into this subject is a decision about this subject, and reading the
		// survivor alone leaves the export contradicting itself — it would
		// carry a predecessor's suppression while withholding the decisions
		// that suppression produced.
		{
			&pkg.CommunicationDecisions, `SELECT phase, requested_category, resolved_category, verdict,
		          reason_code, basis, suppression, mode, decided_at
		   FROM communication_decision
		   WHERE (subject_kind IS DISTINCT FROM 'lead' AND subject_id = ANY($1))
		      OR (subject_kind = 'lead' AND subject_id = ANY($2))`,
			[]any{identities, leads},
		},
		// EVERY IDENTITY, not the surviving row alone. A merge keeps the
		// retiring subject's own basis and suppression rows where they are —
		// the predecessor's objection is evidence that THAT record's subject
		// refused — so an export reading only the survivor shows the copy the
		// merge carried and never the act behind it.
		//
		// identities ALREADY CONTAINS the survivor, so these two take it in
		// place of the bare contact id rather than beside it: a parameter a
		// statement never references is one Postgres cannot infer a type for,
		// and it refuses to prepare the statement at all.
		{
			&pkg.CommunicationBases, `SELECT kind, thread_key, valid_from, valid_until, note,
		          captured_at, revoked_at
		   FROM communication_basis
		   WHERE contact_id = ANY($1) OR lead_id = ANY($2)`,
			[]any{identities, leads},
		},
		{
			// BY ADDRESS AS WELL, because a machine-written stop names no
			// subject. The hard bounce consent/bouncesuppress.go records
			// deliberately carries no contact_id — a contact-scoped row would
			// refuse every address that record has — so a query keyed on the
			// subject's ids alone tells them nothing about an address of theirs
			// this installation has stopped writing to.
			// THE PURPOSE BY ITS NAME, not by its id. Every other column
			// here is something the subject can read, and the reason this
			// section names the purpose at all is so somebody holding a stop
			// on one list can tell it from an objection to everything — which
			// a bare consent_purpose uuid does not tell them. LEFT JOIN, so a
			// broad stop (purpose_id NULL) keeps its row and answers NULL for
			// both columns rather than dropping out of the export.
			&pkg.CommunicationSuppression, `SELECT s.kind, s.source, s.address, s.recorded_at,
		          s.revoked_at, s.decided_by_level, p.key AS purpose_key, p.label AS purpose_label
		   FROM communication_suppression s
		   LEFT JOIN consent_purpose p ON p.id = s.purpose_id
		   WHERE s.contact_id = ANY($1) OR s.lead_id = ANY($2)
		      OR lower(s.address) = ANY($3)`,
			[]any{identities, leads, lowerAll(emails)},
		},
		// A rep vouching that a machine refusal may be overruled for this
		// subject. No address arm, unlike the suppression above: an override
		// carries no address column, so it is reached by the subject's own ids
		// alone.
		{
			&pkg.CommunicationOverrides, `SELECT category, reason, decided_by_level, recorded_at, revoked_at
		   FROM communication_override
		   WHERE contact_id = ANY($1) OR lead_id = ANY($2)`,
			[]any{identities, leads},
		},
		// REACHED THROUGH THE REVIEW, because an instruction names no subject
		// directly: it answers a refusal, and the refusal is what named the
		// recipients. The explanation is exported as stored, which for an
		// erased subject is the tombstone their erasure wrote.
		{
			&pkg.CommunicationExceptions, `SELECT i.reason_code, i.explanation, i.status,
		          i.directed_at, i.revoked_at
		   FROM communication_instruction i
		   JOIN communication_review r ON r.id = i.review_id
		  WHERE EXISTS (
		          SELECT 1 FROM jsonb_array_elements(r.refusals) AS refusal
		           WHERE refusal->>'subject_id' = ANY($1::text[]))`,
			[]any{identityStrings(identities)},
		},
	}
}

// identityStrings renders the subject's ids for a jsonb text comparison, which
// is what a refusal stores them as.
func identityStrings(identities []ids.UUID) []string {
	out := make([]string, 0, len(identities))
	for _, id := range identities {
		out = append(out, id.String())
	}
	return out
}
