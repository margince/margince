// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The technical lookup writes under TWO names, and the difference is the whole
// answer an audit of it is read for.
//
// The sweep NOMINATES — it walks the installation's own companies asking which
// it has not looked at lately — and the per-company lookup READS, going out to
// DNS and a certificate log. Both are the system acting, so both would read as
// "system" under one name, and a reader asking "why did this company's
// technical picture change at 3am" could not tell a scheduled pass from a rep's
// press.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestTheTechnicalSweepAndTheLookupAreTwoActors(t *testing.T) {
	t.Parallel()
	for _, pass := range []struct {
		what string
		bind func(context.Context) context.Context
		name string
	}{
		{"the per-company lookup", technicalActor, "system:technical-lookup"},
		{"the sweep that nominates", technicalBackfillActor, "system:technical-backfill"},
	} {
		t.Run(pass.what, func(t *testing.T) {
			t.Parallel()
			ctx := pass.bind(context.Background())

			actor, bound := principal.Actor(ctx)
			if !bound {
				t.Fatalf("%s binds no actor, so every row it writes says nobody did it", pass.what)
			}
			if actor.Type != principal.PrincipalSystem || actor.ID != pass.name {
				t.Errorf("%s acts as %s/%q, want %s/%q", pass.what,
					actor.Type, actor.ID, principal.PrincipalSystem, pass.name)
			}
			// The trace, not only the actor: the apply and the ledger write are
			// separate statements, and without one nobody can read them back as
			// the same pass.
			if _, traced := principal.CorrelationID(ctx); !traced {
				t.Errorf("%s binds no correlation id, so its apply and its ledger row cannot be read back as one pass", pass.what)
			}
		})
	}

	lookup, _ := principal.Actor(technicalActor(context.Background()))
	sweep, _ := principal.Actor(technicalBackfillActor(context.Background()))
	if lookup.ID == sweep.ID {
		t.Errorf("both halves act as %q — a scheduled nomination and a read of somebody's records are then the same row to an auditor", lookup.ID)
	}
}
