// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func sourceAuthorization(connection ids.UUID, generation int, owner string) json.RawMessage {
	return json.RawMessage(`{"connection_id":"` + connection.String() + `","generation":` + strconv.Itoa(generation) + `,"owner_user_id":` + owner + `}`)
}

func TestAConnectorMarksInternalMailOnlyForItsOwnSource(t *testing.T) {
	seat, other := ids.NewV7(), ids.NewV7()
	seatBox, seatOtherBox, sharedA, sharedB := ids.NewV7(), ids.NewV7(), ids.NewV7(), ids.NewV7()
	personal := sourceAuthorization(seatBox, 2, `"`+seat.String()+`"`)
	shared := sourceAuthorization(sharedA, 1, `null`)
	for _, c := range []struct {
		name          string
		authorization json.RawMessage
		user, box     ids.UUID
		generation    int
		want          bool
	}{
		{"the owner's sync marks the owner's source", personal, seat, seatBox, 2, true},
		{"another seat's sync is refused", personal, other, seatBox, 2, false},
		{"the owner's sync of another connection is refused", personal, seat, seatOtherBox, 2, false},
		{"the owner's sync of a later account generation is refused", personal, seat, seatBox, 3, false},
		{"a sync for nobody cannot mark a seat's source", personal, ids.Nil, seatBox, 2, false},
		{"a shared mailbox's sync marks its own source", shared, ids.Nil, sharedA, 1, true},
		{"another shared mailbox's sync is refused", shared, ids.Nil, sharedB, 1, false},
		{"a seat's sync cannot mark a source with no owner", shared, seat, sharedA, 1, false},
		{"a run that names no connection is refused", shared, ids.Nil, ids.Nil, 1, false},
		{"neither the run nor the authorization names a connection", json.RawMessage(`{"generation":1,"owner_user_id":null}`), ids.Nil, ids.Nil, 1, false},
		{"an authorization with no generation is refused", json.RawMessage(`{"connection_id":"` + sharedA.String() + `","owner_user_id":null}`), ids.Nil, sharedA, 0, false},
		{"an unreadable authorization is refused", json.RawMessage(`{"owner_user_id":"not-a-seat"}`), seat, seatBox, 2, false},
	} {
		if got := authorizationNamesActor(c.authorization, c.user, c.box, c.generation); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
