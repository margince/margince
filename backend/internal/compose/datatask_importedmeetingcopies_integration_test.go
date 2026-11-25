// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// An import is folded onto the author's calendar meeting at the same instant
// under the same title. The calendar meeting gains the import's records. Any
// other import stays as it is.
func TestTheImportedCopiesTaskFoldsOnlyTheAuthorsCalendarCopy(t *testing.T) {
	for _, tc := range []struct {
		name    string
		subject string
		author  func(s seriesSplit) ids.UUID
		folded  bool
	}{
		{name: "the author's calendar holds it", subject: "Meeting " + weeklyUID,
			author: func(s seriesSplit) ids.UUID { return s.e.AdminUser }, folded: true},
		{name: "another title", subject: "Another meeting",
			author: func(s seriesSplit) ids.UUID { return s.e.AdminUser }},
		{name: "another author", subject: "Meeting " + weeklyUID,
			author: func(s seriesSplit) ids.UUID { return s.e.Rep1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newSeriesSplit(t)
			calendar := s.sync(t, weeklyUID, s.owner, splitBuyer)
			contact := s.e.SeedContact(t, "Client at Acme", nil)
			imported := importStoredCopy(t, s, contact, tc.author(s), tc.subject)
			ctx := principal.SystemActing(s.e.Admin(), "system:datatask-test")

			for range 2 {
				runCalendarDescriptionTask(ctx, t, s.e, importedMeetingCopiesDataTask(s.e.Pool))
			}

			live := s.e.WsCount(t, `SELECT count(*) FROM activity WHERE id = $1 AND archived_at IS NULL`, imported)
			if tc.folded != (live == 0) {
				t.Fatalf("the import is live=%v after the task, want folded=%v", live == 1, tc.folded)
			}
			linked := s.e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1 AND contact_id = $2`,
				calendar, contact)
			if tc.folded && linked != 1 {
				t.Errorf("the calendar meeting is filed under the import's contact %d times, want 1", linked)
			}
			if !tc.folded && linked != 0 {
				t.Errorf("an import left as it is filed the calendar meeting under its contact")
			}
			if got := s.e.WsCount(t, `SELECT count(*) FROM activity WHERE id = $1 AND archived_at IS NULL`, calendar); got != 1 {
				t.Error("the calendar meeting is no longer live")
			}
		})
	}
}

// importStoredCopy files a meeting through the declared importer's door as
// Rep1, naming author. It has no calendar identity, the shape such copies are
// stored in: the author's calendar held the identity first.
func importStoredCopy(t *testing.T, s seriesSplit, contact, author ids.UUID, subject string) ids.UUID {
	t.Helper()
	system, sourceID := "mirror:hubspot", "engagement-1"
	authorID := openapi_types.UUID(author)
	req := crmcontracts.CreateActivityRequest{
		Kind:           crmcontracts.CreateActivityRequestKindCreateActivityRequestKindMeeting,
		Subject:        &subject,
		OccurredAt:     &s.slot,
		SourceSystem:   &system,
		SourceId:       &sourceID,
		Source:         "hubspot_import:1",
		SourceAuthorId: &authorID,
	}
	// The link list is an anonymous contract struct, so it is filled the way a client sends it.
	if err := json.Unmarshal([]byte(`[{"entity_type":"contact","entity_id":"`+contact.String()+`"}]`), &req.Links); err != nil {
		t.Fatal(err)
	}
	in, err := activities.LogActivityInputFromImporter(req)
	if err != nil {
		t.Fatalf("mapping the import: %v", err)
	}
	ctx := principal.WithWorkspaceID(context.Background(), s.e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + s.e.Rep1.String(), UserID: s.e.Rep1, OnBehalfOf: s.e.Rep1,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"activity": {Create: true, Read: true, Update: true},
				"contact":  {Create: true, Read: true, Update: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
	created, _, err := activities.NewStore(InstallationDB(s.e.Pool)).LogActivity(ctx, in)
	if err != nil {
		t.Fatalf("importing the meeting: %v", err)
	}
	return ids.UUID(created.Id)
}
