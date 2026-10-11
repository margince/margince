// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// eraseActivityContent is the activity/erase action. Transcript free-text is
// the special-category risk; the record of the meeting stays, its content goes
// — including any attached recording/transcript file (objects first, so the
// purge shares the contact-erase durability guarantee).
//
// `raw` goes with `body`. It is the re-parseable original the schema names, so
// clearing the parsed copy and leaving the source erases nothing — the content
// is one parse away. Nothing in this tree populates the column, which is why
// only a gate will ever notice if this stops: piicoverage_test.go declares the
// assignments this statement IS.
//
// `counterparty_email` and the channel identity (`source_id`, `thread_key`)
// deliberately stay, and that is where this statement parts company with its
// two siblings. The retention action's contract is that the RECORD of the
// meeting survives and its content goes, and who it was with is the record.
// The difference is declared in piicoverage_test.go's retentionKeeps for
// `activity`, so reversing it fails the gate rather than passing silently — the
// data-layer guard `activity_restriction_lift_erases` exists because the same
// kind of difference was once carried in prose and went short.
func (s *RetentionService) eraseActivityContent(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	if err := s.refuseIfShieldedNow(ctx, tx, id); err != nil {
		return err
	}
	_, err := tx.Exec(ctx,
		// language goes with the text: it was read from the body this statement
		// is emptying, so keeping it would answer one question about content
		// that no longer exists.
		// source_author_name and the reply verdict go with it (erasuretimeline.go).
		`UPDATE activity SET body = NULL, raw = NULL, subject = $2, language = NULL, source_author_name = NULL, reply_verdict = NULL, reply_verdict_at = NULL, reply_verdict_by = NULL, archived_at = coalesce(archived_at, now()) WHERE id = $1`,
		id, erasedActivitySubject)
	if err == nil {
		// Everything the text left behind — the verbatim provider original, the
		// vectors, the provenance of fields that are now gone, the transcript
		// readings, the proposals quoting it, the attachments and the
		// transmitted copy.
		//
		// The SAME call the restriction lift and the controller's release make.
		// This list used to live here and a shorter one lived there, and the
		// shorter one was missing the provider original and the quoting
		// proposals — which is what a second list does, whatever the comment
		// beside it promises.
		err = s.eraser.purgeContentDerivedFrom(ctx, tx, id, theClockRanOut)
	}
	if err == nil {
		// Not in the shared helper, and that is the difference between the two
		// arms rather than an omission from one. This arm removes an
		// interaction the relationship aggregates counted, so it re-folds them
		// in the same transaction; the lift paths emit their own events and the
		// bus consumer that handles them is the backstop there. Folding it in
		// would make the shared helper need a seam two of its three callers do
		// not have.
		err = s.invalidateGraph(ctx, tx, id)
	}
	return err
}
