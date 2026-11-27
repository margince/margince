// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package introductions

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ownScoped is asUser on own row scope, so the contact a seat does not own is
// one it cannot open.
func (e *introEnv) ownScoped(u ids.UUID) context.Context {
	actor, _ := principal.Actor(e.asUser(u))
	actor.Permissions.RowScope = principal.RowScopeOwn
	return principal.WithActor(e.asUser(u), actor)
}

// readableBySQL answers whether ReadableSQL admits row id for the reader.
func (e *introEnv) readableBySQL(reader context.Context, t *testing.T, id ids.UUID) bool {
	t.Helper()
	args := []any{id}
	readable, err := ReadableSQL(reader, "i", func(v any) int { args = append(args, v); return len(args) })
	if err != nil {
		t.Fatal(err)
	}
	var admitted bool
	err = e.store.db.Tx(reader, func(tx pgx.Tx) error {
		return tx.QueryRow(reader, `SELECT EXISTS (SELECT 1 FROM intro_request i WHERE i.id = $1 AND `+readable+`)`, args...).Scan(&admitted)
	})
	if err != nil {
		t.Fatal(err)
	}
	return admitted
}

// ReadableSQL and load are one rule in two spellings. Every origin, party and
// contact visibility must get the same answer from both.
func TestReadableSQLAgreesWithLoad(t *testing.T) {
	t.Parallel()
	e := setupIntro(t)
	askSeen, err := e.store.Create(e.asUser(e.requester), e.ask())
	if err != nil {
		t.Fatal(err)
	}
	unseenAsk := e.ask()
	unseenAsk.ContactID = e.unseen
	askUnseen, err := e.store.Create(e.asUser(e.requester), unseenAsk)
	if err != nil {
		t.Fatal(err)
	}
	recordedSeen := e.recorded(t)
	unseenRecording := e.recording()
	unseenRecording.ContactID = e.unseen
	recordedUnseen, err := e.store.Record(e.asUser(e.introducer), unseenRecording)
	if err != nil {
		t.Fatal(err)
	}
	// Private to the stranger now, so even a party to its introductions cannot open it.
	if _, err := e.owner.Exec(context.Background(), `UPDATE contact SET visibility = 'owner' WHERE id = $1`, e.unseen); err != nil {
		t.Fatal(err)
	}
	rows := map[string]ids.UUID{
		"ask, contact the requester owns": askSeen, "ask, contact a stranger owns": askUnseen,
		"recorded, contact the requester owns": recordedSeen, "recorded, contact a stranger owns": recordedUnseen,
	}
	for name, row := range rows {
		for _, seat := range []ids.UUID{e.requester, e.introducer, e.stranger} {
			reader := e.ownScoped(seat)
			_, err := e.store.Get(reader, row)
			if err != nil && !errors.Is(err, apperrors.ErrNotFound) {
				t.Fatal(err)
			}
			if bySQL := e.readableBySQL(reader, t, row); bySQL != (err == nil) {
				t.Errorf("%s, read by %s: ReadableSQL admits it %t, load %t", name, seat, bySQL, err == nil)
			}
		}
	}
}
