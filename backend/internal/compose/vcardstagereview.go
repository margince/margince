// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Turning a mailed card's near-match verdict into a proposal a human can
// decide, and telling the importer when that fails.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/shared/apperrors"
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
func (w *vcardIngestWorker) recordStagingFailure(ctx context.Context, activity ids.UUID, entry people.VCardEntry) error {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID == ids.Nil {
		return errNoMailboxGrantorBound
	}
	_, err := notices.NewStore(InstallationDB(w.pool)).Create(ctx, notices.NewNotice{
		Recipient: ids.From[ids.UserKind](actor.UserID),
		Kind:      noticeKindVCardStagingFailed,
		Subject:   "A card attached to your mail could not be reviewed",
		// Never "reopen the message" or another phrase implying this notice
		// links to it: an activity subject deliberately resolves to no href
		// (worklist.copy.ts's subjectHref, "an activity resolves to nothing on
		// purpose") so there is nothing here to click through to. And never a
		// flat "this failed" either — the fault may have been transient and a
		// later retry may already have staged the same card, with nothing that
		// withdraws this notice to say so; "if you don't see it" is the phrasing
		// that reads true under both outcomes.
		Body: "A card resembling an existing contact was attached to a message in your mailbox, " +
			"and proposing it for your review may have failed. If you don't see this contact in " +
			"your pending reviews, re-send the card and import it through Import cards on the " +
			"contact list to retry.",
		// Keyed on the CARD's own content, not its position in the batch: the
		// entries a retry re-reads are ordered by the attachment rows'
		// created_at (liveCardKeys), which a concurrent archive or a new
		// attachment can shift between one attempt and the next. A key keyed
		// on position would then dedupe the WRONG card's earlier notice, or
		// miss the same card's, on nothing more than an unrelated attachment
		// changing underneath it. vcardStagingFailureKey uses the identity
		// vcardCreateStager's own proposal already keys on instead.
		DedupeKey: fmt.Sprintf("%s:%s:%s", noticeKindVCardStagingFailed, activity, vcardStagingFailureKey(entry)),
		Target:    notices.Target{Type: string(recordTypeActivity), ID: activity},
	})
	return err
}

// vcardStagingFailureKey is a card's stable identity across retries — the
// same full_name/emails/company triple vcardCreateStager's own proposal
// identity keys on (vcardcreateproposal.go), so two cards that would collide
// as one proposal also collide as one notice, and a card that merely moved
// position in a re-read batch does not raise a second line for itself.
func vcardStagingFailureKey(entry people.VCardEntry) string {
	sum := sha256.Sum256([]byte(people.NormalizePersonName(entry.FullName) + "\x00" +
		loweredCardEmails(entry) + "\x00" + strings.TrimSpace(entry.Company)))
	return hex.EncodeToString(sum[:8])
}

// stageReviewsWith is stageReviews against an injected stager and failure
// notice, so a test can make one card's stage fail without needing a real
// staging conflict — what is under test is the aggregation below, not
// vcardCreateStager's own SQL or recordStagingFailure's own write.
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
// notice again.
//
// A card that refused with ErrPermissionDenied/ErrNotFound AND whose notice
// ALSO failed to write has that staging error demoted to %v rather than %w —
// deliberately losing errors.Is visibility into the one sentinel Work reads
// to mean "not a fault, do not retry" (vcardingest.go). Left as %w, the match
// would stand on THIS card's own contribution and end the retry ladder with
// its notice never raised — the exact silent failure this file exists to
// close, recreated one layer down. Demoting only those two sentinels, and
// only when the notice itself failed, forces the default branch so River
// tries again purely to give the notice another chance; every other staging
// error already retries regardless, with nothing to demote.
func stageReviewsWith(
	ctx context.Context, log *slog.Logger, activity ids.UUID,
	stage func(ctx context.Context, entry people.VCardEntry, candidate *ids.PersonID) error,
	recordFailure func(ctx context.Context, activity ids.UUID, entry people.VCardEntry) error,
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
		entry := entries[r.Index]
		err := stage(ctx, entry, r.PersonID)
		if err == nil {
			continue
		}
		log.ErrorContext(ctx, "a card attached to captured mail could not be staged for review",
			"activity", activity, "card", r.Index+1, "err", err)
		if noteErr := recordFailure(ctx, activity, entry); noteErr != nil {
			log.ErrorContext(ctx, "a mailed card's staging failure could not raise a notice for its importer",
				"activity", activity, "card", r.Index+1, "err", noteErr)
			if errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrNotFound) {
				failures = append(failures, fmt.Errorf("staging card %d: %v (its failure notice also could not be raised: %w)",
					r.Index+1, err, noteErr))
				continue
			}
			failures = append(failures, fmt.Errorf("staging card %d: %w (its failure notice also could not be raised: %v)",
				r.Index+1, err, noteErr))
			continue
		}
		failures = append(failures, err)
	}
	if len(failures) > 0 {
		return fmt.Errorf("compose: staging %d of %d mailed cards for review: %w",
			len(failures), eligible, errors.Join(failures...))
	}
	return nil
}
