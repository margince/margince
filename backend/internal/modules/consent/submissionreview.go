// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The queue where somebody decides what a subject proposed.
//
// contact_confirm_submission has carried resolution, resolved_at and
// resolved_by since it shipped and nothing ever wrote them. A subject could
// type a correction into the link we mailed them, the row was filed, and no
// colleague could list it, see it, or act on it — so every correction anybody
// has ever sent is still sitting there unanswered.
//
// NOTHING HERE TOUCHES THE CRM BY ITSELF. stageSubmission says why the proposal
// is only evidence: the subject holds a bearer token and sits outside every
// row-scope probe. Accepting one is a colleague's write, made through the
// ordinary update path under their own authority, which is what keeps a
// correction answerable to the same gates as any other edit.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The decisions a reviewer may record.
const (
	// SubmissionAccepted says the subject was right and the record now says so.
	SubmissionAccepted = "accepted"
	// SubmissionRejected says somebody looked and did not change it.
	//
	// The table's own CHECK fixes this spelling, and it is the same word
	// data_subject_request uses for the same act. "declined" read better and
	// would have been a second vocabulary for one thing.
	SubmissionRejected = "rejected"
)

// The audit payload's field names, shared with the write that staged the
// proposal (confirmsubmit.go). A reader filtering the trail on
// "confirm_submission" wants both halves of the story: the arrival and the
// answer.
const (
	auditFieldSubmissionKind = "confirm_submission"
	auditFieldSubmissionID   = "submission_id"
)

// maxSubmissionNoteRunes bounds the reviewer's note, the same 500 every other
// operator note in this schema carries. The column's own CHECK holds it too:
// the bound outlives whichever door writes it.
const maxSubmissionNoteRunes = 500

// Submission is one proposal and what became of it.
type Submission struct {
	ID            ids.UUID
	ContactID     ids.ContactID
	Kind          string
	Field         *string
	ProposedValue *string
	SubmittedAt   time.Time
	Resolution    *string
	ResolvedAt    *time.Time
	ResolvedBy    *string
	Note          *string
	// ContactName is who proposed it. A queue spanning every contact is
	// unusable without it: two contacts proposing the same title on the same
	// day are indistinguishable, and accepting either changes a different
	// record.
	ContactName string
	// CurrentValue is what the record holds for that field right now. A
	// correction is only reviewable as a COMPARISON, and this is the half the
	// reviewer cannot get from the proposal.
	CurrentValue string
}

// submissionColumns is the one whole-row SELECT list.
//
// Held by: TestOneSelectListSpellsTheSubmissionRow
// (backend/gates/submissioncolumns_test.go)
const submissionColumns = `id, contact_id, kind, field, proposed_value, submitted_at,
	resolution, resolved_at, resolved_by, note`

// currentValueSQL answers what the record holds for the field a correction
// names, so the reviewer sees both halves of the comparison.
//
// The same readers the confirm page shows the subject (confirmcard.go), which
// is the point: the two must agree about what is held, or the subject and the
// reviewer are looking at different records. CASE over the closed set of
// correctable fields — an unrecognised field answers empty rather than
// something plausible.
func currentValueSQL(alias string) string {
	return `CASE s.field
		WHEN '` + ConfirmFieldFullName + `' THEN coalesce(` + alias + `.full_name, '')
		WHEN '` + ConfirmFieldTitle + `' THEN coalesce(` + alias + `.title, '')
		WHEN '` + ConfirmFieldEmail + `' THEN ` + primaryEmailSQL(alias+".id") + `
		WHEN '` + ConfirmFieldPhone + `' THEN coalesce((SELECT pp.phone FROM contact_phone pp
		                  WHERE pp.contact_id = ` + alias + `.id AND pp.archived_at IS NULL
		                  ORDER BY pp.is_primary DESC, pp.created_at LIMIT 1), '')
		ELSE '' END`
}

// ListSubmissionsInput narrows the queue.
type ListSubmissionsInput struct {
	// ContactID lists one contact's proposals. Zero lists every contact's.
	ContactID ids.ContactID
	// Resolved filters on whether somebody has decided. Nil returns both.
	Resolved *bool
	Limit    int
}

// ListSubmissions answers what subjects have sent and what became of it.
//
// UNRESOLVED FIRST, oldest first within that. A correction somebody sent three
// weeks ago is the one still waiting, and a queue that sorted newest-first
// would bury it under everything that arrived since.
func (s *Store) ListSubmissions(ctx context.Context, in ListSubmissionsInput) ([]Submission, error) {
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return nil, err
	}
	limit := in.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var out []Submission
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		// THE ROW SCOPE of the contact each submission is about, so a reviewer
		// sees proposals only for contacts they could open. A submission holds
		// the subject's own words about themselves, which is no less protected
		// for having been typed by them.
		var args []any
		arg := func(v any) int { args = append(args, v); return len(args) }
		scope, err := auth.ScopeClauseFor(ctx, "contact", "c", arg)
		if err != nil {
			return err
		}
		where := "TRUE"
		if scope != "" {
			where = scope
		}
		if !in.ContactID.IsZero() {
			where += fmt.Sprintf(" AND s.contact_id = $%d", arg(in.ContactID.UUID))
		}
		if in.Resolved != nil {
			if *in.Resolved {
				where += " AND s.resolution IS NOT NULL"
			} else {
				where += " AND s.resolution IS NULL"
			}
		}
		rows, err := tx.Query(ctx, `
			SELECT `+prefixed(submissionColumns, "s")+`,
			       coalesce(c.full_name, ''), `+currentValueSQL("c")+`
			  FROM contact_confirm_submission s
			  JOIN contact c ON c.id = s.contact_id
			 WHERE `+where+`
			 ORDER BY s.resolution IS NOT NULL, s.submitted_at
			 LIMIT `+fmt.Sprintf("$%d", arg(limit)), args...)
		if err != nil {
			return fmt.Errorf("consent: listing what subjects proposed: %w", err)
		}
		out, err = pgx.CollectRows(rows, scanListedSubmission)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// scanSubmission reads one row in submissionColumns order.
func scanSubmission(row pgx.CollectableRow) (Submission, error) {
	var sub Submission
	err := row.Scan(&sub.ID, &sub.ContactID, &sub.Kind, &sub.Field, &sub.ProposedValue,
		&sub.SubmittedAt, &sub.Resolution, &sub.ResolvedAt, &sub.ResolvedBy, &sub.Note)
	return sub, err
}

// scanListedSubmission reads a queue row: the submission plus the two halves of
// the comparison the reviewer needs.
func scanListedSubmission(row pgx.CollectableRow) (Submission, error) {
	var sub Submission
	err := row.Scan(&sub.ID, &sub.ContactID, &sub.Kind, &sub.Field, &sub.ProposedValue,
		&sub.SubmittedAt, &sub.Resolution, &sub.ResolvedAt, &sub.ResolvedBy, &sub.Note,
		&sub.ContactName, &sub.CurrentValue)
	return sub, err
}

// prefixed qualifies a column list for a joined query.
func prefixed(columns, alias string) string {
	parts := strings.Split(columns, ",")
	for i, p := range parts {
		parts[i] = alias + "." + strings.TrimSpace(p)
	}
	return strings.Join(parts, ", ")
}

// CorrectionApplier writes an accepted correction onto the contact record.
//
// Declared here by the consumer and implemented by compose: consent may not
// import contacts, and an accepted correction must reach the record through the
// SAME update path every other edit uses. A write of its own would be a second
// way to change a contact, answerable to none of the gates the first one is —
// no row scope, no audit trail, no event.
//
// A nil applier records decisions and applies nothing, which is what every test
// store does.
type CorrectionApplier interface {
	// ApplyCorrection writes one field. It answers ErrUnsupportedField for a
	// field the ordinary update path does not take, which the caller reports
	// rather than swallowing.
	ApplyCorrection(ctx context.Context, contactID ids.ContactID, field, value string) error
}

// ErrUnsupportedField says the accepted correction names a field this product
// cannot write through the ordinary update path.
//
// EMAIL AND PHONE ARE THE CASE, and the reason is not an oversight. The confirm
// page offers four correctable fields; two of them live on satellite tables
// with their own primary-address rules, verification state and uniqueness, and
// the writers that own them are not one call. Accepting such a correction
// through this door would route around all of it.
//
// So the decision is recorded, the reviewer is told the record did not move,
// and the change is theirs to make on the contact. The alternative — a silent
// accept that changed nothing — is the one outcome nobody could act on.
var ErrUnsupportedField = errors.New("consent: that field is not writable through a correction")

// WithCorrectionApplier wires the edge that writes an accepted correction.
func (s *Store) WithCorrectionApplier(a CorrectionApplier) *Store {
	s.corrections = a
	return s
}

// ResolveSubmissionInput is one reviewer's decision.
type ResolveSubmissionInput struct {
	Resolution string
	Note       string
}

// ResolveSubmission records what a reviewer decided, and for an accepted
// correction writes the value onto the record.
//
// IDEMPOTENT on a submission already resolved: the stored decision comes back
// unchanged rather than being overwritten. A second reviewer pressing a stale
// screen is not a second decision, and the first one's name is the one that
// belongs on the row.
//
// TWO TRANSACTIONS, AND THAT IS NOT FIXABLE HERE. The applier goes through
// contacts' ordinary update path, which opens its own — deliberately, so the
// write is governed by the same gates as any other edit — and consent may not
// reach inside another module's store to borrow this one. So the decision and
// the change it causes cannot commit together, and one of two inconsistencies
// is reachable.
//
// THE APPLY RUNS FIRST, which chooses the recoverable one. A failed stamp
// leaves the field changed and the proposal still in the queue: the next
// reviewer sees the proposal beside a current value that already matches it,
// and accepting it again is a no-op that closes the row. The reverse order
// leaves a row reading "accepted" whose value never reached the contact — the
// subject is told their correction was taken, the record disagrees, and
// nothing in the queue shows it, because a resolved row has left.
//
// The comparison the queue shows is what makes the first case visible, which
// is the other reason it carries the current value.
func (s *Store) ResolveSubmission(
	ctx context.Context, id ids.UUID, in ResolveSubmissionInput,
) (Submission, error) {
	if in.Resolution != SubmissionAccepted && in.Resolution != SubmissionRejected {
		return Submission{}, &ValidationError{
			Field:  "resolution",
			Reason: "a decision is either accepted or rejected",
		}
	}
	if n := len([]rune(in.Note)); n > maxSubmissionNoteRunes {
		return Submission{}, &ValidationError{
			Field:  "note",
			Reason: fmt.Sprintf("a note is at most %d characters; this one is %d", maxSubmissionNoteRunes, n),
		}
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return Submission{}, err
	}

	var out Submission
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		current, err := lockSubmission(ctx, tx, id)
		if err != nil {
			return err
		}
		// TWO CHECKS, and the second is not implied by the first.
		//
		// EnsureWritable answers whether this caller can REACH that row — row
		// scope and capture privacy. It does not ask whether they hold the
		// update verb: ensureWriteAuthority returns early for a reader whose
		// role simply lacks it. Omitting the Require was a real hole, caught by
		// TestAReviewerWhoCannotEditTheContactDecidesNothing, and it is the
		// same shape as every other writer in this package, which pairs them.
		//
		// AGAINST THE CONTACT, not the submission. Deciding what a proposal
		// does to a record is a write to that record — even a rejection, which
		// is the workspace's recorded answer about somebody.
		if err := auth.Require(ctx, "contact", principal.ActionUpdate); err != nil {
			return err
		}
		if err := auth.EnsureWritable(ctx, tx, "contact", current.ContactID.UUID); err != nil {
			return err
		}
		if current.Resolution != nil {
			out = current
			return nil
		}
		if err := s.applyAcceptedTx(ctx, current, in); err != nil {
			return err
		}
		out, err = markResolvedTx(ctx, tx, id, in, by)
		return err
	})
	if err != nil {
		return Submission{}, err
	}
	return out, nil
}

// lockSubmission reads the proposal and holds it, so two reviewers deciding at
// once queue rather than both writing.
func lockSubmission(ctx context.Context, tx pgx.Tx, id ids.UUID) (Submission, error) {
	rows, err := tx.Query(ctx,
		`SELECT `+submissionColumns+` FROM contact_confirm_submission WHERE id = $1 FOR UPDATE`, id)
	if err != nil {
		return Submission{}, fmt.Errorf("consent: reading the proposal: %w", err)
	}
	sub, err := pgx.CollectExactlyOneRow(rows, scanSubmission)
	if errors.Is(err, pgx.ErrNoRows) {
		return Submission{}, apperrors.ErrNotFound
	}
	if err != nil {
		return Submission{}, fmt.Errorf("consent: reading the proposal: %w", err)
	}
	return sub, nil
}

// applyAcceptedTx writes an accepted correction onto the record.
//
// A REMOVAL REQUEST APPLIES NOTHING here. It is not a correction to a field:
// what the subject asked for is a rights case, which stageSubmission already
// opened when the proposal arrived, and answering it is that case's business.
// Accepting the submission records that somebody read the request.
func (s *Store) applyAcceptedTx(ctx context.Context, current Submission, in ResolveSubmissionInput) error {
	if in.Resolution != SubmissionAccepted || current.Kind != submissionCorrection {
		return nil
	}
	if current.Field == nil || current.ProposedValue == nil {
		return &ValidationError{
			Field:  "resolution",
			Reason: "this correction names no field or no value, so there is nothing to accept",
		}
	}
	if s.corrections == nil {
		return ErrUnsupportedField
	}
	// ON THE CALLER'S OWN CONTEXT, not the transaction: the applier goes
	// through contacts' ordinary update path, which opens its own transaction
	// and makes its own row-scope probe under this reviewer's authority. Handing
	// it this transaction would let a module write inside another's, which the
	// ownership gate refuses for exactly the reasons that make it worth doing
	// this way.
	return s.corrections.ApplyCorrection(ctx, current.ContactID, *current.Field, *current.ProposedValue)
}

// markResolvedTx stamps the decision and answers the row it wrote.
func markResolvedTx(
	ctx context.Context, tx pgx.Tx, id ids.UUID, in ResolveSubmissionInput, by string,
) (Submission, error) {
	var note *string
	if trimmed := strings.TrimSpace(in.Note); trimmed != "" {
		note = &trimmed
	}
	rows, err := tx.Query(ctx, `
		UPDATE contact_confirm_submission
		   SET resolution = $2, resolved_at = now(), resolved_by = $3, note = $4
		 WHERE id = $1
		RETURNING `+submissionColumns, id, in.Resolution, by, note)
	if err != nil {
		return Submission{}, fmt.Errorf("consent: recording the decision: %w", err)
	}
	out, err := pgx.CollectExactlyOneRow(rows, scanSubmission)
	if err != nil {
		return Submission{}, fmt.Errorf("consent: recording the decision: %w", err)
	}
	// Audited against the CONTACT, as the submission itself was: a later reader
	// asks "what did we do about what this contact sent us". The proposed value
	// is deliberately absent for the reason stageSubmission gives — it is the
	// subject's own data, it sits on the row, and a copy in the audit trail
	// would outlive the erasure that clears the first.
	if _, err := storekit.AuditEvent(ctx, tx, "update", "contact", out.ContactID.UUID, map[string]any{
		auditFieldSubmissionKind: out.Kind,
		auditFieldSubmissionID:   id,
		"submission_decision":    in.Resolution,
	}); err != nil {
		return Submission{}, err
	}
	return out, nil
}
