// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What a verdict CREATES, and for whom.
//
// Two of the sender kinds end in records: `contact` makes the workspace's
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

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// createCounterparty is the `real` effect: the records capture withheld while
// the sender was ambiguous, created now under the human who granted the
// connection — not under the job, which owns nothing.
//
// An address suppressed since capture — an erasure landed while the question was
// open — creates nothing, and says so: the row is corrected to `suppressed`
// rather than left reading `real`. Erasure outranks a verdict, and a ledger (or
// a SAR built from it) that reports `real` for someone with no record would be
// describing a contact who does not exist.
func (e *CounterpartyVerdictEngine) createCounterparty(ctx context.Context, tx pgx.Tx, row capture.PendingCounterparty) (string, error) {
	created, err := createCounterpartyRecords(ctx, tx, e.contacts, e.tagFiler, counterpartyCreation{
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
	created, err := createCounterpartyRecords(ctx, tx, e.contacts, e.tagFiler, counterpartyCreation{
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
	// OwnerScoped births the contact visible to the mailbox owner alone. An
	// ordinary verdict leaves this false, which is what PROMOTES a record
	// capture minted owner-scoped; an advisor verdict sets it, so the record is
	// made and the promotion does not happen.
	OwnerScoped bool
}

// counterpartyCreated reports what a `real` answer produced that its caller has
// to act on AFTER the transaction commits. Today that is one thing: the domain
// whose company question is still open, which somebody has to queue a
// triage read for.
type counterpartyCreated struct {
	// Suppressed marks an address erased between capture and the answer. Nothing
	// was created: erasure outranks a verdict.
	Suppressed bool
	// TriageDomain names the domain still owed a company verdict, empty
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
func createCounterpartyRecords(ctx context.Context, tx pgx.Tx, store *contacts.Store,
	filer *connectorTagFiler, in counterpartyCreation,
) (counterpartyCreated, error) {
	res, err := store.EnsureCounterpartyTx(ctx, tx, contacts.EnsureCounterpartyInput{
		Email:       in.Email,
		DisplayName: in.DisplayName,
		Domain:      in.Domain,
		OwnerID:     in.OwnerID,
		ActivityID:  ids.From[ids.ActivityKind](in.ActivityID),
		Source:      in.Source,
		CapturedBy:  in.CapturedBy,
		OwnerScoped: in.OwnerScoped,
	})
	if errors.Is(err, contacts.ErrCounterpartySuppressed) {
		return counterpartyCreated{Suppressed: true}, nil
	}
	if err != nil {
		return counterpartyCreated{}, err
	}
	// The ensure links the message that raised the question; the sender may have
	// written more while it was open, and all of them belong on this contact's
	// timeline rather than only the first.
	//
	// Synchronously, on the verdict's own transaction, although the same repair
	// also runs off the contact event: this cohort is a commit-time promise the
	// verdict makes, and a promise kept by a consumer is kept at some other time.
	// The promotion is idempotent, so the consumer's later pass finds nothing
	// left to do.
	if _, err := store.PromoteContactCohortTx(ctx, tx, res.ContactID); err != nil {
		return counterpartyCreated{}, err
	}
	// Only a contact this ensure MADE. An address that already had a record is
	// not something this connector captured — filing it now would claim the
	// batch brought in a contact that was already here.
	if res.ContactCreated {
		if err := filer.fileUnderConnectorTag(ctx, tx, in, res.ContactID); err != nil {
			return counterpartyCreated{}, err
		}
	}
	out := counterpartyCreated{}
	if res.TriagePending {
		out.TriageDomain = res.TriageDomain
	}
	return out, nil
}

// createContactForVerdict makes the contact a `contact` verdict earns, and
// decides HOW WIDELY it is visible.
//
// The verdict says the sender is a named human. It does not say the workspace
// should be told, and two cases turn on that difference:
//
//   - A message the mailbox owner WROTE, to an address that has never answered.
//     That is an intention, not a relationship: nobody has written in, nothing
//     is owed, and minting a shared contact publishes who a rep is prospecting.
//     The record is the owner's until the address answers.
//   - A thread a confidentiality hold covers. The hold is a statement about who
//     may read the correspondence, and a workspace-visible contact minted off
//     it announces the counterparty the hold exists to keep quiet.
//
// Both keep the record — the owner corresponded with somebody real — and both
// keep it owner-scoped. Anything else is the ordinary shared contact.
func (e *CounterpartyVerdictEngine) createContactForVerdict(
	ctx context.Context, tx pgx.Tx, row capture.PendingCounterparty,
) (string, error) {
	narrow, err := e.contactStaysTheOwners(ctx, tx, row)
	if err != nil {
		return "", err
	}
	if narrow {
		// The ledger says so too, in this transaction.
		//
		// Two other readers ask this ledger whether the sender is a judged
		// contact and treat the answer as permission to publish: the widening
		// sweep that reopens held mail, and the birth decision that shares a
		// future message. Recording the withholding only on the contact row left
		// both of them matching a contact that had deliberately been kept
		// private, so the next pass republished what this one withheld.
		if err := capture.MarkWithheldFromWorkspaceTx(ctx, tx, row.ID); err != nil {
			return "", err
		}
		return e.createOwnerScopedCounterparty(ctx, tx, row)
	}
	triageDomain, err := e.createCounterparty(ctx, tx, row)
	if err != nil {
		return "", err
	}
	// The mail a `classified` mailbox held while it waited for this answer.
	// Bounded, and not drained here: this transaction already carries the
	// ledger resolution and a contact record, and a sender with a thousand held
	// messages would hold it open for all of them. The reconciling pass
	// finishes what this leaves.
	//
	// Only on the shared path. Widening the held mail for a record that is
	// deliberately owner-scoped would publish exactly what the narrowing is
	// withholding.
	return triageDomain, e.widenClearedSender(ctx, tx, row.Email)
}

// contactStaysTheOwners reports whether a `contact` verdict's record must stay
// visible to the mailbox owner alone.
func (e *CounterpartyVerdictEngine) contactStaysTheOwners(
	ctx context.Context, tx pgx.Tx, row capture.PendingCounterparty,
) (bool, error) {
	// An address WE reached that has never answered. Judged on the recorded
	// direction rather than inferred: a row written before the direction was
	// recorded says nothing, and treating an unknown direction as outbound
	// would narrow every historical contact at once.
	if row.Direction == connector.DirectionOutbound && !row.WroteBack {
		return true, nil
	}
	// A thread under a confidentiality hold. The hold says who may read the
	// correspondence, and a workspace-visible contact minted off it announces
	// the counterparty the hold exists to keep quiet — the record would name
	// them on a surface everybody reads while the mail itself stayed shut.
	return capture.ThreadHoldsItsCounterparty(ctx, tx, row.ActivityID)
}
