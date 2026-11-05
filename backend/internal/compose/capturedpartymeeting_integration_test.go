// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture/gcal"
	"github.com/margince/margince/backend/internal/modules/capture/graphcal"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestCapturedPartiesMeetingNeedsTheCapturingSeatsAttendance(t *testing.T) {
	for _, tc := range []struct {
		name, ownerResponse, guestResponse, guest, visibility string
		organizes, canceled, want                             bool
	}{
		{name: "organized business meeting", guest: "bob@buyer.test", organizes: true, want: true},
		{name: "accepted business meeting", ownerResponse: "accepted", guest: "bob@buyer.test", want: true},
		{name: "unanswered invitation", ownerResponse: "needsAction", guest: "bob@buyer.test"},
		{name: "tentative invitation", ownerResponse: "tentative", guest: "bob@buyer.test"},
		{name: "declined invitation", ownerResponse: "declined", guest: "bob@buyer.test"},
		{name: "declining guest", ownerResponse: "accepted", guestResponse: "declined", guest: "bob@buyer.test"},
		{name: "canceled meeting", ownerResponse: "accepted", guest: "bob@buyer.test", canceled: true},
		{name: "consumer guest", ownerResponse: "accepted", guest: "bob@gmail.com"},
		{name: "private meeting", ownerResponse: "accepted", guest: "bob@buyer.test", visibility: "private"},
		{name: "confidential meeting", ownerResponse: "accepted", guest: "bob@buyer.test", visibility: "confidential"},
		{name: "public meeting", ownerResponse: "accepted", guest: "bob@buyer.test", visibility: "public", want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := integration.Setup(t)
			e.GrantRole(t, e.AdminUser, "admin")
			seedCalendarConnection(t, e)
			organizer := "host@buyer.test"
			if tc.organizes {
				organizer = backfillOwner
			}
			guestResponse := tc.guestResponse
			if guestResponse == "" {
				guestResponse = "accepted"
			}
			status := "confirmed"
			if tc.canceled {
				status = "cancelled"
			}
			event := map[string]any{"id": ids.NewV7().String(), "status": status, "summary": "Product requirements", "visibility": tc.visibility, "start": map[string]string{"dateTime": "2026-10-05T10:00:00Z"}, "end": map[string]string{"dateTime": "2026-10-05T11:00:00Z"}, "organizer": map[string]any{"email": organizer, "self": tc.organizes}, "attendees": []map[string]any{
				{"email": backfillOwner, "responseStatus": tc.ownerResponse, "self": true, "organizer": tc.organizes},
				{"email": tc.guest, "displayName": "Bob Buyer", "responseStatus": guestResponse},
			}}
			raw, err := json.Marshal(event)
			if err != nil {
				t.Fatal(err)
			}
			rec, err := gcal.RecordOf(raw, backfillOwner)
			if err != nil {
				if !tc.want && errors.Is(err, connector.ErrSkip) {
					return
				}
				t.Fatal(err)
			}
			sink := newCaptureSink(e.Pool, CaptureConfig{})
			ref, err := sink.Upsert(calendarOwnerCtx(e, e.AdminUser), rec)
			if err != nil {
				t.Fatal(err)
			}
			count := e.WsCount(t, `SELECT count(*) FROM contact_email WHERE lower(email)=$1 AND archived_at IS NULL`, tc.guest)
			if (count == 1) != tc.want {
				t.Fatalf("guest contacts=%d, wanted admission=%v", count, tc.want)
			}
			if !tc.organizes {
				wantHost := tc.ownerResponse == "accepted" && !tc.canceled && (tc.visibility == "" || tc.visibility == "public")
				hostCount := e.WsCount(t, `SELECT count(*) FROM contact_email WHERE lower(email)='host@buyer.test' AND archived_at IS NULL`)
				if (hostCount == 1) != wantHost {
					t.Fatalf("host contacts=%d, wanted admission=%v", hostCount, wantHost)
				}
			}
			if tc.want && e.WsCount(t, `SELECT count(*) FROM activity_link l JOIN contact_email ce ON ce.contact_id=l.contact_id WHERE l.activity_id=$1 AND lower(ce.email)=$2`, ref.ID, tc.guest) != 1 {
				t.Fatal("meeting was not filed on its admitted guest")
			}
		})
	}
}

func TestCapturedPartiesHonorsGraphCalendarSensitivity(t *testing.T) {
	for _, sensitivity := range []string{"normal", "private", "confidential", "personal"} {
		t.Run(sensitivity, func(t *testing.T) {
			e := integration.Setup(t)
			e.GrantRole(t, e.AdminUser, "admin")
			owner := seatAddress(t, e, e.AdminUser)
			e.WsExec(t, `INSERT INTO capture_connection(provider,user_id,status,account_label) VALUES('graphcal',$1,'connected',$2)`, e.AdminUser, owner)
			event := map[string]any{
				"id": ids.NewV7().String(), "subject": "Product requirements", "sensitivity": sensitivity,
				"start":          map[string]string{"dateTime": "2026-10-05T10:00:00", "timeZone": "UTC"},
				"end":            map[string]string{"dateTime": "2026-10-05T11:00:00", "timeZone": "UTC"},
				"responseStatus": map[string]string{"response": "accepted"},
				"organizer":      map[string]any{"emailAddress": map[string]string{"address": "host@buyer.test"}},
				"attendees": []map[string]any{
					{"type": "required", "emailAddress": map[string]string{"address": owner}, "status": map[string]string{"response": "accepted"}},
					{"type": "required", "emailAddress": map[string]string{"address": "bob@buyer.test", "name": "Bob Buyer"}, "status": map[string]string{"response": "accepted"}},
				},
			}
			raw, err := json.Marshal(event)
			if err != nil {
				t.Fatal(err)
			}
			rec, err := graphcal.RecordOf(raw, owner)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := newCaptureSink(e.Pool, CaptureConfig{}).Upsert(connectorOwnerCtx(e, e.AdminUser, "graphcal"), rec); err != nil {
				t.Fatal(err)
			}
			count := e.WsCount(t, `SELECT count(*) FROM contact_email WHERE lower(email) IN ('bob@buyer.test','host@buyer.test') AND archived_at IS NULL`)
			want := 0
			if sensitivity == "normal" {
				want = 2
			}
			if count != want {
				t.Fatalf("contacts=%d, want %d", count, want)
			}
		})
	}
}
