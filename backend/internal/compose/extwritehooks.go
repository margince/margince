// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
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

func runWriteHook(ctx context.Context, tx pgx.Tx, e extension.Extension, hook extension.WriteHook, w extension.Write) error {
	via := "hook/write/" + w.Entity + "." + w.Action
	rt := joinedRuntime{callRuntime: runtimeFor(ctx, string(e.Name), string(e.Version), via, boundExtensionRuntime()), tx: tx}
	defer rt.release()
	return hook.Handle(ctx, rt, w)
}

// joinedRuntime is the Runtime of a unit running inside a core write. Its Tx
// joins that write's transaction, so the unit's rows commit or roll back with
// core's.
type joinedRuntime struct {
	*callRuntime
	tx pgx.Tx
}

func (r joinedRuntime) Tx(ctx context.Context, fn func(ctx context.Context, tx extension.Tx) error) error {
	return r.runTx(ctx, func(_ context.Context, body func(pgx.Tx) error) error {
		return body(r.tx)
	}, fn)
}
