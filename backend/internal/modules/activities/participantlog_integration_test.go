// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// Who a logged interaction names as our side.
//
// A rep who logs their own call is on it. An admin who imports another
// system's history is not: the source names its own author, and stamping the
// caller would put every imported meeting on the importer's worklist and give
// them a relationship with every imported contact.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type participantRow struct {
	user    *ids.UUID
	contact *ids.UUID
	role    string
}

func (e *sendEnv) participantsOf(t *testing.T, id ids.UUID) []participantRow {
	t.Helper()
	rows, err := e.owner.Query(context.Background(),
		`SELECT user_id, contact_id, role FROM activity_participant WHERE activity_id = $1`, id)
	if err != nil {
		t.Fatalf("reading participants: %v", err)
	}
	defer rows.Close()
	var out []participantRow
	for rows.Next() {
		var r participantRow
		if err := rows.Scan(&r.user, &r.contact, &r.role); err != nil {
			t.Fatalf("scanning participant: %v", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading participants: %v", err)
	}
	return out
}

func (e *sendEnv) logWithAuthor(t *testing.T, kind, sourceID string, direction *string, contact ids.UUID, author storekit.SourceAuthorInput) []participantRow {
	t.Helper()
	subject := "Review"
	in := LogActivityInput{
		Kind: kind, Subject: &subject, Direction: direction, Source: "manual",
		Links:  []ActivityLinkInput{{EntityType: linkEntityContact, EntityID: contact}},
		Author: author,
	}
	if author.AuthorID != nil || author.AuthorName != nil {
		system := "import-test"
		in.SourceSystem, in.SourceID = &system, &sourceID
	}
	activity, _, err := e.store(nil).LogActivity(e.as(principal.RowScopeAll), in)
	if err != nil {
		t.Fatalf("LogActivity: %v", err)
	}
	return e.participantsOf(t, ids.UUID(activity.Id))
}

func userRoles(rows []participantRow) map[ids.UUID]string {
	out := map[ids.UUID]string{}
	for _, r := range rows {
		if r.user != nil {
			out[*r.user] = r.role
		}
	}
	return out
}

func TestAHandLoggedMeetingStampsTheCallerAsOurSide(t *testing.T) {
	e := setupSend(t)
	contact := e.seedContact(t, "Counterparty")

	rows := e.logWithAuthor(t, "meeting", "", nil, contact, storekit.SourceAuthorInput{})

	users := userRoles(rows)
	if len(users) != 1 || users[e.rep] != "from" {
		t.Errorf("user participants = %v, want only the caller as from", users)
	}
	if len(rows) != 2 {
		t.Errorf("participants = %d, want the caller and the linked contact", len(rows))
	}
}

func TestAnImportedMeetingStampsItsAuthorSeatNotTheImporter(t *testing.T) {
	e := setupSend(t)
	contact := e.seedContact(t, "Counterparty")
	author := e.other

	rows := e.logWithAuthor(t, "meeting", "m-1", nil, contact, storekit.SourceAuthorInput{AuthorID: &author})
	users := userRoles(rows)
	if _, stamped := users[e.rep]; stamped {
		t.Errorf("the importing caller is a participant: %v", users)
	}
	if len(users) != 1 || users[author] != "from" {
		t.Errorf("user participants = %v, want only the author seat as from", users)
	}

	// Inbound keeps the direction rule: our side receives.
	inbound := "inbound"
	rows = e.logWithAuthor(t, "call", "c-1", &inbound, contact, storekit.SourceAuthorInput{AuthorID: &author})
	users = userRoles(rows)
	if len(users) != 1 || users[author] != "to" {
		t.Errorf("inbound user participants = %v, want only the author seat as to", users)
	}
}

func TestAnImportedMeetingNamingOnlyAnAuthorNameStampsNoUser(t *testing.T) {
	e := setupSend(t)
	contact := e.seedContact(t, "Counterparty")
	name := "Someone Elsewhere"

	rows := e.logWithAuthor(t, "meeting", "m-2", nil, contact, storekit.SourceAuthorInput{AuthorName: &name})

	if users := userRoles(rows); len(users) != 0 {
		t.Errorf("user participants = %v, want none — the author holds no seat here", users)
	}
	if len(rows) != 1 || rows[0].contact == nil || *rows[0].contact != contact || rows[0].role != "to" {
		t.Errorf("participants = %+v, want the linked contact alone, as to", rows)
	}
}
