// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// capturedQuery keeps the statement a read sends and answers it with an error,
// so the read is rendered without a database.
type capturedQuery struct {
	pgx.Tx
	sql string
}

var errCaptured = errors.New("captured")

func (c *capturedQuery) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	c.sql = sql
	return nil, errCaptured
}

// A signal finding reads the activities its signal cites, and its suggestion's
// evidence reads more. Postgres plans every copy of the content gate the
// statement carries, and a pooled connection keeps that memory until it
// closes. So both are gated in one pass, and this pins it.
func TestSignalFindingsGateTheirCitedActivitiesOnce(t *testing.T) {
	read := principal.ObjectGrant{Read: true}
	ctx := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:rep", UserID: ids.NewV7(),
		TeamIDs: []ids.UUID{ids.NewV7()},
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects: map[string]principal.ObjectGrant{
				"signal": read, "activity": read, "company": read, "deal": read,
			},
			RowScope: principal.RowScopeTeam,
		},
	})
	tx := &capturedQuery{}
	if _, err := assuranceSignalExceptions(ctx, tx, nil, nil); !errors.Is(err, errCaptured) {
		t.Fatalf("rendering the signal findings read: %v", err)
	}
	if !strings.Contains(tx.sql, "deal_suggestion s") {
		t.Fatal("the read names no suggestion, so this test reads the wrong statement")
	}
	if got := strings.Count(tx.sql, "FROM activity request_source"); got != 1 {
		t.Fatalf("the signal findings read carries %d content gates, want 1", got)
	}
}
