// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Ending a contact's confidentiality hold once nothing holds it any more.
//
// A `contact` verdict on a held thread keeps the contact to its owner and marks
// the ledger withheld. The thread may clear later. releaseHeldContactTx is the
// one decision that may then publish the contact. The thread hooks, the
// reconciling pass and the data task all call it, so verdict order and caller
// never change where a contact ends.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// heldContactRelease is what one release did to a contact.
type heldContactRelease int

const (
	// releaseNotHeld: the contact is not held for confidentiality alone, so the
	// release did not look further.
	releaseNotHeld heldContactRelease = iota
	// releaseKept: some hold or open question still applies, or nothing on the
	// ledger shows the contact was a judged business contact.
	releaseKept
	// releasePublished: the contact and its sender's ledger rows went to the workspace.
	releasePublished
	// releaseRenarrowed: the hold ended, and the contact stays the owner's
	// because the owner wrote to an address that never answered.
	releaseRenarrowed
)

// releaseHeldContactTx ends contact id's confidentiality hold when the ledger
// proves it may end, on the caller's transaction.
//
// Every piece of evidence is read after the contact row is locked. Then
// contactStaysTheOwners' rule picks the workspace or outbound_no_answer, as it
// would have for a thread that never held.
func releaseHeldContactTx(ctx context.Context, tx pgx.Tx, store *contacts.Store, id ids.ContactID) (heldContactRelease, error) {
	held, ok, err := contacts.LockConfidentialityHeldTx(ctx, tx, id)
	if err != nil || !ok {
		return releaseNotHeld, err
	}
	next, kept, err := heldContactNextReason(ctx, tx, held.Emails)
	if err != nil || kept {
		return releaseKept, err
	}
	moved, err := store.EndConfidentialityHoldTx(ctx, tx, id, next)
	if err != nil || !moved {
		return releaseKept, err
	}
	if next != "" {
		return releaseRenarrowed, nil
	}
	return releasePublished, publishReleasedSenderTx(ctx, tx, store, held)
}

// heldContactNextReason answers what a held contact becomes: "" for the
// workspace, NarrowedOutboundNoAnswer, or kept when the hold has not ended.
func heldContactNextReason(ctx context.Context, tx pgx.Tx, emails []string) (contacts.NarrowingReason, bool, error) {
	standings, err := capture.SenderStandingsTx(ctx, tx, emails)
	if err != nil {
		return "", true, err
	}
	withheld, unanswered := 0, false
	for _, s := range standings {
		if s.OpenQuestion() {
			return "", true, nil
		}
		if !s.WithheldContact() {
			continue
		}
		withheld++
		stillHeld, err := capture.ThreadHoldsItsCounterparty(ctx, tx, s.ActivityID)
		if err != nil || stillHeld {
			return "", true, err
		}
		if s.Outbound {
			answered, err := capture.AddressAnsweredTx(ctx, tx, s.Email)
			if err != nil {
				return "", true, err
			}
			unanswered = unanswered || !answered
		}
	}
	if withheld == 0 {
		return "", true, nil
	}
	if unanswered {
		return contacts.NarrowedOutboundNoAnswer, false, nil
	}
	return "", false, nil
}

// publishReleasedSenderTx does what a publishing `contact` verdict does next.
// The ledger stops withholding the sender. Mail held for the sender's answer
// opens up to the live bound. The sender's captured messages join the contact.
func publishReleasedSenderTx(ctx context.Context, tx pgx.Tx, store *contacts.Store, held contacts.ConfidentialityHeldContact) error {
	for _, email := range held.Emails {
		if err := capture.ClearWithheldFromWorkspaceTx(ctx, tx, email); err != nil {
			return err
		}
		if _, err := capture.WidenClearedSenderTx(ctx, tx, email, clearedSenderLiveBound, activities.RecomputeAudienceTx); err != nil {
			return err
		}
	}
	_, err := store.PromoteContactCohortTx(ctx, tx, held.ID)
	return err
}

const (
	// heldContactLiveBound bounds the contacts one cleared thread releases while
	// its verdict is applied. The reconciling pass releases the rest.
	heldContactLiveBound = 20
	// heldContactSweepSenders bounds the addresses one reconciling pass reads.
	heldContactSweepSenders = 50
)

// heldContactReleaser runs releases on their own transactions, each as the
// confidentiality engine, after the thread decision that allowed them has
// committed.
type heldContactReleaser struct {
	pool     *pgxpool.Pool
	contacts *contacts.Store
	log      *slog.Logger
	// after is where each workspace's reconciling pass goes on from. In memory:
	// a restarted worker starts its walk over, which costs a pass and loses
	// nothing, because every release re-reads its evidence.
	mu    sync.Mutex
	after map[ids.UUID]string
}

func newHeldContactReleaser(pool *pgxpool.Pool, log *slog.Logger) *heldContactReleaser {
	return &heldContactReleaser{
		pool: pool, contacts: newCounterpartyStore(pool), log: log,
		after: map[ids.UUID]string{},
	}
}

// releaseThread releases the contacts a cleared thread withheld, up to the live
// bound. A failure is logged and left to the reconciling pass: the thread
// decision has committed, and failing its caller now would undo nothing.
func (r *heldContactReleaser) releaseThread(ctx context.Context, threadKey string) {
	ctx = principal.SystemActing(ctx, confidentialityActor)
	var held []ids.ContactID
	err := database.WithWorkspaceTx(ctx, r.pool, func(tx pgx.Tx) error {
		emails, err := capture.WithheldSendersOnThreadTx(ctx, tx, threadKey, heldContactLiveBound)
		if err != nil {
			return err
		}
		held, err = contacts.ConfidentialityHeldByEmailTx(ctx, tx, emails, heldContactLiveBound)
		return err
	})
	if err != nil {
		r.log.WarnContext(ctx, "confidentiality: listing the contacts a cleared thread withheld failed", "err", err)
		return
	}
	for _, id := range held {
		if _, err := r.releaseOne(ctx, id); err != nil {
			r.log.WarnContext(ctx, "confidentiality: releasing a held contact failed", "contact", id.String(), "err", err)
		}
	}
}

// releaseOne runs releaseHeldContactTx on its own transaction.
func (r *heldContactReleaser) releaseOne(ctx context.Context, id ids.ContactID) (heldContactRelease, error) {
	var out heldContactRelease
	err := database.WithWorkspaceTx(ctx, r.pool, func(tx pgx.Tx) error {
		var err error
		out, err = releaseHeldContactTx(ctx, tx, r.contacts, id)
		return err
	})
	if err != nil {
		return releaseKept, fmt.Errorf("confidentiality: releasing a held contact: %w", err)
	}
	return out, nil
}

// reconcileWorkspace releases the held contacts of the next page of withheld
// senders nothing holds any more, in the workspace bound in ctx. It returns how
// many contacts changed.
func (r *heldContactReleaser) reconcileWorkspace(ctx context.Context) (int, error) {
	workspace, ok := principal.WorkspaceID(ctx)
	if !ok {
		return 0, errors.New("confidentiality: releasing held contacts requires a workspace-bound context")
	}
	ctx = principal.SystemActing(ctx, confidentialityActor)
	r.mu.Lock()
	after := r.after[workspace]
	r.mu.Unlock()
	var held []ids.ContactID
	var senders []string
	err := database.WithWorkspaceTx(ctx, r.pool, func(tx pgx.Tx) error {
		var err error
		senders, err = capture.ReleasableWithheldSendersTx(ctx, tx, after, heldContactSweepSenders)
		if err != nil {
			return err
		}
		held, err = contacts.ConfidentialityHeldByEmailTx(ctx, tx, senders, heldContactSweepSenders)
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("confidentiality: listing held contacts nothing holds: %w", err)
	}
	next := ""
	if len(senders) == heldContactSweepSenders {
		next = senders[len(senders)-1]
	}
	r.mu.Lock()
	r.after[workspace] = next
	r.mu.Unlock()
	changed := 0
	var failed []error
	for _, id := range held {
		out, err := r.releaseOne(ctx, id)
		if err != nil {
			failed = append(failed, err)
			continue
		}
		if out == releasePublished || out == releaseRenarrowed {
			changed++
		}
	}
	if len(failed) > 0 {
		return changed, fmt.Errorf("confidentiality: %d of %d held contacts not released: %w",
			len(failed), len(held), errors.Join(failed...))
	}
	return changed, nil
}
