// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// asActivitySeat is a full seat for user holding grant on activities over
// every row.
func (e *promiseEnv) asActivitySeat(user ids.UUID, grant principal.ObjectGrant) context.Context {
	ctx := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.ws), ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user, SeatType: principal.SeatFull,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"}, RowScope: principal.RowScopeAll,
			Objects: map[string]principal.ObjectGrant{"activity": grant, "contact": {Read: true}},
		},
	})
}

// noLinksRestore restores an activity that sits on no list and has no tag.
var noLinksRestore = storekit.RestoreWith{
	Erased: func(context.Context, pgx.Tx, ids.UUID) (bool, error) { return false, nil },
	Links: storekit.LinkRestore{
		Membership: func(context.Context, pgx.Tx, string, ids.UUID, storekit.ListMembership) (bool, error) {
			return false, nil
		},
		Tag: func(context.Context, pgx.Tx, string, ids.UUID, storekit.TagAssignment) (bool, error) {
			return false, nil
		},
	},
}

// The author of a note logged by hand restores only an archive they made.
// An all-rows editor who did not write the note restores none. A delete
// holder restores any archive.
func TestAnActivityRestoreAsksTheArchivesOwnAuthority(t *testing.T) {
	t.Parallel()
	e := setupPromises(t)
	note := e.seedLinkedNote(t, e.rep)
	e.exec(t, `UPDATE activity SET source = 'manual', captured_by = $2 WHERE id = $1`, note, "human:"+e.rep.String())
	store := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))
	id := ids.From[ids.ActivityKind](note)
	author := e.asActivitySeat(e.rep, principal.ObjectGrant{Read: true, Update: true})
	manager := e.asActivitySeat(ids.NewV7(), principal.ObjectGrant{Read: true, Update: true, Delete: true})
	editor := e.asActivitySeat(ids.NewV7(), principal.ObjectGrant{Read: true, Update: true})
	restoreAs := func(ctx context.Context) error {
		return store.tx(ctx, func(tx pgx.Tx) error {
			_, err := store.RestoreActivityTx(ctx, tx, id, nil, noLinksRestore)
			return err
		})
	}

	if _, err := store.ArchiveActivity(manager, id, nil); err != nil {
		t.Fatalf("the manager's archive: %v", err)
	}
	if _, refused := errors.AsType[*auth.RestoreAuthorityError](restoreAs(author)); !refused {
		t.Error("the author restored an archive the manager made, want restore_archiver_required")
	}
	if err := restoreAs(manager); err != nil {
		t.Fatalf("the manager's restore: %v", err)
	}

	if _, err := store.ArchiveActivity(author, id, nil); err != nil {
		t.Fatalf("the author's archive with edit rights: %v", err)
	}
	if _, refused := errors.AsType[*auth.RestoreAuthorityError](restoreAs(editor)); !refused {
		t.Error("an editor who did not write the note restored it, want restore_archiver_required")
	}
	if err := restoreAs(author); err != nil {
		t.Fatalf("the author's restore of their own archive: %v", err)
	}
}
