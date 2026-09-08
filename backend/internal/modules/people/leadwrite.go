// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// The prologue every lead mutation shares, and nothing beyond it.

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// leadWrite runs one lead mutation inside the module's write shape: the
// custom-field catalog read ABOVE the transaction, and the writability probe
// inside it.
//
// The PROLOGUE and not the transaction, which is the whole distinction. What
// the two verbs on the other side of it share is the shape every store method
// here spells; what they do not share is any decision inside it — the liveness
// their locks and reads take (disqualify acts only on live rows, reopen only on
// archived ones), the guards, the audit action and the event. A helper covering
// those would take them as parameters and be a switch between two verbs wearing
// one name, which is harder to read than the two. This takes none of them: the
// callback is handed the transaction and the columns and decides everything.
//
// THE OBJECT GATE STAYS WITH THE CALLER, and that is deliberate rather than an
// omission. Passing the actions in made them a variable at the auth.Require
// call site, and identity's grantreachability scan resolves a verb only when it
// is a literal — an unresolvable one takes the door out of the check that every
// grant a handler requires is held by some seeded role. Two lines of repetition
// are worth less than that scan.
//
// The catalog read is deliberately outside the transaction. Read inside one it
// opens a connection of its own while the caller's is held, which is the
// deadlock backend/gates/txseamacquire_test.go refuses.
func (s *Store) leadWrite(
	ctx context.Context,
	id ids.LeadID,
	write func(tx pgx.Tx, active []fieldcatalog.Column) (crmcontracts.Lead, error),
) (crmcontracts.Lead, error) {
	active, err := s.activeColumns(ctx, entityLead)
	if err != nil {
		return crmcontracts.Lead{}, err
	}
	var out crmcontracts.Lead
	err = s.tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureWritable(ctx, tx, entityLead, id.UUID); err != nil {
			return err
		}
		var inner error
		out, inner = write(tx, active)
		return inner
	})
	return out, err
}
