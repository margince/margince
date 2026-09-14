// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package notices

import (
	"reflect"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestNotificationReplayKeepsTheOriginalActorAndOccurrence(t *testing.T) {
	e := setupNotices(t)
	origin := &crmcontracts.NoticeOrigin{EventId: openapi_types.UUID(ids.NewV7()), ActorType: "human", ActorId: "human:" + e.other.String(), OccurredAt: time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC)}
	in := NewNotice{Recipient: e.recipient, Kind: "automation", Subject: "Stage changed", Origin: origin, DedupeKey: origin.EventId.String()}
	first, err := e.store.insertNotice(e.engineCtx(), in, nil)
	if err != nil {
		t.Fatal(err)
	}
	changed := *origin
	changed.ActorId = "system:retry"
	in.Origin = &changed
	again, err := e.store.insertNotice(e.engineCtx(), in, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, again) {
		t.Fatalf("replay changed the notice: first=%+v replay=%+v", first, again)
	}
	rows, err := e.store.UnreadFor(e.asUser(e.recipient), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || !reflect.DeepEqual(rows[0].Origin, origin) {
		t.Fatalf("original change lost: %+v", rows)
	}
	hidden, err := e.store.UnreadFor(e.asUser(e.other), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hidden) != 0 {
		t.Fatal("source actor may not read another recipient's notice")
	}
}
