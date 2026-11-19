// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func callerOf(p principal.Principal) context.Context {
	return principal.WithActor(context.Background(), p)
}

// A connector carries its granting seat's UserID, so a check on the user id
// alone would let it rewrite that seat's mailbox choices.
func TestOnlyAUserOrTheirAgentChangesTheirOwnMailbox(t *testing.T) {
	callers := map[string]context.Context{
		"no caller": context.Background(),
		"connector": callerOf(principal.Principal{Type: principal.PrincipalConnector, ID: "connector:gmail", UserID: ids.NewV7()}),
		"system":    callerOf(principal.Principal{Type: principal.PrincipalSystem, ID: "system:sweep"}),
	}
	r := NewRegistry(nil, nil, nil, nil)
	for name, ctx := range callers {
		t.Run(name, func(t *testing.T) {
			if _, err := r.SetContextTag(ctx, ProviderMailbox("gmail"), nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
				t.Errorf("filing a mailbox under a word answered %v, want a permission refusal", err)
			}
			if _, err := r.SetMailPosture(ctx, ProviderMailbox("gmail"), "private", false); !errors.Is(err, apperrors.ErrPermissionDenied) {
				t.Errorf("setting a mailbox's posture answered %v, want a permission refusal", err)
			}
			if err := r.Disconnect(ctx, ProviderMailbox("gmail")); !errors.Is(err, apperrors.ErrPermissionDenied) {
				t.Errorf("disconnecting answered %v, want a permission refusal", err)
			}
		})
	}
}

// Granting reads the human's own scopes; an agent has a passport, not a grant
// of its own to hand over.
func TestOnlyAHumanGrantsAConnector(t *testing.T) {
	r := NewRegistry(nil, nil, nil, nil)
	r.Register(fakeMailConnector{name: "fake_mail"})
	ctx := callerOf(principal.Principal{Type: principal.PrincipalAgent, ID: "agent:helper", UserID: ids.NewV7()})
	if _, err := r.Connect(ctx, "fake_mail", connector.Auth("token")); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("an agent granting a connector answered %v, want a permission refusal", err)
	}
}

// The identity list says which of a seat's mail is private, so a connector
// acting for that seat must not read or edit it.
func TestAConnectorCannotReadTheSeatsOwnerIdentities(t *testing.T) {
	ctx := callerOf(principal.Principal{Type: principal.PrincipalConnector, ID: "connector:gmail", UserID: ids.NewV7()})
	if _, err := NewOwnerIdentityStore(nil).List(ctx); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a connector listing owner identities answered %v, want a permission refusal", err)
	}
}

// Files staged under the zero workspace would sit outside every reader's and
// every erasure's reach.
func TestAMessagesFilesAreNotStoredWithoutABoundWorkspace(t *testing.T) {
	keeper := &retryStageBoundary{}
	sink := &Sink{files: keeper}
	rec := connector.NormalizedRecord{Parts: []connector.Part{{Filename: "offer.pdf", Body: []byte("%PDF")}}}
	if _, err := sink.stageParts(context.Background(), rec); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("staging files with no workspace answered %v, want a permission refusal", err)
	}
	if keeper.calls != 0 {
		t.Fatalf("the keeper was asked to store %d times with no workspace bound", keeper.calls)
	}
}

// A checkpoint is the page's resume point, so counts that went backwards or
// do not add up must not become it.
func TestAHistoryCheckpointWhoseCountsDoNotAddUpIsRefused(t *testing.T) {
	cases := map[string]connector.BackfillPageResult{
		"scanned without an outcome": {Scanned: 2, Captured: 1},
		"a count below the last":     {Scanned: -1, Failed: -1},
	}
	for name, res := range cases {
		t.Run(name, func(t *testing.T) {
			if err := (&pageProgress{}).Checkpoint(context.Background(), "next", res); !errors.Is(err, apperrors.ErrInvalidArgument) {
				t.Fatalf("checkpoint answered %v, want an invalid-argument refusal", err)
			}
		})
	}
}

// A page whose lease connection is gone no longer holds its advisory lock, so
// its provider work stops instead of racing the next owner.
func TestALostLeaseConnectionEndsTheHistoryImport(t *testing.T) {
	execution := backfillExecution{lease: &database.SessionLease{}}
	ctx := context.WithValue(context.Background(), backfillExecutionKeyType{}, execution)
	r := NewRegistry(nil, nil, nil, nil).WithOwnershipWatch(time.Nanosecond)
	watched, stop := r.monitorBackfill(ctx)
	defer stop()
	<-watched.Done()
	if !errors.Is(context.Cause(watched), apperrors.ErrConflict) {
		t.Fatalf("the page ended with %v, want an ownership conflict", context.Cause(watched))
	}
}
