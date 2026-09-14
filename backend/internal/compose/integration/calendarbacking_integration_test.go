// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Whether a calendar backs a free/busy answer, against the real table.
//
// The agents-side tests inject the verdict through a double, so the statement
// itself, the provider set it is given, and the states it must reject were
// unexercised — and this read is on the critical path of check_availability:
// it fails the whole call rather than degrading. An untested statement whose
// failure mode is "the tool stops answering" is a worse trade than the empty
// diary it was written to prevent.
//
// A grant that is not delivering is the case worth pinning. `error` and
// `reauth_required` rows carry a standing grant and carry no events, so an
// answer that counted them would claim a diary it is not being shown — which
// is the defect, wearing a connection.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// calendarProvidersUnderTest mirrors what compose derives for the seam. It is
// spelled here rather than imported because this suite is asserting that the
// STATEMENT answers for those names; the derivation itself has its own test.
var calendarProvidersUnderTest = []string{"gcal", "graphcal"}

func seedConnection(ctx context.Context, t *testing.T, e *SearchEnv, provider string, user ids.UUID, status string) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO capture_connection (provider, user_id, scopes, status, auth)
			VALUES ($1, $2, '{}', $3, $4)`,
			provider, user, status, []byte(`{"refresh_token":"r","granted":[]}`))
		return err
	}); err != nil {
		t.Fatalf("seeding the %s connection at %s: %v", provider, status, err)
	}
}

func connectedOnAny(ctx context.Context, t *testing.T, e *SearchEnv, user ids.UUID) bool {
	t.Helper()
	var connected bool
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		var err error
		connected, err = capture.ConnectedOnAny(ctx, tx, ids.From[ids.UserKind](user), calendarProvidersUnderTest)
		return err
	}); err != nil {
		t.Fatalf("reading the user's live connections: %v", err)
	}
	return connected
}

// Each calendar provider on its own answers true, so a workspace on Microsoft
// is not told its diary is unread because the set happens to lead with Google.
func TestEachCalendarProviderBacksTheAnswerOnItsOwn(t *testing.T) {
	for _, provider := range calendarProvidersUnderTest {
		t.Run(provider, func(t *testing.T) {
			ctx := context.Background()
			e := SetupSearch(t)
			if connectedOnAny(ctx, t, e, e.Rep1) {
				t.Fatal("a user with no connection at all reads as connected")
			}
			seedConnection(ctx, t, e, provider, e.Rep1, "connected")
			if !connectedOnAny(ctx, t, e, e.Rep1) {
				t.Errorf("a live %s connection does not back the answer", provider)
			}
		})
	}
}

// A grant that is not delivering is not a calendar. Each of these carries a
// row and carries no events.
func TestAConnectionThatIsNotDeliveringDoesNotBackTheAnswer(t *testing.T) {
	for _, status := range []string{"error", "reauth_required", "disconnected"} {
		t.Run(status, func(t *testing.T) {
			ctx := context.Background()
			e := SetupSearch(t)
			seedConnection(ctx, t, e, "gcal", e.Rep1, status)
			if connectedOnAny(ctx, t, e, e.Rep1) {
				t.Errorf("a %s connection backs the answer, so a window with no events in it "+
					"is reported as a reading of the host's diary", status)
			}
		})
	}
}

// And it answers for the user asked about, not for whoever is connected.
func TestAnotherUsersConnectionDoesNotBackThisOne(t *testing.T) {
	ctx := context.Background()
	e := SetupSearch(t)
	seedConnection(ctx, t, e, "gcal", e.Rep1, "connected")
	if connectedOnAny(ctx, t, e, e.Rep3) {
		t.Error("a colleague's calendar backs this user's window")
	}
}
