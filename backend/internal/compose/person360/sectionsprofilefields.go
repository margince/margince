// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package person360

// The enrichment evidence sidecar: the profile fields a pass proposed for this
// contact, the snippet each was read from, and the verdicts a human has since
// recorded over them.
//
// Its own file because it is its own subject. The timeline sections beside it
// answer "what happened with this person"; these answer "what does the product
// claim to know about them, and on whose word".

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// profileFieldsSection is the enrichment evidence sidecar. Evidence-or-omit
// is enforced at write time (the snippet column is NOT NULL), so every row
// here can show the reader the text its value was read from.
func (s *Service) profileFieldsSection(ctx context.Context, tx pgx.Tx, personID ids.PersonID, out *crmcontracts.Person360) error {
	fields, err := s.readProfileFields(ctx, tx, personID)
	if err != nil {
		return err
	}
	out.ProfileFields = &fields
	return nil
}

// readProfileFields is every read of person_profile_field that RENDERS it to a
// reader — the 360 section and the standalone sidecar endpoint both come
// through here.
//
// Held by: TestEveryReaderServingProfileFieldValuesConsultsTheVerdictLedger
// (backend/gates/profilefieldreaders_test.go) — it censuses every statement that
// serves a value from that table and requires each to overlay the verdict, so a
// second render path fails rather than quietly serving the overridden claim.
//
// That matters because the human's verdict is folded in below. A corrected
// value rendered without its marker reads as the machine's assertion, which is
// exactly the claim the human overrode, so consulting the ledger cannot be one
// caller's job: a second read path that skipped it would keep serving the
// rejected value on a surface nobody thought to check.
//
// Other statements touch the table — an existence probe, a merge relink, the
// writers — but exactly one other SERVES values out of it, and it deliberately
// does not come through here: privacy/sar.go's Article 15 export.
//
// That is not a gap. An export owes the subject what this installation HOLDS,
// and it holds two facts: the machine's assertion and the verdict recorded
// against it. So it exports the stored columns and ai_feedback beside them as
// its own section, and the subject sees both. Overlaying the verdict there
// would hand them one merged value and conceal that the override exists — the
// opposite of what an export is for. The two also cannot share this function:
// privacy is a module and may not import compose.
//
// TestEveryReaderServingProfileFieldValuesConsultsTheVerdictLedger holds this
// paragraph, so a third reader that serves values without the overlay fails
// rather than quietly making the sentence above false.
func (s *Service) readProfileFields(ctx context.Context, tx pgx.Tx, personID ids.PersonID) ([]crmcontracts.PersonProfileField, error) {
	rows, err := tx.Query(ctx, `
		-- updated_at, not created_at: this is when the value took its CURRENT
		-- form, which is the date the receipt should show after a human edit.
		SELECT field, value, evidence_snippet, source_ref, confidence, source, captured_by, updated_at,
		       observed_at, superseded_value, superseded_observed_at
		FROM person_profile_field
		WHERE person_id = $1
		ORDER BY field`, personID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]crmcontracts.PersonProfileField, 0, 5)
	for rows.Next() {
		var f crmcontracts.PersonProfileField
		var field string
		if err := rows.Scan(&field, &f.Value, &f.EvidenceSnippet, &f.SourceRef,
			&f.Confidence, &f.Source, &f.CapturedBy, &f.CapturedAt,
			&f.ObservedAt, &f.SupersededValue, &f.SupersededObservedAt); err != nil {
			return nil, err
		}
		f.Field = crmcontracts.PersonProfileFieldField(field)
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return s.applyFieldVerdicts(ctx, tx, personID, out)
}

// applyFieldVerdicts overlays what a human already decided about each field.
func (s *Service) applyFieldVerdicts(
	ctx context.Context,
	tx pgx.Tx,
	personID ids.PersonID,
	fields []crmcontracts.PersonProfileField,
) ([]crmcontracts.PersonProfileField, error) {
	verdicts, err := s.feedback.VerdictsForTx(ctx, tx, "person", personID.UUID)
	if err != nil {
		return nil, err
	}
	for i := range fields {
		f := &fields[i]
		claim := ai.ProfileFieldClaimPath(string(f.Field))
		f.ClaimKey = &claim
		v, found := verdicts[ai.VerdictLookupKey(ai.ClaimProfileField, ai.ClaimKey(claim))]
		if !found {
			continue
		}
		// AGAINST f.CapturedAt, which is this row's updated_at — when the
		// value took its current form. A human's decision is about the answer
		// that was in front of them, and something may have replaced it since:
		// a machine fill cannot (it writes DO NOTHING over a row that exists),
		// but an accepted research claim replaces the whole row and moves this
		// date. A verdict older than that is about a value the record no
		// longer holds, and ai.Verdict.AsOf is where that ruling lives.
		decision, applies := v.AsOf(f.CapturedAt)
		if !applies {
			continue
		}
		verdict := crmcontracts.PersonProfileFieldVerdict(decision.Verdict)
		f.Verdict = &verdict
		f.VerdictNote = decision.Note
		if decision.Verdict == ai.VerdictCorrected && decision.CorrectedValue != nil {
			// The human's value stands. The captured snippet is left in place
			// beneath it on purpose — what the machine read is still the
			// evidence for why it got this wrong, and hiding it would leave the
			// correction unexplainable.
			f.Value = *decision.CorrectedValue
		}
	}
	return fields, nil
}
