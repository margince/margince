// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type identitySnapshotTx struct {
	pgx.Tx
	reads int
}

func (tx *identitySnapshotTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	tx.reads++
	return nil, errors.New("a new identity snapshot needs a database read")
}

func TestMailIdentitySnapshotDoesNotCrossReadersOrTransactions(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"same read", "another transaction", "another reader", "another call"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			tx := &identitySnapshotTx{}
			actor := principal.Principal{Type: principal.PrincipalHuman, ID: "human:first"}
			ctx := principal.WithActor(context.Background(), actor)
			prepared := context.WithValue(ctx, mailSeatSnapshotKey{}, mailSeatSnapshot{
				tx: tx, actorID: actor.ID, identities: map[string][]string{"first": {"first@example.test"}},
			})
			switch scenario {
			case "another transaction":
				tx = &identitySnapshotTx{}
			case "another reader":
				prepared = principal.WithActor(prepared, principal.Principal{Type: principal.PrincipalHuman, ID: "human:second"})
			case "another call":
				prepared = ctx
			}
			identities, err := UnambiguousMailSeatIdentities(prepared, tx)
			if scenario == "same read" {
				if err != nil || tx.reads != 0 || len(identities["first"]) != 1 {
					t.Fatalf("prepared read was not reused: %v, %v", identities, err)
				}
			} else if err == nil || tx.reads != 1 || identities != nil {
				t.Fatalf("snapshot escaped its read boundary: %v, reads=%d, error=%v", identities, tx.reads, err)
			}
		})
	}
}
