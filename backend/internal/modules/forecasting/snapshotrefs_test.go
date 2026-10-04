// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func readerOf(grant principal.ObjectGrant) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, UserID: ids.NewV7(),
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"forecast": grant},
			RowScope: principal.RowScopeAll,
		},
	})
}

// Nothing is frozen against a population that names no single subject, so the
// answer is empty and needs no query — and empty, not nil, because null reads as
// "unknown" on the wire.
func TestManagedTeamsHasNoSnapshotsToList(t *testing.T) {
	t.Parallel()
	refs, err := NewStore(nil).SnapshotRefsTx(
		readerOf(principal.ObjectGrant{Read: true}), nil, Period{}, Scope{Kind: ScopeManagedTeams})
	if err != nil {
		t.Fatalf("listing snapshots under managed_teams: %v", err)
	}
	if refs == nil || len(refs) != 0 {
		t.Fatalf("managed_teams listed %v, want an empty non-nil list", refs)
	}
}

func TestListingSnapshotsNeedsTheForecastReadGrant(t *testing.T) {
	t.Parallel()
	_, err := NewStore(nil).SnapshotRefsTx(
		readerOf(principal.ObjectGrant{Create: true}), nil, Period{}, Scope{Kind: ScopeWorkspace})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a seat without forecast.read listed snapshots: err = %v, want ErrPermissionDenied", err)
	}
}

func TestSnapshotRefsRenderAsAnEmptyListNotAbsent(t *testing.T) {
	t.Parallel()
	if wire := SnapshotRefsToWire(nil); wire == nil || len(*wire) != 0 {
		t.Fatalf("no snapshots rendered as %v, want a present empty list", wire)
	}
	id, at := ids.NewV7(), time.Date(2026, 2, 1, 9, 0, 0, 0, time.UTC)
	wire := SnapshotRefsToWire([]SnapshotRef{{ID: id, TakenAt: at, Trigger: TriggerCall}})
	got := (*wire)[0]
	if ids.UUID(got.Id) != id || !got.TakenAt.Equal(at) || string(got.Trigger) != TriggerCall {
		t.Fatalf("the reference came out as %+v", got)
	}
}
