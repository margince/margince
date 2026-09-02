// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// The one writer of person_profile_field, and the precedence rule it carries.
//
// Several passes fill this table — a mail signature, a public search result, a
// site read, a human's acceptance of a research claim — and a conflict clause
// chosen per pass makes the value a field holds depend on which pass ran last.
//
// The rule is not "first wins" or "last wins". It is about WHO is writing:
//
//   - A DERIVED fill claims an unanswered field and never replaces one. A search
//     snippet or a scraped page is somebody else's description of this person,
//     and it does not outrank an answer already on the record.
//   - A HUMAN'S ACCEPTANCE replaces what is there. Somebody read the claim, the
//     quote behind it and the document it came from, and chose it — that is the
//     one input to this table that has weighed the evidence, and a row it could
//     not replace would leave the reader looking at a value they had just
//     rejected (ADR-0096 D4).
//   - A DATED STATEMENT BY THE PERSON replaces what is older than it. A mail
//     signature and a business card are the person saying so themselves, on a
//     date; a contact who changed jobs last month is describing the record as
//     stale, and a fill that deferred to the older answer would keep a number
//     that no longer rings. What it replaced is kept on the row.
//
// So the argument below is the writer's authority, not the SQL verb.
//
// Recency is measured by OBSERVED date and never by write order. These passes
// read mail, which arrives late and is re-delivered, so a row written today may
// carry a two-year-old statement; comparing write times would let it win.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// personProfileFieldPrecedence is who is writing, and therefore what happens to a
// row that is already there.
type personProfileFieldPrecedence int

const (
	// claimUnanswered is what a machine fill writes under: the evidence row is
	// an admission ticket, and one already issued for this field is the answer.
	claimUnanswered personProfileFieldPrecedence = iota
	// replaceOnAcceptance is a human's decision, which outranks whatever a
	// pass put there.
	replaceOnAcceptance
	// supersedeOnNewerObservation is the person's own dated statement, and it
	// replaces a row observed before it.
	supersedeOnNewerObservation
)

// conflictClause renders the precedence as the ON CONFLICT it means.
//
// The whole row is replaced on acceptance, confidence included. Replacing the
// value and keeping a confidence some earlier pass measured would leave the row
// saying a human's answer had been scored by a model that never saw it.
//
// The supersede clause keeps the replaced value in superseded_*, and fires only
// on a STRICTLY newer observation: equal dates leave the row alone, so two
// passes reading one mail cannot take turns overwriting each other. An identical
// value still advances observed_at — the row then says "still true as of now",
// which is what stops a late-arriving OLDER statement from winning afterwards.
func (p personProfileFieldPrecedence) conflictClause() string {
	switch p {
	case replaceOnAcceptance:
		// The undo buffer is CLEARED, not carried. A human choosing this value
		// ends the question of what it replaced: offering the same undo again
		// afterwards would put back a value the record has just been told is
		// wrong, and offering it after a RESTORE would undo the undo.
		return `DO UPDATE SET value = EXCLUDED.value,
		    evidence_snippet = EXCLUDED.evidence_snippet,
		    source_ref = EXCLUDED.source_ref,
		    confidence = EXCLUDED.confidence,
		    source = EXCLUDED.source,
		    captured_by = EXCLUDED.captured_by,
		    observed_at = EXCLUDED.observed_at,
		    superseded_value = NULL,
		    superseded_captured_by = NULL,
		    superseded_observed_at = NULL`
	case supersedeOnNewerObservation:
		return `DO UPDATE SET value = EXCLUDED.value,
		    evidence_snippet = EXCLUDED.evidence_snippet,
		    source_ref = EXCLUDED.source_ref,
		    confidence = EXCLUDED.confidence,
		    source = EXCLUDED.source,
		    captured_by = EXCLUDED.captured_by,
		    observed_at = EXCLUDED.observed_at,
		    superseded_value = CASE
		        WHEN person_profile_field.value IS DISTINCT FROM EXCLUDED.value
		        THEN person_profile_field.value
		        ELSE person_profile_field.superseded_value END,
		    superseded_captured_by = CASE
		        WHEN person_profile_field.value IS DISTINCT FROM EXCLUDED.value
		        THEN person_profile_field.captured_by
		        ELSE person_profile_field.superseded_captured_by END,
		    superseded_observed_at = CASE
		        WHEN person_profile_field.value IS DISTINCT FROM EXCLUDED.value
		        THEN person_profile_field.observed_at
		        ELSE person_profile_field.superseded_observed_at END
		  WHERE EXCLUDED.observed_at > person_profile_field.observed_at`
	case claimUnanswered:
	}
	return "DO NOTHING"
}

// personProfileFieldRow is one evidence row: a fact about who this person is, and
// what it was read from.
//
// Evidence and SourceRef are both NOT NULL on the table, so a claim that lost
// its quote or its source cannot be stored at all — that is what makes the
// evidence guarantee enforceable rather than promised, and no caller here may
// weaken it.
//
// Confidence is a pointer because most passes have none: a human's acceptance
// and a page read are not scored, and storing a 0 for "unscored" would read as
// "measured, and certainly wrong".
//
// ObservedAt is when the SOURCE stated the value — the mail's own date, the
// moment a card arrived — and not when this pass read it. Nil means "now",
// which is the honest answer for a page read or a human's acceptance: those
// state the value at the moment they are performed. The statement below
// defaults it in SQL so the transaction's own clock decides, rather than a
// wall-clock read taken somewhere up the call stack.
type personProfileFieldRow struct {
	Field           string
	Value           string
	EvidenceSnippet string
	SourceRef       string
	Source          string
	CapturedBy      string
	Confidence      *float64
	ObservedAt      *time.Time
	// Superseded is the value this row replaces when it is not one of this
	// table's own — a title a human typed straight onto the person, which
	// leaves no row here to read the old value from. Empty otherwise, and the
	// conflict clause then keeps what the row itself carried.
	Superseded string
}

// writePersonProfileField writes one evidence row and reports whether it landed.
//
// false carries two meanings and the callers separate them: the field was
// already answered and this writer defers to it, OR the subject went while this
// transaction was deciding. The second is the rarer one and the lock below is
// what makes it reliable; a caller that must tell them apart re-reads, and
// researchclaim.go says why it turns either into a refusal.
//
// The subject is HELD before the row lands, not merely probed by the entry
// point. person_profile_field is a declared PII table and Art. 17 erasure clears
// its rows while stamping the person archived, so a fill decided before that
// commit and applied after it would put the erased subject's details straight
// back — in the window between two statements, not over the hours an entry gate
// closes.
//
// Through auth.LockSubjectLive. This used to carry its own `SELECT … FOR UPDATE`
// inside the INSERT and was the only statement in the tree that did, which is
// one writer of the invariant too many.
//
// Locking PERSON and not the field row: the field row may not exist yet, and
// what has to be serialized is the subject's liveness. Every caller that goes
// on to write a person COLUMN takes the same row next, so the order is
// person-then-person and no new deadlock edge is introduced.
func writePersonProfileField(ctx context.Context, tx pgx.Tx, personID ids.PersonID, row personProfileFieldRow, precedence personProfileFieldPrecedence) (bool, error) {
	if err := auth.LockSubjectLive(ctx, tx, "person", personID.UUID); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			// Not an error to THIS writer, whose whole contract is to report
			// whether the row landed. "The subject went" is one more reason it
			// did not, and the callers refuse an archived subject at their own
			// door — reaching here means it went inside the window, where
			// landed=false is the honest answer rather than a swallowed
			// refusal.
			return false, nil
		}
		return false, err
	}
	tag, err := tx.Exec(ctx, `
		INSERT INTO person_profile_field
		  (person_id, field, value, evidence_snippet, source_ref, confidence, source, captured_by,
		   observed_at, superseded_value)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, COALESCE($9, now()), NULLIF($10, ''))
		ON CONFLICT (person_id, field) `+precedence.conflictClause(),
		personID, row.Field, row.Value, row.EvidenceSnippet, row.SourceRef,
		row.Confidence, row.Source, row.CapturedBy, row.ObservedAt, row.Superseded)
	if err != nil {
		return false, fmt.Errorf("people: profile field evidence row (%s): %w", row.Field, err)
	}
	landed := tag.RowsAffected() > 0
	if landed && row.Field == fieldLinkedin {
		if err := fillEmptyLinkedinSlot(ctx, tx, personID, row.Value); err != nil {
			return false, err
		}
	}
	return landed, nil
}

// fillEmptyLinkedinSlot carries a landed linkedin evidence row into the social
// slot the rest of the product reads. person_social is where the person rail,
// the provider identifier resolver and the SAR export look for a LinkedIn
// handle — an evidence row alone leaves every one of them answering "none"
// while the page's own research section displays the URL, and the provider
// lookup then refuses the contact as having nothing to match on.
//
// Additive only, through the same writer the LinkedIn-match apply and the
// bought-claim apply use: a handle already on the record is somebody's
// statement and a machine fill is not grounds to replace it. A value that is
// not a LinkedIn URL stays evidence-only — the social slot's contract is a
// profile link, and the same host check gates the vCard import's slot.
func fillEmptyLinkedinSlot(ctx context.Context, tx pgx.Tx, personID ids.PersonID, value string) error {
	if !isLinkedinURL(value) {
		return nil
	}
	// The row probe, restated here rather than inherited: every caller funnels
	// through writePersonProfileField, whose entry points each probe the row,
	// but this write reaches person_social and the person row itself, and the
	// slot fill must not become writable from a future caller that forgot. A
	// subject the principal cannot write is a skipped fill, not a fault — the
	// evidence row landed under the caller's own gate.
	if err := auth.HoldWritableLive(ctx, tx, "person", personID.UUID); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return nil
		}
		return err
	}
	_, landed, err := insertSocialHandle(ctx, tx, personID.UUID, socialLinkedIn, value)
	if err != nil || !landed {
		return err
	}
	// The slot is part of the person aggregate: the bump invalidates If-Match
	// tokens held by browsers that never saw it, for touchPerson's reason.
	return touchPerson(ctx, tx, personID.UUID)
}
