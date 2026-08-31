// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Running the replay gate: resolving the record a recorded body is about, the
// other records it names, and whether the caller may still see any of them.
//
// Split from replayscope.go, which is the TABLE — what governs each route, and
// why anything it does not re-check is not re-checked. That file is read when
// adding a route; this one when changing what a probe does.

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// replayProbe answers whether the caller may still see one record, for the
// bodies whose scope rule lives inside a module rather than in a row-scoped
// table. Compose injects them at the composition root, so this package borrows
// the module's own rule instead of keeping a second copy that could drift.
type replayProbe func(ctx context.Context, id ids.UUID) error

// ensureReplayVisible re-runs, against the caller as they are NOW, whichever
// gates govern the body about to be replayed. Anything it cannot resolve fails
// CLOSED: the middleware cannot show the caller may still see what it is
// handing back, and serving it on the strength of a parse failure is the one
// outcome this exists to prevent.
func ensureReplayVisible(ctx context.Context, pool *pgxpool.Pool, probes map[string]replayProbe, route, body string) error {
	target, replayable := replayableOperations[route]
	if !replayable {
		// The middleware only claims keys for routes in this table, so this
		// is unreachable; if it is ever reached, the unclassified case is
		// exactly the one that must not pay out.
		return apperrors.ErrNotFound
	}

	if target.table == "" && target.tableField == "" && target.moduleProbe == "" {
		// No primary record, but a body can still point at one.
		return ensureCompanionsVisible(ctx, pool, target, body)
	}

	if target.moduleProbe != "" {
		probe, wired := probes[target.moduleProbe]
		if !wired {
			// An unwired probe cannot show the caller may still see this, and
			// the composition root is the only place that could have wired it.
			return apperrors.ErrNotFound
		}
		id, err := replayRecordID(ctx, target, body)
		if err != nil {
			return err
		}
		if err := probe(ctx, id); err != nil {
			return err
		}
		return ensureCompanionsVisible(ctx, pool, target, body)
	}

	if err := ensureCompanionsVisible(ctx, pool, target, body); err != nil {
		return err
	}

	table, err := replayTableFor(target, body)
	if err != nil {
		return err
	}
	id, err := replayRecordID(ctx, target, body)
	if err != nil {
		return err
	}
	return database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		switch table {
		case tableActivity:
			return auth.EnsureActivityContentVisibleLive(ctx, tx, id)
		case tableSignal:
			// A signal has no owner column but is scoped through its subject
			// entity; "no owner_id" is never on its own a reason to skip.
			return auth.EnsureSignalVisibleLive(ctx, tx, id)
		case tableScheduledSend:
			// A scheduled message is the SENDER's own — an unsent body and its
			// blind-copy list are not workspace-readable the way a sent
			// activity is — so the probe is the same scheduled_by predicate the
			// store reads with, not the generic row-scope clause. It also has
			// no archived_at, which the generic probe requires.
			return ensureScheduledSendVisibleLive(ctx, tx, id)
		}
		// LIVE, not merely visible. The recorded body is a frozen snapshot the
		// store itself would no longer serve: Art. 17 erasure anonymizes the
		// person row in place, stamps archived_at and leaves owner_id alone, so
		// a plain visibility probe still answers "yours" and the middleware
		// would hand back the pre-erasure names, e-mails and phone numbers that
		// every live read path now refuses. EnsureVisibleLive also declines to
		// skip the existence half for an unbounded actor, which is the same
		// hole one role wider.
		//
		// auth rejects any name outside its closed row-scoped set, so an
		// unexpected value refuses the replay rather than reaching SQL.
		return auth.EnsureVisibleLive(ctx, tx, table, id)
	})
}

// ensureScheduledSendVisibleLive refuses a replay whose scheduled message no
// longer belongs to the caller. Existence-hiding, like every other row scope:
// somebody else's message is not found rather than forbidden.
func ensureScheduledSendVisibleLive(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	actor, err := storekit.Actor(ctx)
	if err != nil {
		return err
	}
	var visible bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM scheduled_send WHERE id = $1 AND scheduled_by = $2)`,
		id, actor.UserID).Scan(&visible); err != nil {
		return err
	}
	if !visible {
		return apperrors.ErrNotFound
	}
	return nil
}

// bodyHasField reports whether the recorded body carries a non-null field —
// the discriminator between two response shapes one route can answer.
func bodyHasField(body, field string) bool {
	if field == "" {
		return false
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &probe); err != nil {
		return false
	}
	raw, ok := probe[field]
	return ok && string(raw) != jsonNull
}

// replayRecordID resolves the record whose scope governs this body: from the
// recorded body where it names its own record, or from the route parameter
// naming its parent where the body is a projection that omits it.
func replayRecordID(ctx context.Context, target replayTarget, body string) (ids.UUID, error) {
	if target.pathParam == "" {
		return recordIDAt(body, target.idPath)
	}
	raw := chi.RouteContext(ctx).URLParam(target.pathParam)
	id, err := ids.Parse(raw)
	if err != nil {
		return ids.UUID{}, apperrors.ErrNotFound
	}
	return id, nil
}

// recordIDAt walks a dotted path to the record id in a recorded body.
func recordIDAt(body, path string) (ids.UUID, error) {
	raw, err := stringAt(body, path)
	if err != nil {
		return ids.UUID{}, err
	}
	id, err := ids.Parse(raw)
	if err != nil {
		return ids.UUID{}, apperrors.ErrNotFound
	}
	return id, nil
}

// stringAt walks a dotted path to a string in a recorded body. Every miss is
// ErrNotFound rather than a distinct error: whichever way the body surprised
// us, the middleware cannot prove the caller may still see what it is about to
// hand back, and that is the client-visible fact.
func stringAt(body, path string) (string, error) {
	var node any
	if err := json.Unmarshal([]byte(body), &node); err != nil {
		return "", apperrors.ErrNotFound
	}
	for _, segment := range strings.Split(path, ".") {
		object, ok := node.(map[string]any)
		if !ok {
			return "", apperrors.ErrNotFound
		}
		if node, ok = object[segment]; !ok {
			return "", apperrors.ErrNotFound
		}
	}
	text, ok := node.(string)
	if !ok {
		return "", apperrors.ErrNotFound
	}
	return text, nil
}

// replayTableFor resolves which table this body's record lives in: the entry's
// own, the one the body names for a polymorphic reference, or the alternate a
// route that answers two shapes says this body is. The marker is a field only
// the alternate carries, so the choice is read off the recorded body rather
// than guessed from the route.
func replayTableFor(target replayTarget, body string) (string, error) {
	table := target.table
	if target.tableField != "" {
		resolved, err := stringAt(body, target.tableField)
		if err != nil {
			return "", err
		}
		table = resolved
	}
	if target.altTable != "" && bodyHasField(body, target.altMarker) {
		table = target.altTable
	}
	return table, nil
}

// ensureCompanionsVisible re-checks every OTHER record the body names.
//
// An absent or null field names nothing and is skipped: these are optional by
// contract, and a person captured with no employer carries no organization id.
// A field that is present and unreadable as an id refuses, on the same terms as
// the primary — the middleware cannot show the caller may still see what it is
// handing back.
func ensureCompanionsVisible(ctx context.Context, pool *pgxpool.Pool, target replayTarget, body string) error {
	for _, companion := range target.companions {
		if !bodyHasField(body, companion.idPath) {
			continue
		}
		id, err := recordIDAt(body, companion.idPath)
		if err != nil {
			return err
		}
		if err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
			return auth.EnsureVisibleLive(ctx, tx, companion.table, id)
		}); err != nil {
			return err
		}
	}
	return nil
}
