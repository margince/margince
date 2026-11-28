// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package database

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A resolver that cannot answer must surface, not be swallowed into a
// transaction that then runs unbound. The installation is unresolvable in two
// real states — not bootstrapped yet, and more than one live workspace — and
// both must reach the caller saying which step failed.
func TestAResolverThatCannotAnswerStopsTheTransaction(t *testing.T) {
	t.Parallel()
	refused := errors.New("more than one active workspace")
	db := &DB{workspace: func(context.Context) (ids.WorkspaceID, error) { return ids.WorkspaceID{}, refused }}

	err := db.Tx(context.Background(), func(pgx.Tx) error {
		t.Fatal("the transaction body ran with no workspace resolved")
		return nil
	})
	if !errors.Is(err, refused) {
		t.Fatalf("Tx = %v, want the resolver's own error to reach the caller", err)
	}
	if !strings.Contains(err.Error(), "resolving the installation's workspace") {
		t.Errorf("the wrap must say which step failed, got %q", err)
	}
}

// A store built with no handle answers ErrNoWorkspace rather than panicking.
// The sentinel is load-bearing: callers distinguish "the gate denied me" from
// "the gate admitted me and the probe reached a database it could not bind" by
// exactly this error.
func TestAnUninjectedHandleAnswersTheNoWorkspaceSentinel(t *testing.T) {
	t.Parallel()
	var db *DB

	err := db.Tx(context.Background(), func(pgx.Tx) error { return nil })
	if !errors.Is(err, ErrNoWorkspace) {
		t.Fatalf("Tx on a nil handle = %v, want ErrNoWorkspace", err)
	}
	if !strings.Contains(err.Error(), "compose") {
		t.Errorf("the refusal must name where the handle comes from, got %q", err)
	}

	// Workspace answers the same, because a store that resolves the workspace
	// BEFORE opening a transaction — to name the row that transaction will
	// write — must not be more fragile than one that just opens it. A panic
	// here would turn an un-injected handle from a refusal into a crash on
	// whichever call site happened to ask first.
	if _, err := db.Workspace(context.Background()); !errors.Is(err, ErrNoWorkspace) {
		t.Fatalf("Workspace on a nil handle = %v, want ErrNoWorkspace", err)
	}
}

// BindTo pins the workspace it is given — the shape bootstrap and every
// multi-tenant fixture rely on, where no installation exists for a resolver to
// find or several do.
func TestBindToPinsTheWorkspaceItIsGiven(t *testing.T) {
	t.Parallel()
	ws := ids.From[ids.WorkspaceKind](ids.NewV7())

	got, err := BindTo(nil, ws).Workspace(context.Background())
	if err != nil {
		t.Fatalf("resolving a pinned handle: %v", err)
	}
	if got != ws {
		t.Errorf("pinned workspace = %s, want %s", got, ws)
	}
}

// A handle that spans every workspace names none, so a caller that asks which
// one fails the way an un-injected handle does.
func TestAHandleAcrossWorkspacesNamesNoWorkspace(t *testing.T) {
	t.Parallel()
	if _, err := NewAcrossWorkspaces(nil).Workspace(context.Background()); !errors.Is(err, ErrNoWorkspace) {
		t.Fatalf("Workspace on an across-workspaces handle = %v, want ErrNoWorkspace", err)
	}
}

// An older successful check that finishes after a newer failure must leave the
// handle unproven, and an older success must not displace a newer one.
func TestAnOlderProofNeverOverwritesANewerCheck(t *testing.T) {
	t.Parallel()
	var held atomic.Pointer[proof]
	company := ids.From[ids.WorkspaceKind](ids.NewV7())
	stampProof(&held, proof{at: 100, company: company})
	stampProof(&held, proof{at: -200})
	stampProof(&held, proof{at: 150, company: company})
	if got := held.Load().at; got != -200 {
		t.Fatalf("an older success after a newer failure left the stamp at %d, want -200", got)
	}
	stampProof(&held, proof{at: 300, company: company})
	stampProof(&held, proof{at: 250})
	if got := held.Load().at; got != 300 {
		t.Fatalf("an older check displaced a newer one: stamp %d, want 300", got)
	}
}

// Inside the recheck window the handle answers the proven company with no
// query, and a failed check ends the window at once.
func TestTheProvenCompanyIsTrustedOnlyInsideTheWindow(t *testing.T) {
	t.Parallel()
	at := time.Unix(1_000, 0)
	company := ids.From[ids.WorkspaceKind](ids.NewV7())
	db := (&DB{installation: true, proven: new(atomic.Pointer[proof]), recheck: time.Hour}).
		WithClock(func() time.Time { return at })
	stampProof(db.proven, proof{at: at.UnixNano(), company: company})
	got, err := db.Workspace(context.Background())
	if err != nil || got != company {
		t.Fatalf("inside the window Workspace = %v, %v; want %v with no query", got, err, company)
	}
	at = at.Add(2 * time.Hour)
	if _, ok := db.provenCompany(); ok {
		t.Fatal("a proof older than the window was still trusted")
	}
	at = at.Add(time.Minute)
	stampProof(db.proven, proof{at: -at.UnixNano()})
	if _, ok := db.provenCompany(); ok {
		t.Fatal("a handle whose last check failed counted as proven")
	}
}

// A bounded handle is the same handle with a ceiling. It keeps checking the
// installation and shares its proof.
func TestABoundedHandleKeepsItsInstallationCheck(t *testing.T) {
	t.Parallel()
	base := ForInstallation(nil)
	bounded := base.Bounded(time.Second)
	if !bounded.installation || bounded.proven != base.proven {
		t.Fatalf("Bounded dropped the installation check: installation=%v, shared proof=%v", bounded.installation, bounded.proven == base.proven)
	}
}

// Handles on one pool share a proof, so a request that builds many stores pays
// the check once.
func TestHandlesOnOnePoolShareOneProof(t *testing.T) {
	t.Parallel()
	pool := new(pgxpool.Pool)
	first, second := ForInstallation(pool), ForInstallation(pool)
	if first.proven != second.proven {
		t.Fatal("two handles on one pool hold separate proofs")
	}
	if ForInstallation(pool).proven == ForInstallation(new(pgxpool.Pool)).proven {
		t.Fatal("handles on different pools share a proof")
	}
}
