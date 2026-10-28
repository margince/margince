// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestRealWorklistKeepsRecordedSilenceOptionalAndHonorsTheReadersChoice(t *testing.T) {
	e := integration.Setup(t)
	now := quietLaneNow
	seat := sharedMailbox(t, e, e.Rep1)
	owner := ids.From[ids.UserKind](e.Rep1)
	const address = "quiet@prospect.example"
	contact, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
		FullName: "Demo quiet contact", Source: "manual", OwnerID: &owner,
		Emails: []contacts.ContactEmailInput{{Email: address, EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	captured := []ids.UUID{}
	for index, from := range []string{address, seat} {
		to := address
		if index == 0 {
			to = seat
		}
		at := now.AddDate(0, 0, -61+index)
		subject := fmt.Sprintf("Demo exchange %d", index)
		raw := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMessage-ID: <optional-%d@prospect.example>\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nThank you for the discussion.\r\n", from, to, subject, at.Format(time.RFC1123Z), index)
		captureMailVia(t, e, e.Rep1, "gmail", []byte(raw))
		captured = append(captured, activityBySubject(t, e, subject))
	}
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	if err := e.DB().Tx(reader, func(tx pgx.Tx) error { return search.RecomputeEdgesForActivities(reader, tx, captured) }); err != nil {
		t.Fatal(err)
	}
	feed := newAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return now })
	handler := attention.NewHandlers(feed)
	read := func(ctx context.Context) crmcontracts.Worklist {
		t.Helper()
		response := httptest.NewRecorder()
		handler.GetWorklist(response, httptest.NewRequest(http.MethodGet, "/v1/worklist", nil).WithContext(ctx), crmcontracts.GetWorklistParams{})
		if response.Code != http.StatusOK {
			t.Fatalf("worklist status=%d: %s", response.Code, response.Body.String())
		}
		var result crmcontracts.Worklist
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	initial := read(reader)
	if initial.OptionalRelationships == nil || len(*initial.OptionalRelationships) != 1 || (*initial.OptionalRelationships)[0].Id != ids.UUID(contact.Id).String() {
		t.Fatalf("real scoped optional read lost the recorded contact: %+v", initial.OptionalRelationships)
	}
	for _, row := range initial.Queue {
		if row.Source == "relationship_decay" {
			t.Fatal("silence became responsible work")
		}
	}
	if err := e.Activities.PinWorklistRow(reader, activities.WorklistRowRef{Source: "relationship_decay", RowID: ids.UUID(contact.Id).String()}); err != nil {
		t.Fatal(err)
	}
	chosen := read(reader)
	found := false
	if chosen.Focus != nil {
		for _, row := range chosen.Focus.Items {
			if row.Source == "relationship_decay" {
				found = true
				if row.Level != (*initial.OptionalRelationships)[0].Level {
					t.Fatalf("choice became urgency: %+v", row)
				}
			}
		}
	}
	if !found || chosen.OptionalRelationships == nil || len(*chosen.OptionalRelationships) != 0 {
		t.Fatalf("real pin did not choose the contact: %+v", chosen.Focus)
	}
	other := read(e.As(e.Rep2, []ids.UUID{e.Team2}, integration.AdminPerms))
	for _, row := range other.Queue {
		if row.Source == "relationship_decay" {
			t.Fatal("reader choice leaked to another seat")
		}
	}
}
