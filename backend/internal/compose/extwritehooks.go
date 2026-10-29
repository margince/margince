// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/pkg/extension"
)

// dispatchWriteHooks is the storekit.WriteHook compose installs: every
// composed unit's hooks on one audited write, in unit order. A unit's own
// ledger rows (ext_*) run none, so a hook's writes are not hooked again.
func dispatchWriteHooks(ctx context.Context, tx pgx.Tx, w storekit.AuditedWrite) error {
	if strings.HasPrefix(w.EntityType, "ext_") {
		return nil
	}
	write := extension.Write{
		Entity: w.EntityType, Action: w.Action, ID: w.EntityID.String(), Before: w.Before, After: w.After,
	}
	for _, e := range ComposedExtensions() {
		for _, hook := range e.WriteHooks {
			if hook.Entity != w.EntityType || !slices.Contains(hook.Actions, w.Action) {
				continue
			}
			if err := runWriteHook(ctx, tx, e, hook, write); err != nil {
				return unitRefusal(err)
			}
		}
	}
	return nil
}

// runWriteHook runs one hook UNATTENDED. A write may run as the system
// principal, which every RBAC check passes. So the hook gets no Core() door,
// and works on its own tables through the joined transaction.
func runWriteHook(ctx context.Context, tx pgx.Tx, e extension.Extension, hook extension.WriteHook, w extension.Write) error {
	via := "hook/write/" + w.Entity + "." + w.Action
	rt := joinedRuntime{callRuntime: unattendedRuntimeFor(ctx, string(e.Name), string(e.Version), via, boundExtensionRuntime()), tx: tx}
	defer rt.release()
	return hook.Handle(ctx, rt, w)
}

// joinedRuntime is the Runtime of a unit running inside a core write. Its Tx
// joins that write's transaction, so the unit's rows commit or roll back with
// core's. What would take a second connection under that transaction's locks
// is refused.
type joinedRuntime struct {
	*callRuntime
	tx pgx.Tx
}

// Tx runs fn on the joined write and queues its export entries there too. A
// second connection under that write could wait on its locks forever.
func (r joinedRuntime) Tx(ctx context.Context, fn func(ctx context.Context, tx extension.Tx) error) error {
	ctx, namespace, err := r.enterUnitTx(ctx)
	if err != nil {
		return err
	}
	defer r.leaveTx()
	audit := &readAudit{}
	if err := fn(ctx, r.txOn(r.tx, namespace, audit)); err != nil {
		return err
	}
	return audit.flushOn(ctx, r.tx)
}

var errInsideWrite = fmt.Errorf("%w: a write hook runs inside core's write; ingest and sync from a job or a tool instead", extension.ErrForbidden)

func (joinedRuntime) Ingest(context.Context, extension.UserID, extension.Record) (extension.Result, error) {
	return extension.Result{}, errInsideWrite
}

func (joinedRuntime) IngestBatch(context.Context, extension.UserID, extension.SourceName, []extension.Record, extension.BatchPosition) ([]extension.Result, error) {
	return nil, errInsideWrite
}

func (joinedRuntime) SyncNow(context.Context, extension.JobName) error {
	return errInsideWrite
}
