// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// T1: whether this workspace has dealt with an address enough for a record to
// be honest. Its own file because it is the tier's whole question, and the
// ladder that consults it (sinkensure.go) has four more.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// dealtWithEnoughToRecord answers T1: has this workspace dealt with the address
// enough that a record is honest.
//
// Three questions, and T1 used to ask only the first. Whether the workspace
// wrote here (corresponded); whether that amounts to an exchange rather than
// unreturned intent (exchanged); and whether a person is reachable at the
// address at all (recordWorthy) — a mailbox owner books flights and answers
// their own robots, so writing somewhere is not evidence a human is behind it.
// A single send is deferred rather than refused, so a real prospect written to
// once still becomes a contact, for a reason.
//
// A MEETING is the fourth, and the strongest. Mail is evidence about intent —
// a founder mails forty people and hears from six — while a meeting is evidence
// about time, which nobody spends by accident. Before this, a partner the
// workspace was meeting next week satisfied nothing here, and the invitation
// made it worse: it reaches the mailbox as machine-generated mail, so the
// classifier read it as transactional and judged that same partner noise.
//
// The meeting satisfies BOTH mail clauses, and it has to or it carries nothing:
// `corresponded` reads attested outbound mail, `exchanged` reads a reply or a
// second thread, and a guest who only ever appears on a calendar has neither.
// One answer feeding both is what says the meeting IS the evidence, rather than
// a third clause somebody must remember to combine.
//
// recordWorthy still binds. A meeting names people, and `noreply@` on the
// invitation is not one of them.
func (s *Sink) dealtWithEnoughToRecord(
	ctx context.Context, tx pgx.Tx, cp connector.Counterparty,
) (dealtWith, error) {
	var out dealtWith
	var err error
	if out.corresponded, err = correspondencePositiveTx(ctx, tx, cp.Email); err != nil {
		return dealtWith{}, err
	}
	if out.exchanged, out.replied, err = s.exchangedHow(ctx, tx, cp.Email); err != nil {
		return dealtWith{}, err
	}
	met, err := metInPersonTx(ctx, tx, cp.Email)
	if err != nil {
		return dealtWith{}, err
	}
	out.create = (out.corresponded || met) && (out.exchanged || met) && s.recordWorthy(cp)
	return out, nil
}

// dealtWith is everything T1 concluded about one address, carried as one value
// because the fields are all bools answering different questions and a caller
// that swapped two of them would still compile.
type dealtWith struct {
	// create: the tier's answer — a record here is honest.
	create bool

	// corresponded: the workspace has provably written to this address.
	corresponded bool
	// exchanged: that amounts to an exchange rather than unreturned intent.
	exchanged bool
	// replied: they wrote to US, the only one that says the person initiated
	// contact. A meeting deliberately does not set it — sitting in a meeting is
	// worth a record and is not somebody writing to us first.
	replied bool
}
