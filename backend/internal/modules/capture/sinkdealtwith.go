// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// T1: whether this workspace has dealt with an address enough for a record to
// be honest. Its own file because it is the tier's whole question, and the
// ladder that consults it (sinkensure.go) has four more.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// A provider-accepted send records intent; a calendar alone can admit only a business address.
func (s *Sink) dealtWithEnoughToRecord(
	ctx context.Context, tx pgx.Tx, cp connector.Counterparty, capturing ids.UUID,
) (dealtWith, error) {
	var out dealtWith
	var err error
	if out.corresponded, err = correspondencePositiveTx(ctx, tx, cp.Email); err != nil {
		return dealtWith{}, err
	}
	delivered, err := partyWasWrittenToTx(ctx, tx, cp.Email)
	if err != nil {
		return dealtWith{}, err
	}
	out.corresponded = out.corresponded || delivered
	if out.exchanged, out.replied, err = s.exchangedHow(ctx, tx, cp.Email); err != nil {
		return dealtWith{}, err
	}
	// The meeting read is LAST and conditional, because it is the only one of
	// the three that runs per captured message without an index it can fully
	// use. Mail that already satisfies both clauses cannot be changed by it —
	// the answer is already yes — so the ordinary case pays nothing, and the
	// query runs only for the addresses the mail evidence left short.
	if !out.corresponded || !out.exchanged {
		if out.met, out.metElsewhere, err = metInPersonTx(ctx, tx, cp.Email, capturing); err != nil {
			return dealtWith{}, err
		}
	}
	consumer, err := consumerMailSender(ctx, tx, cp.Domain)
	if err != nil {
		return dealtWith{}, err
	}
	if consumer && !delivered {
		out.met, out.metElsewhere = false, false
	}
	out.create = (delivered || out.met) && s.recordWorthy(cp)
	return out, nil
}

// dealtWith is everything T1 concluded about one address, carried as one value
// because the fields are all bools answering different questions and a caller
// that swapped two of them would still compile.
type dealtWith struct {
	// create: the tier's answer — a record here is honest.
	create bool
	// met: a captured meeting connects the workspace to this address. The row
	// being captured counts, which is how a calendar record's guest becomes a
	// contact.
	met bool
	// metElsewhere: the same, minus the row being decided about. It is what may
	// outrank a settled judgement — see positive() and metInPersonTx.
	metElsewhere bool

	// corresponded: the workspace has provably written to this address.
	corresponded bool
	// exchanged: that amounts to an exchange rather than unreturned intent.
	exchanged bool
	// replied: they wrote to US, the only one that says the contact initiated
	// contact. A meeting deliberately does not set it — sitting in a meeting is
	// worth a record and is not somebody writing to us first.
	replied bool
}

// positive reports T1 in the sense the tiers BELOW it read: this workspace has
// dealt with the address, so a stale judgement about it no longer governs.
//
// The distinction matters and cost a bug. `corresponded` alone is what T2 and
// the settled-disposition arm used to consult, and it reads attested outbound
// MAIL — so a partner known only from a meeting was still suppressed by the ESP
// registry, and a prior `noise` verdict still stopped them, while the comments
// beside both said T1 outranks them. Meeting somebody is the same claim those
// arms defer to, so it belongs in the same answer.
//
// It reads metElsewhere and NOT met, and the difference is the whole guard.
// `kind` is caller-supplied, so a message that calls itself a meeting would
// otherwise be its own authority to lift the suppression standing against its
// own sender — one word in a request body, and a judged sender's mail is
// published to the workspace. A meeting captured on some OTHER row is evidence;
// the row under judgement is not evidence about itself.
func (d dealtWith) positive() bool { return d.corresponded || d.metElsewhere }
