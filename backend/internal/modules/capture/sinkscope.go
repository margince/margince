// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The two ways the ladder opens a verdict question, and what each is asking.
//
// deferAmbiguous asks WHETHER a sender should become a record at all: nothing is
// created until the answer comes back. askWhoseRecord asks whose the record
// already being created is, because capture mints every person owner-scoped and
// only a verdict can widen one.
//
// They share recordDisposition and differ in what a full queue costs, which is
// why they are two functions and not one with a flag: a capped deferral means a
// message nobody will judge, while a capped create means a record that stays the
// mailbox owner's until the sender writes again.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// askWhoseRecord opens the verdict question for a sender the ladder is about to
// create a record for.
//
// The record is minted owner-scoped, because capture cannot tell whose it is:
// the workspace writing to an address proves the address is a counterparty, not
// that the counterparty is the business's. A founder's lawyer and a founder's
// customer are both correspondence-positive, and publishing the first to the
// workspace announces that the founder has a lawyer.
//
// So the create tiers ask the same question the deferred tier does, and the
// `person` verdict is what widens the row (people's promoteIfWorkspaceScoped).
// Without this the T1 record stays the mailbox owner's forever: nothing else
// promotes, and the strongest evidence a sender is a counterparty would produce
// the most private record.
//
// alreadyKnown means the ledger has already answered about this address, so
// there is nothing to ask: re-opening it would spend a model call to re-derive a
// decision that stands, and would re-ask an `advisor` answer whose whole point
// is that the record stays the owner's.
func (s *Sink) askWhoseRecord(ctx context.Context, tx pgx.Tx, row dispositionRow, alreadyKnown bool) error {
	if alreadyKnown {
		return nil
	}
	row.Status = PendingStatusPending
	// The ceiling's answer is deliberately dropped. On the deferred tier a cap
	// means the message stands unjudged and the member is told so; here the
	// record is created either way, and the only loss is that it stays the
	// owner's until a later message re-opens the question.
	if _, err := recordDisposition(ctx, tx, row); err != nil {
		return err
	}
	return nil
}

// deferAmbiguous is T4: a first-time sender nothing about this address yet calls
// stranger or customer. ADR-0063's create-on-sight is what manufactured junk
// from exactly this class, so the message is captured, no record is minted, and
// the verdict engine answers the question the ledger now holds.
func (s *Sink) deferAmbiguous(ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord, row dispositionRow) (string, error) {
	row.Status = PendingStatusPending
	capped, err := recordDisposition(ctx, tx, row)
	if err != nil {
		return "", err
	}
	if capped == "" {
		return "", nil
	}
	// A ceiling is holding this question back, which an outsider can drive by
	// mailing from fresh addresses. Say so where an operator will see it, and say
	// WHICH ceiling: silence would read as a sender that was judged and dismissed,
	// and "the queue is full" would misdescribe one domain flooding it while every
	// other sender still gets through.
	detail := "the workspace is at its open-disposition ceiling; the message stands unjudged"
	if capped == CapReasonDomain {
		detail = "this sender's domain is at its share of the open-disposition ceiling; the message stands unjudged"
	}
	// The ceiling rides its own field, not only the prose: an operator filtering
	// for one flooding domain should not have to match on a sentence.
	// The member's answer differs from the operator's here: a capped deferral is
	// not "waiting for a verdict", it is a question that will never be asked.
	return TraceReasonDeferralCapped, s.logBreadcrumbTx(ctx, tx, "capture_deferral_capped", rec, detail,
		map[string]any{"ceiling": capped})
}
