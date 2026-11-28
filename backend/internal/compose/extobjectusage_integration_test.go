// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A unit's object calls over real Postgres: the running count, the quota it
// holds the unit to, and the audit row of each call.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose/extcapability"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/pkg/extension"
)

func objectsInDatabase(e *integration.Env, unit string) extcapability.Objects {
	return extcapability.Objects{
		Store: blobstore.NewMemory(), Ledger: extcapability.PoolLedger(e.Pool), Unit: unit, Workspace: extcapability.StoredWorkspace(e.Pool),
		Gate: func(ctx context.Context) (context.Context, error) { return ctx, nil },
	}
}

func TestEachObjectCallIsCountedAndAudited(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	objects := objectsInDatabase(e, "summaries")
	for i, call := range []func() error{
		func() error { return objects.Put(ctx, "a", []byte("abc")) },
		func() error { return objects.Put(ctx, "a", []byte("abcde")) },
		func() error { return objects.Put(ctx, "b", []byte("xy")) },
		func() error { return objects.Delete(ctx, "b") },
		func() error { return objects.Delete(ctx, "never-stored") },
	} {
		if err := call(); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if got := e.WsScalar(t, `SELECT objects || '/' || bytes FROM extension_object_usage WHERE unit = 'summaries'`); got != "1/5" {
		t.Errorf("usage = %s objects/bytes, want 1/5", got)
	}
	audited := e.WsCount(t, `SELECT count(*) FROM audit_log a JOIN extension_object_usage u ON u.id = a.entity_id
		WHERE a.entity_type = 'extension_object_usage' AND u.unit = 'summaries' AND a.evidence->>'object_unit' = 'summaries'
		  AND a.evidence->>'object' IN ('a', 'b', 'never-stored')`)
	if audited != 4 {
		t.Errorf("four object calls and a no-op delete left %d audit rows naming the unit and the object, want 4", audited)
	}
}

func TestAUnitAtItsCountCapMayReplaceButNotAdd(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	objects := objectsInDatabase(e, "summaries")
	if err := objects.Put(ctx, "kept", []byte("abc")); err != nil {
		t.Fatal(err)
	}
	e.WsExec(t, `UPDATE extension_object_usage SET objects = $1 WHERE unit = 'summaries'`, extension.MaxObjectsPerUnit)
	if err := objects.Put(ctx, "new", []byte("x")); !errors.Is(err, extension.ErrObjectQuotaReached) {
		t.Errorf("a new object at the count cap: err = %v, want ErrObjectQuotaReached", err)
	}
	if err := objects.Put(ctx, "kept", []byte("abcd")); err != nil {
		t.Errorf("replacing an object at the count cap: %v", err)
	}
	if err := objectsInDatabase(e, "other").Put(ctx, "new", []byte("x")); err != nil {
		t.Errorf("another unit, under its own cap: %v", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'extension_object_usage' AND evidence->>'object' = 'new'
		AND evidence->>'object_unit' = 'summaries'`); n != 0 {
		t.Errorf("a refused Put left %d audit rows", n)
	}
}
