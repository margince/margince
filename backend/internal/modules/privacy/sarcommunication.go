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
func sarCommunicationSections(pkg *SARPackage, leads, identities []ids.UUID) []sarSection {
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
			&pkg.CommunicationSuppression, `SELECT kind, source, address, recorded_at, revoked_at,
		          decided_by_level
		   FROM communication_suppression
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
