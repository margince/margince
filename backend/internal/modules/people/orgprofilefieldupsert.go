// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// The one statement that records what a company field says and who said it.
//
// Two writers land these rows — a person filling the company form, and an
// accepted read-back — and they wrote the same upsert twice, once with every
// value bound and once with a human's four values inlined as literals. The
// second was the first with `”`, `”`, `1`, `'human'` and an unconditional
// WHERE, which is a set of ARGUMENTS rather than a second statement.
//
// The precedence rule is the WHERE, and it is the reason this is worth having
// once: a machine's re-accept refreshes a row it captured and never touches one
// a person has since claimed. A human write passes `true` because a person's
// answer always outranks what is there — including their own earlier one.
//
// The values arrive through a SELECT over the organization rather than a VALUES
// list, and that is the LIVE test: an archived company yields no row, so neither
// arm runs — nothing is inserted, so there is no conflict for the update arm to
// take either. A profile field is what the company's own columns are evidenced
// by, and the columns refuse an archived record (orgColumnWrites); evidence
// landing on one the column write skipped would leave the two disagreeing about
// the same accept.
//
// It reads as a refusal rather than an error, matching the column writes: every
// entry point probes the record live first, so a caller reaching an archived
// company here has been overtaken by the archive mid-apply, and a stale write
// silently declining is the same residue the column write leaves.
const upsertOrgProfileField = `
	INSERT INTO organization_profile_field (organization_id, field, value, evidence_snippet, source_url, confidence, source, captured_by)
	SELECT $1, $2, $3, $4, $5, $6, $7, $8
	  FROM organization o
	 WHERE o.id = $1 AND o.archived_at IS NULL
	ON CONFLICT (organization_id, field)
	DO UPDATE SET value = EXCLUDED.value, evidence_snippet = EXCLUDED.evidence_snippet,
	              source_url = EXCLUDED.source_url, confidence = EXCLUDED.confidence,
	              source = EXCLUDED.source,
	              captured_by = EXCLUDED.captured_by, captured_at = now()
	WHERE $9 OR organization_profile_field.captured_by NOT LIKE 'human:%'`

// humanAuthoredConfidence is what a person's own answer about their own company
// scores. They are not guessing about themselves.
const humanAuthoredConfidence = float32(1)
