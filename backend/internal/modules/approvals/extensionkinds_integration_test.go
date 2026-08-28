// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package approvals

// The existence probe a registered unit table gets, run against a real one.
//
// Everything else about an extension kind is decided in memory and is proven
// there. This half cannot be: the probe interpolates a table NAME into a
// statement and qualifies it with the ext schema, and whether that statement
// parses, resolves and answers is a question only Postgres can settle. A unit
// table addressed without its schema, or quoted wrongly, fails here and
// nowhere else — and its failure mode in production is an approval nobody can
// see, because a staged row whose target cannot be proven to exist is dropped
// from the inbox rather than shown.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The ext schema is core's, and this table stands in for
// a unit's own. Created by the test rather than migrated in: the lane migrates
// core and custom only, so no unit table exists here — and what is under test
// is the probe's SQL against a table in that schema, which any table proves.
const extProbeTable = "ext_probe_staged_row"

func TestTheExistenceProbeReadsARegisteredUnitTable(t *testing.T) {
	e := setupStaging(t)
	ctx := context.Background()
	if _, err := e.owner.Exec(ctx,
		`CREATE TABLE IF NOT EXISTS ext.`+extProbeTable+` (id uuid PRIMARY KEY)`); err != nil {
		t.Fatalf("standing up the unit table this probe reads: %v", err)
	}
	// The probe runs as the APPLICATION role, not as the owner, so the grant a
	// unit migration writes is part of what makes a table probe-able at all —
	// without it the inbox answers "permission denied" for a row that is
	// plainly there, and a real unit table that forgot the grant would fail
	// exactly here.
	if _, err := e.owner.Exec(ctx,
		`GRANT SELECT ON ext.`+extProbeTable+` TO margince_app`); err != nil {
		t.Fatalf("granting the application role the read a unit migration grants: %v", err)
	}
	t.Cleanup(func() {
		if _, err := e.owner.Exec(context.Background(), `DROP TABLE IF EXISTS ext.`+extProbeTable); err != nil {
			t.Errorf("dropping the probe table: %v", err)
		}
	})
	present := ids.NewV7()
	if _, err := e.owner.Exec(ctx,
		`INSERT INTO ext.`+extProbeTable+` (id) VALUES ($1)`, present); err != nil {
		t.Fatalf("seeding the row the probe must find: %v", err)
	}

	if err := RegisterExtensionKinds([]ExtensionKind{{
		Verb: "probe_forget", TargetTable: extProbeTable,
		RbacObject: "ext_probe_row", RbacAction: principal.ActionDelete,
	}}); err != nil {
		t.Fatalf("registering the kind: %v", err)
	}
	t.Cleanup(func() {
		if err := RegisterExtensionKinds(nil); err != nil {
			t.Errorf("clearing the registered kinds: %v", err)
		}
	})

	opCtx := principal.WithWorkspaceID(ctx, e.ws)
	for name, probe := range map[string]struct {
		id   ids.UUID
		want bool
	}{
		"a row that is there":     {present, true},
		"a row that is not there": {ids.NewV7(), false},
	} {
		var got bool
		if err := withProbeTx(opCtx, t, e, func(tx pgx.Tx) error {
			// Through targetExists rather than ExtensionTargetExists, so what is
			// proven is the path the INBOX takes: the classification says this
			// type is existence-probed, and the probe it dispatches to is the
			// registered one.
			exists, err := targetExists(context.Background(), tx, extProbeTable, probe.id)
			got = exists
			return err
		}); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got != probe.want {
			t.Errorf("%s: probe answered %t, want %t", name, got, probe.want)
		}
	}

	// And the classification that sends the inbox to that probe at all.
	if p := probeFor(extProbeTable); p != probeExistence {
		t.Errorf("probe classification = %v, want existence", p)
	}
}

func withProbeTx(ctx context.Context, t *testing.T, e *stagingEnv, fn func(pgx.Tx) error) error {
	t.Helper()
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if rollback := tx.Rollback(context.Background()); rollback != nil && rollback != pgx.ErrTxClosed {
			t.Errorf("rolling back the probe transaction: %v", rollback)
		}
	}()
	return fn(tx)
}
