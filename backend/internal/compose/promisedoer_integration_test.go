// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// promiserOwnRecordsPerms is a colleague who may edit only the records they
// own, so a contact a teammate owns is read-only for them.
var promiserOwnRecordsPerms = principal.Permissions{
	RoleKeys: []string{"rep"},
	Objects: map[string]principal.ObjectGrant{
		"contact":               {Read: true, Update: true},
		"company":               {Read: true},
		"activity":              {Create: true, Read: true, Update: true},
		"installation_settings": {Read: true},
	},
	RowScope: principal.RowScopeOwn,
}

// A promise a colleague made in their own mail is theirs to keep. That holds
// when a teammate owns the customer and the record is read-only for them. The
// task reaches the promiser's own lane, and never the contact owner's.
func TestAPromiseInMyOwnMailIsMineWhoeverOwnsTheContact(t *testing.T) {
	e, reply := seedMailedPromise(t, "active")
	ines := ids.MustParse(e.WsScalar(t, `SELECT id::text FROM contact WHERE full_name = 'Ines Huber'`))
	promiser := e.As(e.Rep2, []ids.UUID{e.Team1}, promiserOwnRecordsPerms)
	contactOwner := e.As(e.Rep1, []ids.UUID{e.Team1}, promiserOwnRecordsPerms)

	if _, err := e.Contacts.GetContact(promiser, ids.From[ids.ContactKind](ines), storekit.LiveOnly); err != nil {
		t.Fatalf("the promiser cannot read the contact (err %v); the case needs it readable", err)
	}
	rename := "Ines M. Huber"
	if _, err := e.Contacts.UpdateContact(promiser, ids.From[ids.ContactKind](ines),
		contacts.UpdateContactInput{FullName: &rename}); !errors.Is(err, apperrors.ErrPermissionDenied) &&
		!errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("the promiser edited a contact a colleague owns (err %v); the case needs it read-only", err)
	}

	if err := runSignalPass(t, e, reply); err != nil {
		t.Fatalf("the signal pass: %v", err)
	}
	if got := e.WsScalar(t, `SELECT coalesce(string_agg(t.assignee_id::text, ','), '')
		FROM conversation_claim c JOIN activity t ON t.id = c.task_activity_id
		WHERE c.kind = 'commitment_ours' AND c.contact_id = $1`, ines); got != e.Rep2.String() {
		t.Fatalf("the promise on the contact is kept by %q, want the colleague who made it %s", got, e.Rep2)
	}

	// The lane reads as of the mail's own clock, so a skewed fixture clock
	// cannot age the promise out of its 30-day window.
	sent := e.WsScalar(t, `SELECT to_char(occurred_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM activity WHERE kind = 'email'`)
	now, err := time.Parse(time.RFC3339, sent)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(48 * time.Hour)
	until := now.AddDate(0, 1, 0)
	tasks := attentionTasks{store: e.Activities}
	for name, c := range map[string]struct {
		reader context.Context
		want   int
	}{
		"the promiser":      {promiser, 1},
		"the contact owner": {contactOwner, 0},
	} {
		got, err := tasks.CountOpenForViewer(c.reader, now, until, attention.TasksMine, ids.Nil, nil)
		if err != nil {
			t.Fatalf("%s's task lane: %v", name, err)
		}
		if got != c.want {
			t.Errorf("%s's own task lane counts %d, want %d", name, got, c.want)
		}
	}
}
