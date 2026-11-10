// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestMaintenanceKeepsTheTagRemovalThatCommitsWhileItWaits(t *testing.T) {
	for _, commit := range []bool{true, false} {
		t.Run(map[bool]string{true: "committed removal", false: "rolled back removal"}[commit], func(t *testing.T) {
			e := integration.Setup(t)
			ctx := e.Admin()
			store := collections.NewStore(e.DB())
			contact := e.SeedContact(t, "Tagged contact", nil)
			tag, err := store.CreateTag(ctx, "Maintenance", nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.ApplyTag(ctx, tag.ID, "contact", contact); err != nil {
				t.Fatal(err)
			}
			release := make(chan struct{})
			finishRemoval := sync.OnceFunc(func() { close(release) })
			defer finishRemoval()
			ready, removed := make(chan int, 1), make(chan error, 1)
			rollback := errors.New("cancel the tag removal")
			go func() {
				removed <- e.DB().Tx(ctx, func(tx pgx.Tx) error {
					if _, err := store.RemoveTagTx(ctx, tx, tag.ID, "contact", contact); err != nil {
						ready <- 0
						return err
					}
					ready <- backendPID(t, tx)
					<-release
					if !commit {
						return rollback
					}
					return nil
				})
			}()
			holder := <-ready
			if holder == 0 {
				t.Fatal(<-removed)
			}
			applied := make(chan error, 1)
			go func() {
				system := principal.SystemActing(ctx, "system:tag-maintenance")
				applied <- e.DB().Tx(system, func(tx pgx.Tx) error {
					_, err := collections.ApplySystemTagTx(system, tx, "Maintenance", "contact", []ids.UUID{contact}, nil)
					return err
				})
			}()
			if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
				waitForBackendBlockedBy(t, tx, holder, applied)
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			finishRemoval()
			if err := <-removed; err != nil && (commit || !errors.Is(err, rollback)) {
				t.Fatal(err)
			}
			if err := <-applied; err != nil {
				t.Fatal(err)
			}
			want := 1
			if commit {
				want = 0
			}
			if n := e.WsCount(t, `SELECT count(*) FROM taggable WHERE tag_id=@tag AND entity_id=@contact`, pgx.NamedArgs{"tag": tag.ID, "contact": contact}); n != want {
				t.Fatalf("tag assignments=%d, want %d", n, want)
			}
			if commit {
				if _, err := store.ApplyTag(ctx, tag.ID, "contact", contact); err != nil {
					t.Fatalf("a human cannot put the tag back: %v", err)
				}
			}
		})
	}
}
