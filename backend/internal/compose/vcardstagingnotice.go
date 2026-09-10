// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Turning a mailed card's near-match verdict into a proposal, and telling the
// importer when that fails — split out of vcardingest.go, which this shares a
// worker with, once the failure-notice half grew past a comment's worth.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// errNoMailboxGrantorBound names the one impossible case a staging-failure
// notice refuses to guess at: stageReviews always runs inside
// asMailboxGrantor's own context, so an actor that does not resolve means the
// context this ran under was not the one importCards built.
var errNoMailboxGrantorBound = errors.New("compose: a mailed card's staging failure has no importer to notify")

// noticeKindVCardStagingFailed labels the Worklist notice a mailed card's
// staging failure raises.
const noticeKindVCardStagingFailed = "vcard_staging_failed"

// stageReviews turns every near-match into a proposal a human can decide.
//
// ImportVCards reports a card that RESEMBLES somebody rather than merging it,
// and that verdict is only worth having if the question outlives the import. The
// browser upload stages each one; without this the mailed path would log the
// same verdict and drop it, so a contact who posted their card is never created
// and nothing ever reaches a queue.
//
// The stager is self-only and takes the ACTING principal as the proposal's
// subject, which here is the mailbox's granting human. That is the right
// reviewer: the card arrived in their mailbox.
func (w *vcardIngestWorker) stageReviews(ctx context.Context, activity ids.UUID, entries []people.VCardEntry, results []people.VCardResult) error {
	return stageReviewsWith(ctx, w.log, activity, vcardCreateStager(w.pool), w.recordStagingFailure, entries, results)
}

// recordStagingFailure is stageReviews' failure-notice port: the same
// importer a successful proposal would have been staged for gets a durable
// Worklist notice instead — notices is the product's existing transport for
// exactly this ("a line addressed to one person that a system flow needed
// them to see", notices/doc.go), the same one the lead-SLA escalation and
// automation's notify action already use. Reusing it rather than inventing a
// second channel is what makes this reach a screen a person actually opens:
// an audit row alone would not — /records/{entity_type}/{id}/history admits
// no `user` entity type, so nothing renders it anywhere.
//
// The actor is read from ctx rather than threaded as a parameter because
// stageReviewsWith already carries the actor context asMailboxGrantor built;
// a second copy handed down beside it is a second place for the two to drift,
// and reading it here rather than trusting a caller-supplied recipient is
// what keeps this self-only by construction rather than by a caller's promise.
func (w *vcardIngestWorker) recordStagingFailure(ctx context.Context, activity ids.UUID, card int) error {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID == ids.Nil {
		return errNoMailboxGrantorBound
	}
	_, err := notices.NewStore(InstallationDB(w.pool)).Create(ctx, notices.NewNotice{
		Recipient: ids.From[ids.UserKind](actor.UserID),
		Kind:      noticeKindVCardStagingFailed,
		Subject:   "A card attached to your mail could not be reviewed",
		Body: "A card resembling an existing contact was attached to a message in your mailbox, " +
			"and proposing it for your review failed. Reopen the message and re-import the card " +
			"through Import cards on the contact list to retry.",
		// Keyed on the CARD, not just the message: two cards in one batch can
		// each fail, and each is its own open question — a key naming only the
		// activity would let the second failure's notice silently answer the
		// first's dedupe check and never land. River retries the whole message
		// up to vcardIngestMaxAttempts times, and every attempt reaches here
		// again for a card that keeps failing; the key is what keeps that one
		// line, not one line per attempt.
		DedupeKey: fmt.Sprintf("%s:%s:%d", noticeKindVCardStagingFailed, activity, card),
		Target:    notices.Target{Type: string(recordTypeActivity), ID: activity},
	})
	return err
}

// stageReviewsWith is stageReviews against an injected stager and failure
// notice, so a test can make one card's stage fail without needing a real
// staging conflict — what is under test is the aggregation below, not
// vcardCreateStager's own SQL or RecordVCardStagingFailure's own write.
//
// Every eligible card gets its own attempt: one card's staging fault must not
// cost its siblings in the same message their own review, the way ImportVCards
// already holds for the import itself.
//
// A card that failed is still worth a retry — the fault may be the database,
// not the data — so the aggregate error is returned when any card failed, and
// River still retries the whole message against vcardIngestMaxAttempts. It
// wraps every card's error with errors.Join rather than reporting only the
// count: Work classifies this error by errors.Is (ErrPermissionDenied,
// ErrNotFound mean "not a fault, do not retry"), and a plain count would make
// every staging failure look like a fault regardless of what actually failed.
//
// recordFailure runs for every card that failed, retry or no: the browser
// path's own equivalent (the response's per-card Reason) is not something a
// retry might still deliver, and neither is this — a person checking now
// deserves the same answer a person checking after the fifth attempt gets.
// Its own DedupeKey is what keeps a retried attempt from raising the same
// notice again. Its own failure is logged and does not join the aggregate: a
// notice that could not be written is a reason to keep retrying the notice,
// not a reason to tell River the CARD's staging failed differently than it
// did.
func stageReviewsWith(
	ctx context.Context, log *slog.Logger, activity ids.UUID,
	stage func(ctx context.Context, entry people.VCardEntry, candidate *ids.PersonID) error,
	recordFailure func(ctx context.Context, activity ids.UUID, card int) error,
	entries []people.VCardEntry, results []people.VCardResult,
) error {
	var eligible int
	var failures []error
	for _, r := range results {
		// The index is ImportVCards' own position in the slice it was handed, so
		// the bound is a belt on a contract that already holds — but a panic in
		// an unattended writer is worth one comparison.
		if r.Outcome != people.VCardNeedsReview || r.Index < 0 || r.Index >= len(entries) {
			continue
		}
		eligible++
		if err := stage(ctx, entries[r.Index], r.PersonID); err != nil {
			log.ErrorContext(ctx, "a card attached to captured mail could not be staged for review",
				"activity", activity, "card", r.Index+1, "err", err)
			failures = append(failures, err)
			if noteErr := recordFailure(ctx, activity, r.Index+1); noteErr != nil {
				log.ErrorContext(ctx, "a mailed card's staging failure could not raise a notice for its importer",
					"activity", activity, "card", r.Index+1, "err", noteErr)
			}
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("compose: staging %d of %d mailed cards for review: %w",
			len(failures), eligible, errors.Join(failures...))
	}
	return nil
}
