// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package webforms

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestWebsiteOwnerNoticesFollowTheFormSwitchAndDeliveryChoice(t *testing.T) {
	for _, test := range []struct {
		name                string
		enabled             bool
		delivery            string
		assigned            bool
		wantNotice, wantJob int
	}{
		{"off by default", false, "email", true, 0, 0},
		{"in app", true, "in_app", true, 1, 0},
		{"email", true, "email", true, 1, 1},
		{"digest", true, "digest", true, 1, 0},
		{"off preference", true, "off", true, 1, 0},
		{"unassigned", true, "email", false, 0, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			e := setupFormsApp(t)
			owner := apptest.SessionUserID(t, e)
			seat := inquirySeatContext(t, e, ids.MustParse(owner))
			if _, err := notices.NewStore(e.DB()).SaveNotificationPreference(seat, notices.ClassWebsiteLead, test.delivery); err != nil {
				t.Fatal(err)
			}

			input := integration.AnyMap{"name": "Request a meeting", "sender": "browser", "allowed_origins": []string{siteOrigin}, "notify_owner": test.enabled}
			if test.assigned {
				input["owner_id"] = owner
			}
			form := createForm(t, e, input)
			fields := url.Values{"email": {"visitor@notice.example"}, "full_name": {"Website Visitor"}, "company": {"Visitor Company"}, "_mrg_id": {"notice-once"}}
			for range 2 {
				answer := browserPost(t, e, form.FormKey, fields, siteOrigin)
				if answer.Status != http.StatusAccepted {
					t.Fatalf("submission: %d %s", answer.Status, answer.Body)
				}
			}
			var count, jobs int
			if err := e.Owner.QueryRow(t.Context(), `SELECT count(*) FROM notice WHERE kind='website_lead'`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if err := e.Owner.QueryRow(t.Context(), `SELECT count(*) FROM river_job WHERE kind='notification_email'`).Scan(&jobs); err != nil {
				t.Fatal(err)
			}
			if count != test.wantNotice || jobs != test.wantJob {
				t.Fatalf("notices=%d jobs=%d, want %d/%d", count, jobs, test.wantNotice, test.wantJob)
			}
			if count == 1 {
				var recipient, target string
				if err := e.Owner.QueryRow(t.Context(), `SELECT recipient_user_id::text,target_type FROM notice WHERE kind='website_lead' AND body LIKE '%visitor@notice.example%'`).Scan(&recipient, &target); err != nil {
					t.Fatal(err)
				}
				if recipient != owner || target != "lead" {
					t.Fatalf("recipient=%s target=%s", recipient, target)
				}
			}
		})
	}
}

func TestWebsiteOwnerNoticeWaitsForTheHandlingPlansAssignment(t *testing.T) {
	for _, mode := range []string{"one_colleague", "coordinator"} {
		t.Run(mode, func(t *testing.T) {
			e := setupFormsApp(t)
			form := createForm(t, e, integration.AnyMap{"name": "Handled inquiries", "sender": "browser", "allowed_origins": []string{siteOrigin}, "notify_owner": true})
			setupInquiryPolicy(t, e, form, mode)
			answer := browserPost(t, e, form.FormKey, url.Values{"email": {"assigned@notice.example"}, "full_name": {"Assigned Visitor"}, "_mrg_id": {"handled-once"}}, siteOrigin)
			if answer.Status != http.StatusAccepted {
				t.Fatalf("submission: %d %s", answer.Status, answer.Body)
			}
			var before int
			if err := e.Owner.QueryRow(t.Context(), `SELECT count(*) FROM notice WHERE kind='website_lead'`).Scan(&before); err != nil {
				t.Fatal(err)
			}
			if before != 0 {
				t.Fatal("told someone before the plan chose an owner")
			}
			event := inquiryEvent(t, e, "assigned@notice.example", "activity.captured")
			engine := compose.NewWorkflowEngine(e.DB())
			for range 2 {
				if err := engine.HandleEvent(t.Context(), event); err != nil {
					t.Fatal(err)
				}
				event.EventID = ids.NewV7()
			}
			var count int
			if err := e.Owner.QueryRow(t.Context(), `SELECT count(*) FROM notice WHERE kind='website_lead'`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			want := 1
			if mode == "coordinator" {
				want = 0
			}
			if count != want {
				t.Fatalf("%s: %d notices, want %d", mode, count, want)
			}
		})
	}
}

func TestWebsiteOwnerNoticeKeepsAnExistingRecordsOwner(t *testing.T) {
	for _, kind := range []string{"lead", "contact"} {
		t.Run(kind, func(t *testing.T) {
			f := setupInquiryRecipient(t)
			e := f.e
			owner := apptest.SessionUserID(t, e)
			input := integration.AnyMap{"source": "manual", "owner_id": owner, "full_name": "Existing owner record"}
			if kind == "lead" {
				input["email"] = "existing@notice.example"
			} else {
				input["emails"] = []integration.AnyMap{{"email": "existing@notice.example", "email_type": "work", "is_primary": true}}
			}
			if status := e.Call(t, "POST", "/v1/"+kind+"s", input, nil, nil); status != http.StatusCreated {
				t.Fatalf("create %s: %d", kind, status)
			}
			form := createForm(t, e, integration.AnyMap{"name": "Existing records", "sender": "browser", "allowed_origins": []string{siteOrigin}, "notify_owner": true, "owner_id": f.colleague.Id})
			answer := browserPost(t, e, form.FormKey, url.Values{"email": {"existing@notice.example"}, "message": {"Follow up"}}, siteOrigin)
			if answer.Status != http.StatusAccepted {
				t.Fatalf("post: %d %s", answer.Status, answer.Body)
			}
			var recipient, target string
			if err := e.Owner.QueryRow(t.Context(), `SELECT recipient_user_id::text,target_type FROM notice WHERE kind='website_lead'`).Scan(&recipient, &target); err != nil {
				t.Fatal(err)
			}
			if recipient != owner || target != kind {
				t.Fatalf("recipient=%s target=%s, want %s/%s", recipient, target, owner, kind)
			}
		})
	}
}

func TestWebsiteOwnerNoticeSkipsAnInactiveOwnerAndAnUnreadableNote(t *testing.T) {
	for _, state := range []string{"inactive", "note hidden"} {
		t.Run(state, func(t *testing.T) {
			f := setupInquiryRecipient(t)
			e := f.e
			form := createForm(t, e, integration.AnyMap{"name": "Owner access", "sender": "browser", "allowed_origins": []string{siteOrigin}, "notify_owner": true, "owner_id": f.colleague.Id})
			if state == "inactive" {
				if status := e.Call(t, "POST", "/v1/users/"+f.colleague.Id.String()+"/deactivate", integration.AnyMap{}, nil, nil); status != http.StatusOK {
					t.Fatalf("deactivate: %d", status)
				}
			} else {
				f.revoke(t, "activity")
			}
			answer := browserPost(t, e, form.FormKey, url.Values{"email": {"hidden@notice.example"}, "message": {"Keep the accepted post"}}, siteOrigin)
			if answer.Status != http.StatusAccepted {
				t.Fatalf("post: %d %s", answer.Status, answer.Body)
			}
			var count int
			if err := e.Owner.QueryRow(t.Context(), `SELECT count(*) FROM notice WHERE kind='website_lead'`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("%s got %d notices", state, count)
			}
		})
	}
}
