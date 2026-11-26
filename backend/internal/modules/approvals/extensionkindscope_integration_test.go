// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package approvals

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/recordkinds"
	"github.com/margince/margince/backend/pkg/extension"
)

const extScopedProbeTable = "ext_probe_owned_row"

// A staged row of a kind with an owner column exists, for the staging check
// and the inbox, only inside the caller's row scope. A colleague's row answers
// as missing at the own level, so no one stages or sees an operation on it.
func TestAStagedRowOfAnOwnedKindExistsOnlyInsideTheCallersScope(t *testing.T) {
	e := setupStaging(t)
	ctx := context.Background()
	for _, stmt := range []string{
		`DROP TABLE IF EXISTS ext.` + extScopedProbeTable,
		`CREATE TABLE ext.` + extScopedProbeTable + ` (id uuid PRIMARY KEY, owner_id uuid)`,
		`GRANT SELECT ON ext.` + extScopedProbeTable + ` TO margince_app`,
	} {
		if _, err := e.owner.Exec(ctx, stmt); err != nil {
			t.Fatalf("preparing the owned table (%s): %v", stmt, err)
		}
	}
	recordkinds.Register("probe", []extension.RecordKindDeclaration{{
		Kind: "owned_row", DisplayName: "Owned row", Table: extScopedProbeTable, OwnerColumn: "owner_id",
		Fields: []extension.RecordKindField{{Name: "id", Label: "Id"}}, NaturalKey: []string{"id"},
	}})
	if err := RegisterExtensionKinds([]ExtensionKind{{
		Verb: "probe_owned_forget", TargetTable: extScopedProbeTable,
		RbacObject: "ext_probe_owned_row", RbacAction: principal.ActionDelete, RequiredScope: principal.ScopeWrite,
	}}); err != nil {
		t.Fatalf("registering the kind: %v", err)
	}
	t.Cleanup(func() {
		recordkinds.Reset()
		if err := RegisterExtensionKinds(nil); err != nil {
			t.Errorf("clearing the registered kinds: %v", err)
		}
		if _, err := e.owner.Exec(context.Background(), `DROP TABLE IF EXISTS ext.`+extScopedProbeTable); err != nil {
			t.Errorf("dropping the owned table: %v", err)
		}
	})
	mine, colleagues, unowned := ids.NewV7(), ids.NewV7(), ids.NewV7()
	if _, err := e.owner.Exec(ctx, `INSERT INTO ext.`+extScopedProbeTable+` (id, owner_id) VALUES ($1, $4), ($2, $5), ($3, NULL)`,
		mine, colleagues, unowned, e.rep, ids.NewV7()); err != nil {
		t.Fatalf("seeding the rows: %v", err)
	}
	for _, level := range []struct {
		scope principal.RowScope
		want  map[ids.UUID]bool
	}{
		{principal.RowScopeOwn, map[ids.UUID]bool{mine: true, unowned: true}},
		{principal.RowScopeAll, map[ids.UUID]bool{mine: true, colleagues: true, unowned: true}},
	} {
		actor := principal.WithActor(principal.WithWorkspaceID(ctx, e.ws), principal.Principal{
			Type: principal.PrincipalHuman, ID: "human:" + e.rep.String(), UserID: e.rep,
			Permissions: principal.Permissions{RowScope: level.scope},
		})
		for _, id := range []ids.UUID{mine, colleagues, unowned} {
			var inbox, staging bool
			if err := withProbeTx(actor, t, e, func(tx pgx.Tx) error {
				found, err := targetsExist(actor, tx, extScopedProbeTable, []ids.UUID{id})
				if err != nil {
					return err
				}
				inbox = found[id]
				staging, err = ExtensionTargetExists(actor, tx, extScopedProbeTable, id)
				return err
			}); err != nil {
				t.Fatalf("%s: %v", level.scope, err)
			}
			if inbox != level.want[id] || staging != level.want[id] {
				t.Errorf("%s scope, row %s: inbox=%t staging=%t, want %t", level.scope, id, inbox, staging, level.want[id])
			}
		}
	}
}
