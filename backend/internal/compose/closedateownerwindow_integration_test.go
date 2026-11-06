// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The close-date sweep decides to correct a deal, and writes it in a later
// transaction. An owner who leaves, loses the deal grant or turns the switch
// off in between gets no correction.

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/installseam"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// changedBeforeTheWrite is the production policy. It changes the owner's
// state as the sweep's write starts to bind the owner. The first bind is the
// sweep's decision to correct the deal. The second opens the write's
// transaction, which holds no lock yet when the change commits.
type changedBeforeTheWrite struct {
	closeDatePolicy
	change func()
	binds  int
}

func (p *changedBeforeTheWrite) ActForOwner(owner ids.UUID) deals.OwnerAuthorityTx {
	bind := p.closeDatePolicy.ActForOwner(owner)
	return func(ctx context.Context, tx pgx.Tx) (context.Context, error) {
		p.binds++
		if p.binds == 2 {
			p.change()
		}
		return bind(ctx, tx)
	}
}

func TestAnOwnerWhoLeavesBeforeTheSweepWritesKeepsTheirDate(t *testing.T) {
	for name, change := range ownerChanges {
		t.Run(name, func(t *testing.T) {
			e := setupCloseDate(t)
			owner := dealOwnerAuthority{db: e.DB(), users: identity.NewServiceFor(e.DB())}
			policy := &changedBeforeTheWrite{
				owner:  owner,
				change: func() { change(e, t) },
			}
			e.corrector = deals.NewCloseDateCorrector(e.DB(), policy,
				quietReviewReader{db: e.DB(), owner: owner}, slog.New(slog.NewTextHandler(os.Stderr, nil)),
				installseam.Deals()).SetClock(func() time.Time { return e.day }).
				WithBaseLanguage(installationLanguage(e.Pool))
			// Overdue and active: the tier that re-dates the deal outright.
			id := e.seedSweepDeal(t, "Slipped but alive", e.early, nil, new(-12), 3)

			if err := e.sweep(); err != nil {
				t.Fatal(err)
			}

			wantDate, outcome := today().AddDate(0, 0, -12), "checked"
			if name == "nothing changes" {
				wantDate, outcome = today().AddDate(0, 0, 2*14), "changed"
			}
			if swept := e.readSwept(t, id); swept.expectedClose == nil || !swept.expectedClose.Equal(wantDate) {
				t.Errorf("date = %v, want %s", swept.expectedClose, wantDate.Format(time.DateOnly))
			}
			if got, want := e.sweptTier(t, id), `member outcome "`+outcome+`", staged []`; got != want {
				t.Errorf("the sweep recorded %q, want %q", got, want)
			}
		})
	}
}

// rowLocksHeld counts the record locks tx already holds.
func rowLocksHeld(ctx context.Context, tx pgx.Tx) (int, error) {
	var held int
	err := tx.QueryRow(ctx, `SELECT count(*) FROM pg_locks WHERE pid = pg_backend_pid()
		AND locktype = 'relation' AND mode IN ('RowShareLock', 'RowExclusiveLock')`).Scan(&held)
	return held, err
}

// bindsBeforeRecords is the production policy, recording the record locks
// each owner bind finds its transaction already holding.
type bindsBeforeRecords struct {
	closeDatePolicy
	locksAtBind []int
}

func (p *bindsBeforeRecords) ActForOwner(owner ids.UUID) deals.OwnerAuthorityTx {
	bind := p.closeDatePolicy.ActForOwner(owner)
	return func(ctx context.Context, tx pgx.Tx) (context.Context, error) {
		held, err := rowLocksHeld(ctx, tx)
		if err != nil {
			return nil, err
		}
		p.locksAtBind = append(p.locksAtBind, held)
		return bind(ctx, tx)
	}
}

// A member's removal takes the authorization lock and then locks that member's
// deals. The sweep binds the owner before it locks a deal, so the two cannot
// wait on each other in a cycle.
func TestTheSweepBindsTheOwnerBeforeItLocksADeal(t *testing.T) {
	e := setupCloseDate(t)
	owner := dealOwnerAuthority{db: e.DB(), users: identity.NewServiceFor(e.DB())}
	policy := &bindsBeforeRecords{owner: owner}
	e.corrector = deals.NewCloseDateCorrector(e.DB(), policy,
		quietReviewReader{db: e.DB(), owner: owner}, slog.New(slog.NewTextHandler(os.Stderr, nil)),
		installseam.Deals()).SetClock(func() time.Time { return e.day }).
		WithBaseLanguage(installationLanguage(e.Pool))
	id := e.seedSweepDeal(t, "Slipped but alive", e.early, nil, new(-12), 3)

	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}

	if got := e.sweptTier(t, id); got != `member outcome "changed", staged []` {
		t.Fatalf("the sweep recorded %q, want the deal corrected", got)
	}
	if len(policy.locksAtBind) != 2 || policy.locksAtBind[0] != 0 || policy.locksAtBind[1] != 0 {
		t.Errorf("record locks held at each owner bind = %v, want [0 0]: the decision's and the write's", policy.locksAtBind)
	}
}

// switchedOffAfterTheWriteReads is the production policy, with Rep1 turning
// corrections off right after the write's transaction read the mode.
type switchedOffAfterTheWriteReads struct {
	closeDatePolicy
	e      *closeDateEnv
	t      *testing.T
	reads  int
	waited bool
}

func (p *switchedOffAfterTheWriteReads) CorrectsWithoutAsking(ctx context.Context, tx pgx.Tx, owner ids.UUID) (bool, error) {
	allowed, err := p.closeDatePolicy.CorrectsWithoutAsking(ctx, tx, owner)
	p.reads++
	if p.reads == 2 {
		p.waited = waitsForTheApply(p.t, switchesOff(p.e))
	}
	return allowed, err
}

// A switch turned off after the sweep's write read the mode waits for that
// write. It cannot commit under a write that already decided to run.
func TestASwitchTurnedOffWaitsForTheSweepsWriteInFlight(t *testing.T) {
	e := setupCloseDate(t)
	if err := e.setSwitch(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	owner := dealOwnerAuthority{db: e.DB(), users: identity.NewServiceFor(e.DB())}
	policy := &switchedOffAfterTheWriteReads{owner: owner, e: e, t: t}
	e.corrector = deals.NewCloseDateCorrector(e.DB(), policy,
		quietReviewReader{db: e.DB(), owner: owner}, slog.New(slog.NewTextHandler(os.Stderr, nil)),
		installseam.Deals()).SetClock(func() time.Time { return e.day }).
		WithBaseLanguage(installationLanguage(e.Pool))
	id := e.seedSweepDeal(t, "Slipped but alive", e.early, nil, new(-12), 3)

	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}

	if !policy.waited {
		t.Error("the switch committed while the sweep's write was open, want it to wait")
	}
	if got := e.sweptTier(t, id); got != `member outcome "changed", staged []` {
		t.Errorf("the sweep recorded %q, want the write the switch waited for", got)
	}
}
