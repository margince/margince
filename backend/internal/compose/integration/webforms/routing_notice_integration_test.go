// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package webforms

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func enableWebsiteLeadRouting(t *testing.T, e *apptest.AppEnv, owner string) {
	t.Helper()
	var routing crmcontracts.Automation
	if status := e.Call(t, "POST", "/v1/automations", integration.AnyMap{"key": "assign_lead_owner", "name": "Assign website leads", "params": integration.AnyMap{"owners": []string{owner}}}, nil, &routing); status != http.StatusCreated {
		t.Fatalf("create routing: %d", status)
	}
	if status := e.Call(t, "PATCH", "/v1/automations/"+routing.Id.String(), integration.AnyMap{"status": "enabled"}, inquiryVersion(t, routing), nil); status != http.StatusOK {
		t.Fatalf("enable routing: %d", status)
	}
}

func TestWebsiteLeadRoutingNotifiesOnceFromTheOriginalPost(t *testing.T) {
	for _, tc := range []struct {
		enabled       bool
		delivery      string
		notices, jobs int
	}{
		{false, "email", 0, 0}, {true, "in_app", 1, 0}, {true, "email", 1, 1}, {true, "digest", 1, 0}, {true, "off", 1, 0},
	} {
		t.Run(tc.delivery+" switch "+map[bool]string{true: "on", false: "off"}[tc.enabled], func(t *testing.T) {
			e := setupFormsApp(t)
			owner := apptest.SessionUserID(t, e)
			seat := inquirySeatContext(t, e, ids.MustParse(owner))
			if _, err := notices.NewStore(e.DB()).SaveNotificationPreference(seat, notices.ClassWebsiteLead, tc.delivery); err != nil {
				t.Fatal(err)
			}
			enableWebsiteLeadRouting(t, e, owner)
			form := createForm(t, e, integration.AnyMap{"name": "Website requests", "sender": "browser", "allowed_origins": []string{siteOrigin}, "notify_owner": tc.enabled})
			for _, post := range []struct{ key, name string }{{"original-post", "Original Visitor"}, {"later-post", "Later Visitor"}} {
				answer := browserPost(t, e, form.FormKey, url.Values{"email": {"routed@notice.example"}, "full_name": {post.name}, "_mrg_id": {post.key}}, siteOrigin)
				if answer.Status != http.StatusAccepted {
					t.Fatalf("post: %d %s", answer.Status, answer.Body)
				}
			}
			var before int
			if err := e.Owner.QueryRow(t.Context(), `SELECT count(*) FROM notice WHERE kind='website_lead'`).Scan(&before); err != nil {
				t.Fatal(err)
			}
			if before != 0 {
				t.Fatal("unassigned lead was announced")
			}
			event := inquiryEvent(t, e, "routed@notice.example", "lead.created")
			engine := compose.NewWorkflowEngine(e.DB())
			for range 2 {
				if err := engine.HandleEvent(t.Context(), event); err != nil {
					t.Fatal(err)
				}
				event.EventID = ids.NewV7()
			}
			var gotOwner string
			if err := e.Owner.QueryRow(t.Context(), `SELECT owner_id::text FROM lead WHERE email='routed@notice.example'`).Scan(&gotOwner); err != nil {
				t.Fatal(err)
			}
			if gotOwner != owner {
				t.Fatalf("routed owner = %s", gotOwner)
			}
			var count, jobs int
			if err := e.Owner.QueryRow(t.Context(), `SELECT count(*) FROM notice WHERE kind='website_lead'`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if err := e.Owner.QueryRow(t.Context(), `SELECT count(*) FROM river_job WHERE kind='notification_email'`).Scan(&jobs); err != nil {
				t.Fatal(err)
			}
			if count != tc.notices || jobs != tc.jobs {
				t.Fatalf("notices/jobs %d/%d, want %d/%d", count, jobs, tc.notices, tc.jobs)
			}
			if count == 1 {
				var recipient, key, body, source string
				if err := e.Owner.QueryRow(t.Context(), `SELECT recipient_user_id::text, dedupe_key, body FROM notice WHERE kind='website_lead'`).Scan(&recipient, &key, &body); err != nil {
					t.Fatal(err)
				}
				if err := e.Owner.QueryRow(t.Context(), `SELECT id::text FROM activity WHERE source_id=(SELECT source_id FROM lead WHERE email='routed@notice.example') AND source_system='web_form'`).Scan(&source); err != nil {
					t.Fatal(err)
				}
				if recipient != owner || key != "website_lead:"+source || !strings.Contains(body, "Original Visitor") || strings.Contains(body, "Later Visitor") {
					t.Fatalf("notice recipient=%s key=%s body=%s", recipient, key, body)
				}
			}
		})
	}
}

func TestWebsiteLeadRoutingSkipsAnUnreadableAcceptedNote(t *testing.T) {
	f := setupInquiryRecipient(t)
	e := f.e
	f.revoke(t, "activity")
	enableWebsiteLeadRouting(t, e, f.colleague.Id.String())
	form := createForm(t, e, integration.AnyMap{"name": "Private requests", "sender": "browser", "allowed_origins": []string{siteOrigin}, "notify_owner": true})
	answer := browserPost(t, e, form.FormKey, url.Values{"email": {"private-route@notice.example"}, "_mrg_id": {"private-route"}}, siteOrigin)
	if answer.Status != http.StatusAccepted {
		t.Fatalf("post: %d %s", answer.Status, answer.Body)
	}
	if err := compose.NewWorkflowEngine(e.DB()).HandleEvent(t.Context(), inquiryEvent(t, e, "private-route@notice.example", "lead.created")); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := e.Owner.QueryRow(t.Context(), `SELECT count(*) FROM notice WHERE kind='website_lead'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("unreadable note announced %d times", count)
	}
}

func TestWebsiteLeadRoutingRollsBackWhenItsNoticeCannotCommit(t *testing.T) {
	e := setupFormsApp(t)
	owner := apptest.SessionUserID(t, e)
	enableWebsiteLeadRouting(t, e, owner)
	form := createForm(t, e, integration.AnyMap{"name": "Atomic arrival", "sender": "browser", "allowed_origins": []string{siteOrigin}, "notify_owner": true})
	answer := browserPost(t, e, form.FormKey, url.Values{"email": {"rollback-route@notice.example"}, "_mrg_id": {"rollback-route"}}, siteOrigin)
	if answer.Status != http.StatusAccepted {
		t.Fatalf("post: %d %s", answer.Status, answer.Body)
	}
	t.Cleanup(func() {
		if _, err := e.Owner.Exec(context.Background(), `DROP TRIGGER IF EXISTS fail_routed_website_notice ON notice; DROP FUNCTION IF EXISTS fail_routed_website_notice()`); err != nil {
			t.Error(err)
		}
	})
	if _, err := e.Owner.Exec(t.Context(), `CREATE FUNCTION fail_routed_website_notice() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'refuse the test website notice'; END $$;
  CREATE TRIGGER fail_routed_website_notice BEFORE INSERT ON notice FOR EACH ROW WHEN (NEW.kind = 'website_lead') EXECUTE FUNCTION fail_routed_website_notice()`); err != nil {
		t.Fatal(err)
	}
	if err := compose.NewWorkflowEngine(e.DB()).HandleEvent(t.Context(), inquiryEvent(t, e, "rollback-route@notice.example", "lead.created")); err == nil {
		t.Fatal("notice failure was accepted")
	}
	var unassigned bool
	if err := e.Owner.QueryRow(t.Context(), `SELECT owner_id IS NULL FROM lead WHERE email='rollback-route@notice.example'`).Scan(&unassigned); err != nil {
		t.Fatal(err)
	}
	if !unassigned {
		t.Fatal("assignment escaped the failed notice transaction")
	}
	var count int
	if err := e.Owner.QueryRow(t.Context(), `SELECT count(*) FROM notice WHERE kind='website_lead'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("failed arrival left %d notices", count)
	}
}

func TestWebsiteLeadRoutingDoesNotReannounceAnAlreadyOwnedLead(t *testing.T) {
	f := setupInquiryRecipient(t)
	e := f.e
	admin := apptest.SessionUserID(t, e)
	enableWebsiteLeadRouting(t, e, admin)
	form := createForm(t, e, integration.AnyMap{"name": "Owned arrival", "sender": "browser", "allowed_origins": []string{siteOrigin}, "notify_owner": true, "owner_id": f.colleague.Id})
	answer := browserPost(t, e, form.FormKey, url.Values{"email": {"owned-route@notice.example"}, "_mrg_id": {"owned-route"}}, siteOrigin)
	if answer.Status != http.StatusAccepted {
		t.Fatalf("post: %d %s", answer.Status, answer.Body)
	}
	var lead string
	var version int64
	if err := e.Owner.QueryRow(t.Context(), `SELECT id::text, version FROM lead WHERE email='owned-route@notice.example'`).Scan(&lead, &version); err != nil {
		t.Fatal(err)
	}
	if status := e.Call(t, "PATCH", "/v1/leads/"+lead, integration.AnyMap{"owner_id": admin}, map[string]string{"If-Match": strconv.FormatInt(version, 10)}, nil); status != http.StatusOK {
		t.Fatalf("reassign lead: %d", status)
	}
	event := inquiryEvent(t, e, "owned-route@notice.example", "lead.created")
	event.EventID = ids.NewV7()
	if err := compose.NewWorkflowEngine(e.DB()).HandleEvent(t.Context(), event); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := e.Owner.QueryRow(t.Context(), `SELECT count(*) FROM notice WHERE kind='website_lead'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("owner change announced the original post %d times", count)
	}
}
