// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// Who a logged meeting names as its host.
//
// A rep who logs a meeting with no host named held it. An admin who imports
// another system's meetings did not: the host is the source author's seat, or
// nobody, and the import must not land in the importer's own upcoming meetings.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (e *sendEnv) logMeeting(t *testing.T, sourceID string, host *ids.UserID, author storekit.SourceAuthorInput) ids.UUID {
	t.Helper()
	subject := "Review"
	in := LogActivityInput{Kind: KindMeeting, Subject: &subject, Source: "manual", HostUserID: host, Author: author}
	if author.AuthorID != nil || author.AuthorName != nil {
		system := "import-test"
		in.SourceSystem, in.SourceID = &system, &sourceID
	}
	activity, _, err := e.store(nil).LogActivity(e.as(principal.RowScopeAll), in)
	if err != nil {
		t.Fatalf("LogActivity: %v", err)
	}
	return ids.UUID(activity.Id)
}

func (e *sendEnv) hostOf(t *testing.T, id ids.UUID) (*ids.UUID, bool) {
	t.Helper()
	var host *ids.UUID
	var claims bool
	if err := e.owner.QueryRow(context.Background(),
		`SELECT host_user_id, claims_host_slot FROM activity WHERE id = $1`, id).Scan(&host, &claims); err != nil {
		t.Fatalf("reading host: %v", err)
	}
	return host, claims
}

func (e *sendEnv) onMeetingsOf(t *testing.T, user ids.UUID) map[ids.UUID]bool {
	t.Helper()
	reader := ids.From[ids.UserKind](user)
	rows, _, err := e.store(nil).ListActivities(e.as(principal.RowScopeAll), ListActivitiesInput{OnMeetingOf: &reader})
	if err != nil {
		t.Fatalf("ListActivities: %v", err)
	}
	out := map[ids.UUID]bool{}
	for _, a := range rows {
		out[ids.UUID(a.Id)] = true
	}
	return out
}

func TestAHandLoggedMeetingIsHostedByTheCaller(t *testing.T) {
	e := setupSend(t)

	id := e.logMeeting(t, "", nil, storekit.SourceAuthorInput{})

	if host, claims := e.hostOf(t, id); host == nil || *host != e.rep || claims {
		t.Errorf("host = %v, claims slot = %v; want the caller, no slot claimed", host, claims)
	}
	if !e.onMeetingsOf(t, e.rep)[id] {
		t.Error("the caller's own logged meeting is missing from their meetings")
	}
}

func TestAnImportedMeetingIsHostedByItsAuthorSeatNotTheImporter(t *testing.T) {
	e := setupSend(t)
	author := e.other

	id := e.logMeeting(t, "m-1", nil, storekit.SourceAuthorInput{AuthorID: &author})

	if host, claims := e.hostOf(t, id); host == nil || *host != author || claims {
		t.Errorf("host = %v, claims slot = %v; want the author seat, no slot claimed", host, claims)
	}
	if e.onMeetingsOf(t, e.rep)[id] {
		t.Error("the imported meeting is in the importer's meetings")
	}
	if !e.onMeetingsOf(t, author)[id] {
		t.Error("the imported meeting is missing from its author's meetings")
	}
	// Two imported meetings in the same hour for the same author must both land:
	// an import claims no slot, so the double-booking guard does not apply.
	again := e.logMeeting(t, "m-1b", nil, storekit.SourceAuthorInput{AuthorID: &author})
	if again == id {
		t.Error("the second import replayed the first")
	}
}

func TestAnImportedMeetingNamingOnlyAnAuthorNameHasNoHost(t *testing.T) {
	e := setupSend(t)
	name := "Someone Elsewhere"

	id := e.logMeeting(t, "m-2", nil, storekit.SourceAuthorInput{AuthorName: &name})

	host, claims := e.hostOf(t, id)
	if host != nil {
		t.Errorf("host = %v, want none — the author holds no seat here", *host)
	}
	if claims {
		t.Error("the imported meeting claims a host slot")
	}
	if e.onMeetingsOf(t, e.rep)[id] {
		t.Error("the imported meeting is in the importer's meetings")
	}
}

func TestAnExplicitMeetingHostIsKept(t *testing.T) {
	e := setupSend(t)
	other := ids.From[ids.UserKind](e.other)
	name := "Someone Elsewhere"

	logged := e.logMeeting(t, "", &other, storekit.SourceAuthorInput{})
	imported := e.logMeeting(t, "m-3", &other, storekit.SourceAuthorInput{AuthorName: &name})

	for label, id := range map[string]ids.UUID{"logged": logged, "imported": imported} {
		if host, _ := e.hostOf(t, id); host == nil || *host != e.other {
			t.Errorf("%s: host = %v, want the named host", label, host)
		}
	}
}
