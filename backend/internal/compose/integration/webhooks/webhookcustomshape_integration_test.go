// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package webhooks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const fakeToken = "Bearer test-token-not-real"

type shapedSubscription struct {
	ID            string `json:"id"`
	Version       int64  `json:"version"`
	CustomHeaders []struct {
		Name       string `json:"name"`
		Configured bool   `json:"configured"`
	} `json:"custom_headers"`
	BodyTemplate    *string `json:"body_template"`
	BodyContentType *string `json:"body_content_type"`
}

func (we *webhookEnv) createShaped(t *testing.T, body integration.AnyMap) (shapedSubscription, string) {
	t.Helper()
	var created struct {
		Subscription  shapedSubscription `json:"subscription"`
		SigningSecret string             `json:"signing_secret"`
	}
	if status := we.Call(t, "POST", "/v1/webhook-subscriptions", body, nil, &created); status != http.StatusCreated {
		t.Fatalf("create shaped subscription → %d", status)
	}
	return created.Subscription, created.SigningSecret
}

func (we *webhookEnv) patchSubscription(t *testing.T, sub shapedSubscription, body integration.AnyMap) (shapedSubscription, int) {
	t.Helper()
	var out shapedSubscription
	status := we.Call(t, "PATCH", "/v1/webhook-subscriptions/"+sub.ID, body,
		map[string]string{"If-Match": strconv.FormatInt(sub.Version, 10)}, &out)
	return out, status
}

func (we *webhookEnv) auditMentions(t *testing.T, needle string) bool {
	t.Helper()
	var found bool
	if err := we.Owner.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM audit_log a WHERE a::text LIKE '%' || $1 || '%')`, needle).Scan(&found); err != nil {
		t.Fatalf("reading the audit log: %v", err)
	}
	return found
}

func TestAShapedSubscriptionDeliversItsHeadersAndRenderedBodySigned(t *testing.T) {
	we := setupWebhooks(t)
	rcv := newReceiver(t, http.StatusOK)
	now := clocktest.Now(t)
	deliverer := newTestDeliverer(we, &now, rcv.server.Client())

	sub, secret := we.createShaped(t, integration.AnyMap{
		"target_url": rcv.server.URL + "/hook", "event_types": []string{"deal.created"},
		"custom_headers": []integration.AnyMap{{"name": "authorization", "value": fakeToken}},
		"body_template":  `{"text": "{{ type }} on {{ entity.type }}", "id": "{{ entity.id }}"}`,
	})
	if len(sub.CustomHeaders) != 1 || sub.CustomHeaders[0].Name != "Authorization" || !sub.CustomHeaders[0].Configured {
		t.Fatalf("custom_headers = %+v, want Authorization configured", sub.CustomHeaders)
	}
	var raw json.RawMessage
	we.Call(t, "GET", "/v1/webhook-subscriptions/"+sub.ID, nil, nil, &raw)
	if strings.Contains(string(raw), "test-token") || strings.Contains(string(raw), "sealed") {
		t.Fatalf("a read carries the header value: %s", raw)
	}

	env := makeEnvelope(we.wsID, "deal.created")
	if err := deliverer.HandleEvent(context.Background(), env); err != nil {
		t.Fatalf("handle: %v", err)
	}
	hits := rcv.snapshot()
	if len(hits) != 1 {
		t.Fatalf("got %d POSTs, want 1", len(hits))
	}
	hit := hits[0]
	if got := hit.header.Get("Authorization"); got != fakeToken {
		t.Errorf("Authorization = %q, want the stored value", got)
	}
	want := fmt.Sprintf(`{"text": "deal.created on deal", "id": "%s"}`, env.Entity.ID)
	if string(hit.body) != want {
		t.Errorf("body = %s, want %s", hit.body, want)
	}
	if ct := hit.header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	ts, err := strconv.ParseInt(hit.timestamp, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	if hit.signature != verifySWSignature(t, secret, hit.webhookID, ts, hit.body) {
		t.Error("the signature does not cover the rendered body")
	}
	if we.auditMentions(t, "test-token") {
		t.Error("the audit log carries the header value")
	}
}

func TestAReplacedHeaderReachesTheRetryAndAKeptOneStays(t *testing.T) {
	we := setupWebhooks(t)
	rcv := newReceiver(t, http.StatusUnauthorized)
	now := clocktest.Now(t).Truncate(time.Microsecond)
	deliverer := newTestDeliverer(we, &now, rcv.server.Client())

	sub, _ := we.createShaped(t, integration.AnyMap{
		"target_url": rcv.server.URL + "/hook", "event_types": []string{"deal.created"},
		"custom_headers":    []integration.AnyMap{{"name": "X-Api-Key", "value": "old-key"}, {"name": "X-Tenant", "value": "acme"}},
		"body_content_type": nil,
	})
	if err := deliverer.HandleEvent(context.Background(), makeEnvelope(we.wsID, "deal.created")); err != nil {
		t.Fatal(err)
	}
	before := sub.Version
	sub, status := we.patchSubscription(t, sub, integration.AnyMap{
		"state":          "active",
		"custom_headers": []integration.AnyMap{{"name": "X-Api-Key", "value": "new-key"}, {"name": "x-tenant"}},
	})
	if status != http.StatusOK {
		t.Fatalf("patch headers → %d", status)
	}
	if sub.Version != before+1 {
		t.Errorf("one save moved version from %d to %d, want one step", before, sub.Version)
	}
	rcv.setStatus(http.StatusOK)
	now = now.Add(time.Minute)
	if err := deliverer.SweepOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	hits := rcv.snapshot()
	if len(hits) != 2 {
		t.Fatalf("got %d POSTs, want the first attempt and the retry", len(hits))
	}
	if got := hits[1].header.Get("X-Api-Key"); got != "new-key" {
		t.Errorf("retry X-Api-Key = %q, want the replaced value", got)
	}
	if got := hits[1].header.Get("X-Tenant"); got != "acme" {
		t.Errorf("retry X-Tenant = %q, want the kept value", got)
	}
	if _, status := we.patchSubscription(t, shapedSubscription{ID: sub.ID, Version: sub.Version - 1},
		integration.AnyMap{"custom_headers": []integration.AnyMap{}}); status != http.StatusConflict {
		t.Errorf("a stale If-Match on a header-only patch → %d, want 409", status)
	}
	if we.auditMentions(t, "new-key") || we.auditMentions(t, "old-key") {
		t.Error("the audit log carries a header value")
	}
	if !we.auditMentions(t, "custom_headers_replaced") {
		t.Error("the audit log does not say which header value was replaced")
	}
}

func TestInputThatCouldMisdirectADeliveryIsRefused(t *testing.T) {
	we := setupWebhooks(t)
	for _, tc := range []struct {
		name  string
		body  integration.AnyMap
		field string
	}{
		{"reserved header", integration.AnyMap{"custom_headers": []integration.AnyMap{{"name": "webhook-signature", "value": "v1,x"}}}, "custom_headers"},
		{"header injection", integration.AnyMap{"custom_headers": []integration.AnyMap{{"name": "X-A", "value": "a\r\nX-B: 1"}}}, "custom_headers"},
		{"placeholder outside a string", integration.AnyMap{"body_template": `{"n": {{ data.amount }}}`}, "body_template"},
		{"content type without a template", integration.AnyMap{"body_content_type": "text/plain"}, "body_content_type"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.body["target_url"], tc.body["event_types"] = "https://receiver.example/hook", []string{"deal.created"}
			var problem struct {
				Details struct {
					Errors []struct {
						Field string `json:"field"`
					} `json:"errors"`
				} `json:"details"`
			}
			if status := we.Call(t, "POST", "/v1/webhook-subscriptions", tc.body, nil, &problem); status != http.StatusUnprocessableEntity {
				t.Fatalf("create → %d, want 422", status)
			}
			if errs := problem.Details.Errors; len(errs) == 0 || errs[0].Field != tc.field {
				t.Errorf("problem = %+v, want field %s", problem, tc.field)
			}
		})
	}
}

func TestARedirectIsAFailedAttemptAndIsNotFollowed(t *testing.T) {
	we := setupWebhooks(t)
	rcv := newReceiver(t, http.StatusMovedPermanently)
	now := clocktest.Now(t)
	deliverer := newTestDeliverer(we, &now, rcv.server.Client())
	subID, _ := we.createSubscription(t, rcv.server.URL+"/hook", []string{"deal.created"})

	if err := deliverer.HandleEvent(context.Background(), makeEnvelope(we.wsID, "deal.created")); err != nil {
		t.Fatal(err)
	}
	if n := rcv.count.Load(); n != 1 {
		t.Fatalf("receiver saw %d requests, want 1: a redirect is never followed", n)
	}
	assertDeliveryStatus(t, we, subID, "retrying", 1)
	if reason := we.deliveryError(t, subID); !strings.Contains(reason, "301") {
		t.Errorf("the row says %q, want the 301", reason)
	}
}

func TestABodyOverTheCapIsParkedUnsentAndCannotBeReplayed(t *testing.T) {
	we := setupWebhooks(t)
	rcv := newReceiver(t, http.StatusOK)
	now := clocktest.Now(t)
	deliverer := newTestDeliverer(we, &now, rcv.server.Client())
	sub, _ := we.createShaped(t, integration.AnyMap{
		"target_url": rcv.server.URL + "/hook", "event_types": []string{"deal.created"},
		"body_template": strings.Repeat("{{ data.blob }}", 500), "body_content_type": "text/plain",
	})
	env := makeEnvelope(we.wsID, "deal.created")
	env.Payload = []byte(`{"blob": "` + strings.Repeat("y", 600) + `"}`)
	if err := deliverer.HandleEvent(context.Background(), env); err != nil {
		t.Fatalf("a render failure must not re-drive the bus: %v", err)
	}
	if err := deliverer.HandleEvent(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	if n := rcv.count.Load(); n != 0 {
		t.Fatalf("receiver saw %d requests, want none", n)
	}
	assertDeliveryStatus(t, we, sub.ID, "dead_lettered", 0)
	if reason := we.deliveryError(t, sub.ID); !strings.Contains(reason, "256 KiB") {
		t.Errorf("the row says %q, want the size cap", reason)
	}
	path := fmt.Sprintf("/v1/webhook-subscriptions/%s/deliveries/%s/replay", sub.ID, we.deliveryID(t, sub.ID))
	if status := we.Call(t, "POST", path, nil, nil, nil); status != http.StatusUnprocessableEntity {
		t.Errorf("replaying a body-less delivery → %d, want 422", status)
	}
}

func TestPreviewRendersASampleOfTheEventWithTheDeliveryEngine(t *testing.T) {
	we := setupWebhooks(t)
	var preview struct {
		Body        string `json:"body"`
		ContentType string `json:"content_type"`
		ByteLength  int    `json:"byte_length"`
	}
	status := we.Call(t, "POST", "/v1/webhook-subscriptions/body-preview", integration.AnyMap{
		"event_type": "deal.stage_changed", "body_template": `{"to": "{{ data.to_status }}", "t": "{{ type }}"}`,
	}, nil, &preview)
	if status != http.StatusOK {
		t.Fatalf("preview → %d", status)
	}
	if !strings.HasPrefix(preview.Body, `{"to": "`) || !strings.HasSuffix(preview.Body, `", "t": "deal.stage_changed"}`) ||
		preview.ByteLength != len(preview.Body) || preview.ContentType != "application/json" {
		t.Fatalf("preview = %+v", preview)
	}
	if status := we.Call(t, "POST", "/v1/webhook-subscriptions/body-preview", integration.AnyMap{
		"event_type": "deal.created", "body_template": `{"x": {{ data.name }}}`,
	}, nil, nil); status != http.StatusUnprocessableEntity {
		t.Errorf("an unsafe template → %d, want 422", status)
	}
	var count int
	we.inWorkspaceTx(t, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT count(*) FROM webhook_delivery`).Scan(&count)
	})
	if count != 0 {
		t.Errorf("a preview stored %d deliveries", count)
	}
}

func TestABodyThatRendersEmptyIsSentAndCanBeReplayed(t *testing.T) {
	we := setupWebhooks(t)
	rcv := newReceiver(t, http.StatusInternalServerError)
	now := clocktest.Now(t)
	deliverer := newTestDeliverer(we, &now, rcv.server.Client())
	sub, _ := we.createShaped(t, integration.AnyMap{
		"target_url": rcv.server.URL + "/hook", "event_types": []string{"activity.captured"},
		"body_template": "{{ data.absent }}", "body_content_type": "text/plain",
	})
	// A real record, so the replay's visibility check finds its subject.
	if err := deliverer.HandleEvent(context.Background(), activityEnvelope(we.wsID, we.seedOpenActivity(t))); err != nil {
		t.Fatal(err)
	}
	assertDeliveryStatus(t, we, sub.ID, "retrying", 1)
	path := fmt.Sprintf("/v1/webhook-subscriptions/%s/deliveries/%s/replay", sub.ID, we.deliveryID(t, sub.ID))
	if status := we.Call(t, "POST", path, nil, nil, nil); status != http.StatusOK {
		t.Fatalf("replaying an empty body that was sent → %d, want 200", status)
	}
	// The API's replay dials through the production client, which refuses this
	// loopback receiver: a replay that sent records a dial failure, not the 500.
	assertDeliveryStatus(t, we, sub.ID, "retrying", 1)
	if reason := we.deliveryError(t, sub.ID); !strings.Contains(reason, "request failed") {
		t.Errorf("after the replay the row says %q, want the replay's own failed request", reason)
	}
}

func TestADeliveryNamesItsRecordAndLinksToItsPage(t *testing.T) {
	we := setupWebhooks(t)
	rcv := newReceiver(t, http.StatusOK)
	now := clocktest.Now(t)
	deliverer := newTestDeliverer(we, &now, rcv.server.Client()).WithPublicBaseURL("https://crm.example.test/")
	we.createShaped(t, integration.AnyMap{
		"target_url": rcv.server.URL + "/hook", "event_types": []string{"deal.created"},
		"body_template": `{"text": "{{ entity.name }}", "link": "{{ entity.url }}"}`,
	})
	dealID := apptest.CreateOpenDeal(t, we.AppEnv, apptest.DiscoverSeededPipeline(t, we.AppEnv))
	var deal struct {
		Name string `json:"name"`
	}
	we.Call(t, "GET", "/v1/deals/"+dealID, nil, nil, &deal)
	env := makeEnvelope(we.wsID, "deal.created")
	env.Entity.ID = mustParseUUID(t, dealID)
	if err := deliverer.HandleEvent(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	hits := rcv.snapshot()
	if len(hits) != 1 {
		t.Fatalf("got %d POSTs, want 1", len(hits))
	}
	want := fmt.Sprintf(`{"text": "%s", "link": "https://crm.example.test/#/deals/%s"}`, deal.Name, dealID)
	if string(hits[0].body) != want {
		t.Errorf("body = %s, want %s", hits[0].body, want)
	}
}

func TestAContactIsLinkedButNotNamed(t *testing.T) {
	we := setupWebhooks(t)
	rcv := newReceiver(t, http.StatusOK)
	now := clocktest.Now(t)
	deliverer := newTestDeliverer(we, &now, rcv.server.Client()).WithPublicBaseURL("https://crm.example.test")
	we.createShaped(t, integration.AnyMap{
		"target_url": rcv.server.URL + "/hook", "event_types": []string{"contact.created"},
		"body_template": `{"name": "{{ entity.name }}", "link": "{{ entity.url }}"}`,
	})
	contactID := we.seedContact(t, "Ada Lovelace")
	env := makeEnvelopeFor(we.wsID, "contact.created", "contact")
	env.Entity.ID = contactID
	if err := deliverer.HandleEvent(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	hits := rcv.snapshot()
	if len(hits) != 1 {
		t.Fatalf("got %d POSTs, want 1", len(hits))
	}
	want := fmt.Sprintf(`{"name": "", "link": "https://crm.example.test/#/contacts/%s"}`, contactID)
	if string(hits[0].body) != want {
		t.Errorf("body = %s, want %s: a contact's name stays out of stored deliveries", hits[0].body, want)
	}
}

func TestADeliveryNamesTheRecordsOwnerAndLeavesItEmptyWithoutOne(t *testing.T) {
	we := setupWebhooks(t)
	rcv := newReceiver(t, http.StatusOK)
	now := clocktest.Now(t)
	deliverer := newTestDeliverer(we, &now, rcv.server.Client())
	we.createShaped(t, integration.AnyMap{
		"target_url": rcv.server.URL + "/hook", "event_types": []string{"deal.created", "company.created"},
		"body_template": `{"owner": "{{ entity.owner_name }}"}`,
	})
	dealID := mustParseUUID(t, apptest.CreateOpenDeal(t, we.AppEnv, apptest.DiscoverSeededPipeline(t, we.AppEnv)))
	ownerID := ids.NewV7()
	we.execInWorkspace(t, `INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Shayne Owner')`,
		ownerID, "owner-"+ownerID.String()+"@webhooks.test")
	var companyID ids.UUID
	we.inWorkspaceTx(t, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`UPDATE deal SET owner_id = $2 WHERE id = $1 RETURNING company_id`, dealID, ownerID).Scan(&companyID)
	})
	we.execInWorkspace(t, `UPDATE company SET owner_id = $2 WHERE id = $1`, companyID, ownerID)

	deliver := func(eventType, entityType string, id ids.UUID) {
		env := makeEnvelopeFor(we.wsID, eventType, entityType)
		env.Entity.ID = id
		if err := deliverer.HandleEvent(context.Background(), env); err != nil {
			t.Fatal(err)
		}
	}
	deliver("deal.created", "deal", dealID)
	deliver("company.created", "company", companyID)
	we.execInWorkspace(t, `UPDATE deal SET owner_id = NULL WHERE id = $1`, dealID)
	deliver("deal.created", "deal", dealID)

	hits := rcv.snapshot()
	if len(hits) != 3 {
		t.Fatalf("got %d POSTs, want 3", len(hits))
	}
	for i, want := range []string{`{"owner": "Shayne Owner"}`, `{"owner": "Shayne Owner"}`, `{"owner": ""}`} {
		if string(hits[i].body) != want {
			t.Errorf("delivery %d: body = %s, want %s", i+1, hits[i].body, want)
		}
	}
}
