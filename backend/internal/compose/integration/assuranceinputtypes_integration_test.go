// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	_ "github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Over the whole event catalog, a new event starts unread only when the
// freshness consumer marks something for its type. inputChangesOf skips an
// undeclared type and ScopeNone. The expectation reads that table, not
// MarkingEventTypes, so a type dropped from the list fails here.
func TestAnEventStartsUnreadOnlyWhenTheDealCheckReadsItsType(t *testing.T) {
	e := Setup(t)
	ctx := principal.WithCorrelationID(principal.SystemActing(e.Admin(), "system:assurance-test"), ids.NewV7())
	consumed := func(eventType string) bool {
		change, ok := assurance.InputChangeOf(eventType)
		return ok && change.Scope != assurance.ScopeNone
	}
	emitted := map[ids.UUID]string{}
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		for _, eventType := range events.Types() {
			entity := ids.NewV7()
			if err := storekit.Emit(ctx, tx, ids.NewV7(), eventType, "deal", entity, nil); err != nil {
				return err
			}
			emitted[entity] = eventType
		}
		return nil
	}); err != nil {
		t.Fatalf("emitting one event of each type: %v", err)
	}
	if !slices.ContainsFunc(events.Types(), consumed) {
		t.Fatal("the freshness consumer reads no event type, so the case proves nothing")
	}
	for entity, eventType := range emitted {
		unread := e.WsScalar(t, `SELECT (NOT assurance_inputs_read)::text FROM event_outbox WHERE envelope->'entity'->>'id' = $1`, entity.String())
		if want := consumed(eventType); (unread == "true") != want {
			t.Errorf("a new %s event starts unread=%s, want %v", eventType, unread, want)
		}
	}
}
