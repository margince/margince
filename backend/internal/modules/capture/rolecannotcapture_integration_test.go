// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

// The paths besides sync that unseal a member's credential and call their
// provider: the history import preview and the push-watch renewal. A member
// whose role or seat cannot capture reaches neither the vault nor the provider.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
	"github.com/margince/margince/backend/internal/shared/ports/authz/authztest"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// demotableAuthority answers a member who can capture until demoted is set,
// then a read seat, as a member moved to Read-only would read.
type demotableAuthority struct {
	fixtureAuthority
	demoted *bool
}

func (a demotableAuthority) SeatType(ctx context.Context, ws, user ids.UUID) (principal.SeatType, error) {
	if *a.demoted {
		return principal.SeatRead, nil
	}
	return a.fixtureAuthority.SeatType(ctx, ws, user)
}

func (a demotableAuthority) AdmittedAuthority(ctx context.Context, ws, human, _ ids.UUID) (authz.RBAC, principal.SeatType, error) {
	return authztest.AdmittedFromPair(ctx, ws, human, a.EffectiveRBAC, a.SeatType)
}

// unsealCounter counts unseals, so a test can prove none happened.
type unsealCounter struct {
	keyvault.Vault
	gets *int
}

func (v unsealCounter) Get(ctx context.Context, ws ids.WorkspaceID, ref keyvault.Ref) ([]byte, error) {
	*v.gets++
	return v.Vault.Get(ctx, ws, ref)
}

// providerCounter is a mail connector that backfills and keeps a push watch,
// counting every call that would reach its provider.
type providerCounter struct {
	fixtureConnector
	calls *int
}

func (p providerCounter) EstimateBackfill(context.Context, connector.Auth, time.Time) (connector.BackfillEstimate, error) {
	*p.calls++
	return connector.BackfillEstimate{Messages: 3}, nil
}

func (p providerCounter) BackfillPage(context.Context, connector.Auth, time.Time, string, connector.Sink) (connector.BackfillPageResult, error) {
	*p.calls++
	return connector.BackfillPageResult{}, nil
}

func (p providerCounter) Watch(context.Context, connector.Auth, string) (connector.WatchResult, error) {
	*p.calls++
	return connector.WatchResult{ExpiresAt: watchDeadline, Ref: "sub"}, nil
}

type demotedFixture struct {
	ctx     context.Context
	reg     *capture.Registry
	conn    ids.UUID
	user    ids.UserID
	demoted bool
	gets    int
	calls   int
}

// connectThenDemote connects a capturing member's mailbox and then moves them
// to a read seat, with the counters zeroed after the connect.
func connectThenDemote(t *testing.T) *demotedFixture {
	t.Helper()
	ctx, _, _, ws := newCaptureRegistryFixture(t)
	f := &demotedFixture{}
	_, pool := setupCaptureDB(t)
	f.reg = capture.NewRegistry(database.BindTo(pool, ws), nil,
		demotableAuthority{demoted: &f.demoted},
		unsealCounter{Vault: keyvault.NewMemory(), gets: &f.gets})
	f.reg.Register(providerCounter{calls: &f.calls})
	id, err := f.reg.Connect(ctx, "gmail", connector.Auth("fixture-token"))
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	actor, _ := principal.Actor(ctx)
	f.ctx, f.conn, f.user = ctx, id, ids.From[ids.UserKind](actor.UserID)
	f.demoted, f.gets, f.calls = true, 0, 0
	return f
}

func TestARefusedMembersImportPreviewUnsealsNothingAndAsksNoProvider(t *testing.T) {
	f := connectThenDemote(t)

	_, err := f.reg.EstimateBackfill(f.ctx, "gmail", f.user, 6)

	if _, refused := errors.AsType[*capture.RoleCannotCaptureError](err); !refused {
		t.Fatalf("EstimateBackfill = %v, want the role refusal", err)
	}
	if f.gets != 0 || f.calls != 0 {
		t.Fatalf("the refused preview unsealed %d credentials and made %d provider calls, want none", f.gets, f.calls)
	}
}

func TestARefusedMembersWatchRenewalUnsealsNothingAndAsksNoProvider(t *testing.T) {
	f := connectThenDemote(t)

	err := f.reg.RenewWatch(principal.WithWorkspaceID(context.Background(), mustWorkspace(t, f.ctx)), f.conn, "topic")

	if _, refused := errors.AsType[*capture.RoleCannotCaptureError](err); !refused {
		t.Fatalf("RenewWatch = %v, want the role refusal", err)
	}
	if f.gets != 0 || f.calls != 0 {
		t.Fatalf("the refused renewal unsealed %d credentials and made %d provider calls, want none", f.gets, f.calls)
	}
}

func mustWorkspace(t *testing.T, ctx context.Context) ids.UUID {
	t.Helper()
	ws, ok := principal.WorkspaceID(ctx)
	if !ok {
		t.Fatal("the fixture context carries no workspace")
	}
	return ws
}
