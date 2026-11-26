// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// chatRow seeds a connector-captured chat message in a thread and returns its id.
func (e *loadEnv) chatRow(t *testing.T, audience string, importedBy ...ids.UUID) ids.UUID {
	t.Helper()
	row := ids.NewV7()
	e.exec(t, `INSERT INTO activity (id, kind, channel_provider, direction, subject, occurred_at, audience, thread_key, source, captured_by)
		VALUES ($1, 'message', 'whatsapp', 'inbound', 'hello', now(), $2, 'chat-thread', 'seed', 'connector:whatsapp')`, row, audience)
	for _, seat := range importedBy {
		e.exec(t, `INSERT INTO capture_import (activity_id, user_id) VALUES ($1, $2)`, row, seat)
	}
	return row
}

func TestAListedChatRowSaysWhetherTheCallerMayChangeItsThreadAudience(t *testing.T) {
	e := setupLoad(t)
	mine := e.chatRow(t, "workspace", e.rep)
	theirs := e.chatRow(t, "workspace", e.other)
	orphan := e.chatRow(t, "workspace")
	hidden := e.chatRow(t, "participants", e.other)

	list := func(seat ids.UUID) map[ids.UUID]struct{ imported, yours *bool } {
		rows, _, err := handledStore(e).ListActivities(e.followUpReaderAs(seat), ListActivitiesInput{})
		if err != nil {
			t.Fatalf("listing as %s: %v", seat, err)
		}
		out := map[ids.UUID]struct{ imported, yours *bool }{}
		for _, r := range rows {
			out[ids.UUID(r.Id)] = struct{ imported, yours *bool }{r.MailboxImported, r.ThreadAudienceYours}
		}
		return out
	}
	want := func(got struct{ imported, yours *bool }, imported, yours bool) bool {
		return got.imported != nil && *got.imported == imported && got.yours != nil && *got.yours == yours
	}

	asRep := list(e.rep)
	if !want(asRep[mine], true, true) {
		t.Errorf("the importer's own row reads %+v, want imported and yours", asRep[mine])
	}
	if !want(asRep[theirs], true, false) {
		t.Errorf("a colleague's import reads %+v, want imported and not yours", asRep[theirs])
	}
	if !want(asRep[orphan], false, false) {
		t.Errorf("a row whose importer left reads %+v, want neither", asRep[orphan])
	}
	if got, listed := asRep[hidden]; !listed || got.imported != nil || got.yours != nil {
		t.Errorf("a withheld row reads %+v (listed %v), want neither field", got, listed)
	}
	if !want(list(e.other)[theirs], true, true) {
		t.Error("the colleague who imported the row is not told it is theirs")
	}
}
