// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package settings

// The ungated machinery reads, and what makes them ungated rather than
// unguarded.
//
// Their own file because they are their own rule: everything in store.go asks
// the entry's object gate, and these two deliberately do not — the licence is
// the entry's MachineryApplied declaration instead, refused here rather than
// agreed elsewhere. Reading them apart from the gated readers is what keeps a
// contributor from picking one by accident.

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ApplyTx reads a setting inside the caller's transaction WITHOUT the entry's
// read gate — for MACHINERY applying a workspace posture to its own write
// (the capture sink stamping a freshly captured row's audience), where the
// posture must bind whoever the acting principal happens to be: a posture a
// narrow principal could not read would simply not apply to what they
// capture, which is the opposite of a control. Never for a surface that
// ANSWERS the value to a caller — those go through Get/GetTx, whose gate is
// the only control on the un-RLS'd setting table. The restriction is
// enforced, not asked politely: only an entry declared MachineryApplied at
// Define time is readable here, so a convenient ungated read of any other
// setting refuses at the first test that exercises it.
func ApplyTx[T any](ctx context.Context, tx pgx.Tx, e *Entry[T]) (T, error) {
	var zero T
	if !e.machineryApplied {
		return zero, fmt.Errorf("settings: %s is not declared MachineryApplied — read it through Get/GetTx and its gate", e.Key())
	}
	raw, err := currentJSON(ctx, tx, e)
	if err != nil {
		return zero, err
	}
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		return zero, fmt.Errorf("settings: decoding %s: %w", e.Key(), err)
	}
	return out, nil
}

// ApplyManyTx is ApplyTx over several entries in ONE statement, which is the
// difference that matters: the values come from one snapshot.
//
// Two ApplyTx calls in the caller's transaction take two snapshots under READ
// COMMITTED, so a write that changes a related pair — a switch and the target
// it governs — can commit between them and be read half-applied. A policy built
// from two versions of the settings is one the operator never configured, and
// it runs until the next read.
//
// Same admission as ApplyTx and asked of every entry BEFORE the read: an entry
// not declared MachineryApplied refuses the whole batch rather than being
// skipped, because a partial answer here is the half-applied policy this exists
// to prevent, wearing a different cause.
//
// Answers raw values so entries of different types can share the statement, and
// resolves an absent row to the entry's declared default exactly as ApplyTx
// above does. The caller decodes, which is where the types are known.
func ApplyManyTx(ctx context.Context, tx pgx.Tx, defs ...Definition) (map[string]json.RawMessage, error) {
	keys := make([]string, 0, len(defs))
	for _, def := range defs {
		if !def.MachineryAppliedEntry() {
			return nil, fmt.Errorf(
				"settings: %s is not declared MachineryApplied — read it through Get/GetTx and its gate",
				def.Key())
		}
		keys = append(keys, def.Key())
	}
	rows, err := tx.Query(ctx, `SELECT key, value FROM setting WHERE key = ANY($1)`, keys)
	if err != nil {
		return nil, fmt.Errorf("settings: reading %d machinery settings: %w", len(keys), err)
	}
	defer rows.Close()
	out := make(map[string]json.RawMessage, len(defs))
	for rows.Next() {
		var key string
		var raw json.RawMessage
		if err := rows.Scan(&key, &raw); err != nil {
			return nil, fmt.Errorf("settings: scanning a machinery setting: %w", err)
		}
		out[key] = raw
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("settings: reading machinery settings: %w", err)
	}
	// An absent row resolves to the entry's declared default, the same answer
	// the single reader gives, so a caller cannot tell "unset" from "set to the
	// default" — which is right: they are the same posture.
	for _, def := range defs {
		if _, stored := out[def.Key()]; stored {
			continue
		}
		fallback, err := def.DefaultJSON()
		if err != nil {
			return nil, err
		}
		out[def.Key()] = fallback
	}
	return out, nil
}
