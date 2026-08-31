// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What a verdict CREATES, and for whom.
//
// Two of the sender kinds end in records: `person` makes the workspace's
// contact, `advisor` makes the mailbox owner's. They share one assembler
// because they differ in a single field, and a second spelling is how the
// linking, the triage hand-off and the erasure check would drift apart between
// the two.
//
// Split from the engine beside it, which is claim, apply and sweep machinery
// that does not change when a new kind starts creating something.

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// createCounterparty is the `real` effect: the records capture withheld while
// the sender was ambiguous, created now under the human who granted the
// connection — not under the job, which owns nothing.
//
// An address suppressed since capture — an erasure landed while the question was
// open — creates nothing, and says so: the row is corrected to `suppressed`
// rather than left reading `real`. Erasure outranks a verdict, and a ledger (or
// a SAR built from it) that reports `real` for someone with no record would be
// describing a person who does not exist.
func (e *CounterpartyVerdictEngine) createCounterparty(ctx context.Context, tx pgx.Tx, row capture.PendingCounterparty) (string, error) {
	created, err := createCounterpartyRecords(ctx, tx, e.people, e.activities, counterpartyCreation{
		Email:       row.Email,
		DisplayName: row.DisplayName,
		Domain:      row.Domain,
		OwnerID:     row.OwnerID,
		ActivityID:  row.ActivityID,
		Source:      verdictReason,
		CapturedBy:  verdictActor,
	})
	if err != nil {
		return "", err
	}
	if created.Suppressed {
		// The verdict was already written by apply(), and writing it spent the
		// claim — so this corrects the status it just set rather than trying to
		// resolve the row a second time.
		return "", e.pending.CorrectResolution(ctx, tx, row.ID,
			capture.PendingStatusReal, capture.PendingStatusSuppressed,
			"the address was erased before the verdict landed")
	}
	return created.TriageDomain, nil
}

// createOwnerScopedCounterparty is the `advisor` effect: the same records an
// ordinary verdict makes, kept visible to the mailbox owner alone.
//
// It shares createCounterpartyRecords with the ordinary path rather than
// spelling the creation twice — the two differ in ONE field, and a second
// assembler is how the linking, the triage hand-off and the erasure check would
// drift apart between them.
func (e *CounterpartyVerdictEngine) createOwnerScopedCounterparty(ctx context.Context, tx pgx.Tx, row capture.PendingCounterparty) (string, error) {
	created, err := createCounterpartyRecords(ctx, tx, e.people, e.activities, counterpartyCreation{
		Email:       row.Email,
		DisplayName: row.DisplayName,
		Domain:      row.Domain,
		OwnerID:     row.OwnerID,
		ActivityID:  row.ActivityID,
		Source:      verdictReason,
		CapturedBy:  verdictActor,
		OwnerScoped: true,
	})
	if err != nil {
		return "", err
	}
	if created.Suppressed {
		return "", e.pending.CorrectResolution(ctx, tx, row.ID,
			capture.PendingStatusReal, capture.PendingStatusSuppressed,
			"the address was erased before the verdict landed")
	}
	return created.TriageDomain, nil
}

// counterpartyCreation names one deferred sender being turned into records.
type counterpartyCreation struct {
	Email       string
	DisplayName string
	Domain      string
	OwnerID     ids.UUID
	ActivityID  ids.UUID
	// Source is the provenance CHANNEL — which mechanism produced these records.
	Source string
	// CapturedBy is the acting PRINCIPAL, in the contract's declared grammar
	// (`human:<uuid>` | `agent:<id>` | `connector:<name>`). The two are not the
	// same thing and stamping the channel into both puts a value on the wire
	// that no client can parse.
	CapturedBy string
	// OwnerScoped births the person visible to the mailbox owner alone. An
	// ordinary verdict leaves this false, which is what PROMOTES a record
	// capture minted owner-scoped; an advisor verdict sets it, so the record is
	// made and the promotion does not happen.
	OwnerScoped bool
}

// counterpartyCreated reports what a `real` answer produced that its caller has
// to act on AFTER the transaction commits. Today that is one thing: the domain
// whose organization question is still open, which somebody has to queue a
// triage read for.
type counterpartyCreated struct {
	// Suppressed marks an address erased between capture and the answer. Nothing
	// was created: erasure outranks a verdict.
	Suppressed bool
	// TriageDomain names the domain still owed an organization verdict, empty
	// when there is none.
	TriageDomain string
}

// createCounterpartyRecords is the ONE spelling of what a `real` answer does,
// shared by the machine's ordinary verdict, the machine's advisor verdict and
// the human accept. They differ in who decided and in whether the record stays
// the owner's; what gets created — and that the sender's whole captured cohort
// is linked, not just the message that raised the question — must not.
//
// Held by: TestOneAssemblerCreatesEveryCounterpartyAVerdictMakes
// (backend/internal/compose/captureverdictkinds_test.go), which fails when a
// second verdict-side file calls EnsureCounterpartyTx.
func createCounterpartyRecords(ctx context.Context, tx pgx.Tx, store *people.Store,
	timeline *activities.Store, in counterpartyCreation,
) (counterpartyCreated, error) {
	res, err := store.EnsureCounterpartyTx(ctx, tx, people.EnsureCounterpartyInput{
		Email:       in.Email,
		DisplayName: in.DisplayName,
		Domain:      in.Domain,
		OwnerID:     in.OwnerID,
		ActivityID:  ids.From[ids.ActivityKind](in.ActivityID),
		Source:      in.Source,
		CapturedBy:  in.CapturedBy,
		OwnerScoped: in.OwnerScoped,
	})
	if errors.Is(err, people.ErrCounterpartySuppressed) {
		return counterpartyCreated{Suppressed: true}, nil
	}
	if err != nil {
		return counterpartyCreated{}, err
	}
	// The ensure links the message that raised the question; the sender may have
	// written more while it was open, and all of them belong on this person's
	// timeline rather than only the first.
	if err := timeline.LinkCapturedMailTx(ctx, tx, res.PersonID, in.Email); err != nil {
		return counterpartyCreated{}, err
	}
	out := counterpartyCreated{}
	if res.TriagePending {
		out.TriageDomain = res.TriageDomain
	}
	return out, nil
}
