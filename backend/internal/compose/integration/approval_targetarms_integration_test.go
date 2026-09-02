// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// One confirm-first verb per target-visibility arm — a curator's list, a tag, a
// saved view, an offer template and a webhook subscription — each walking the
// whole loop over the passport surface a real agent uses: the agent's call stages
// instead of writing, the human's inbox LISTS the row and answers for it, the
// decision releases it, and the identical call then lands.
//
// All three reads are asserted, not just the decision. `decidable` backs the
// inbox list, the single Get and the Decide alike, and a staged row it rejects is
// invisible AND undecidable: the refusal tells the agent to go and get an approval
// no human can ever give, and the row sits pending until the TTL clears it.

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/webhooks"
)

func TestConfirmFirstArchivesAreDecidableForEveryTargetArm(t *testing.T) {
	// The webhook-subscription route seals a signing secret, so the suite boots
	// with the deployment key the create path needs.
	cipher, err := webhooks.NewCipher(bytes.Repeat([]byte{0x5a}, webhooks.WebhookKeyBytes))
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	e := apptest.SetupAppWithOptions(t, compose.WithWebhookSigningKey(cipher))
	e.BootstrapWorkspace(t)
	bearer := agentBearer(t, e, "target-arm agent")

	// These three archive on the agent's own passport now: a passport carries
	// the granting human's seat and row scope, and each is ordinary work its
	// holder does unaided. What is still asserted is that the ROUTE is reachable
	// for an agent and performs — the half that used to be hidden behind the
	// approval.
	t.Run("tag", func(t *testing.T) {
		id := createdID(t, e, "/v1/tags", AnyMap{"name": "Champion"})
		archivesOnItsOwnPassport(t, e, bearer, "/v1/tags/"+id)
	})

	t.Run("saved_view", func(t *testing.T) {
		id := createdID(t, e, "/v1/views", AnyMap{
			"resource": "people", "name": "My people", "query": AnyMap{"columns": []any{"full_name"}},
		})
		archivesOnItsOwnPassport(t, e, bearer, "/v1/views/"+id)
	})

	t.Run("offer_template", func(t *testing.T) {
		id := createdID(t, e, "/v1/offer-templates", AnyMap{
			"name": "Standard DE", "layout": AnyMap{"logo_url": "https://example.test/logo.png"},
		})
		archivesOnItsOwnPassport(t, e, bearer, "/v1/offer-templates/"+id)
	})

	t.Run("webhook_subscription", func(t *testing.T) {
		var created struct {
			Subscription struct {
				ID string `json:"id"`
			} `json:"subscription"`
		}
		if status := e.Call(t, "POST", "/v1/webhook-subscriptions", AnyMap{
			"target_url": "https://ok.example/hook", "event_types": []string{"deal.created"},
		}, nil, &created); status != http.StatusCreated {
			t.Fatalf("create subscription → %d", status)
		}
		releaseStagedCall(t, e, bearer, "PATCH", "/v1/webhook-subscriptions/"+created.Subscription.ID,
			AnyMap{"state": "paused"}, "webhook_subscription")
	})
}

// A decision is a judgment about the row as the human was shown it, so a row
// that moved underneath the approval revokes it. The staging reads the target's
// version itself (approvals.versionTables), redemption re-checks it, and the
// released call carries it as its own If-Match — three steps that all
// short-circuit on a NULL pin, which is what a target type absent from that set
// stages. Asserted on the subscription patch because it is the confirm-first
// route whose staged change is a field patch rather than an archive: an admin
// re-pointing the endpoint between the staging and the approval is exactly the
// drift the human never agreed to.
func TestAStagedPatchIsRefusedWhenItsTargetMovedBeforeTheApproval(t *testing.T) {
	cipher, err := webhooks.NewCipher(bytes.Repeat([]byte{0x5a}, webhooks.WebhookKeyBytes))
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	e := apptest.SetupAppWithOptions(t, compose.WithWebhookSigningKey(cipher))
	e.BootstrapWorkspace(t)
	bearer := agentBearer(t, e, "skew agent")

	var created struct {
		Subscription struct {
			ID      string `json:"id"`
			Version int64  `json:"version"`
		} `json:"subscription"`
	}
	if status := e.Call(t, "POST", "/v1/webhook-subscriptions", AnyMap{
		"target_url": "https://ok.example/hook", "event_types": []string{"deal.created"},
	}, nil, &created); status != http.StatusCreated {
		t.Fatalf("create subscription → %d", status)
	}
	path := "/v1/webhook-subscriptions/" + created.Subscription.ID
	staged := AnyMap{"state": "paused"}

	var problem struct {
		Code   string `json:"code"`
		Detail string `json:"detail"`
	}
	if status := e.Call(t, "PATCH", path, staged, bearer, &problem); status != http.StatusForbidden ||
		problem.Code != "approval_required" {
		t.Fatalf("agent patch → %d %q, want 403 approval_required", status, problem.Code)
	}
	approvalID := ExtractStagedApprovalID(t, problem.Detail)

	// The admin's own edit, on a different field so the guarded patch really
	// bumps the version rather than finding nothing to change.
	var edited struct {
		Subscription struct {
			Version int64 `json:"version"`
		} `json:"subscription"`
	}
	if status := e.Call(t, "PATCH", path, AnyMap{"event_types": []string{"deal.updated"}}, nil, &edited); status != http.StatusOK {
		t.Fatalf("admin re-point → %d", status)
	}
	if edited.Subscription.Version == created.Subscription.Version {
		t.Fatalf("the admin's edit left version at %d — the fixture proves nothing about a pin it never moved",
			edited.Subscription.Version)
	}

	if status := e.Call(t, "POST", "/v1/approvals/"+approvalID+"/approve", AnyMap{}, nil, nil); status != http.StatusOK {
		t.Fatalf("human approve → %d", status)
	}
	withToken := map[string]string{"X-Approval-Token": approvalID}
	for k, v := range bearer {
		withToken[k] = v
	}
	problem = struct {
		Code   string `json:"code"`
		Detail string `json:"detail"`
	}{}
	if status := e.Call(t, "PATCH", path, staged, withToken, &problem); status != http.StatusConflict ||
		problem.Code != "version_skew" {
		t.Fatalf("release over a moved row → %d %q, want 409 version_skew — the approval authorized the row "+
			"the human saw, not whatever it became", status, problem.Code)
	}
}

// agentBearer mints a passport and returns the Authorization header a governed
// agent call carries. A passport is the credential the tool surface and REST
// alike admit an agent under (ADR-0055), so it is what a confirm-first refusal
// has to be provoked with.
func agentBearer(t *testing.T, e *apptest.AppEnv, label string) map[string]string {
	t.Helper()
	var minted struct {
		Token string `json:"token"`
	}
	if status := e.Call(t, "POST", "/v1/passports", AnyMap{
		"label": label, "scopes": []string{"read", "write"},
	}, nil, &minted); status != http.StatusCreated {
		t.Fatalf("issue passport → %d", status)
	}
	return map[string]string{"Authorization": "Bearer " + minted.Token}
}

// createdID creates one record as the bootstrap admin over their session and
// returns its id — the human-owned row a later agent call stages against.
func createdID(t *testing.T, e *apptest.AppEnv, path string, body AnyMap) string {
	t.Helper()
	var created struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", path, body, nil, &created); status != http.StatusCreated {
		t.Fatalf("POST %s → %d", path, status)
	}
	if created.ID == "" {
		t.Fatalf("POST %s returned no id", path)
	}
	return created.ID
}

// releaseStagedCall asserts the full confirm-first loop for one route: the agent
// is refused with a staged approval, the human can see and decide that approval,
// and the identical call then executes under the approval token.
//
// The identical body is sent twice because the diff_hash binding is what makes an
// approval authorize THIS call and no other.
// archivesOnItsOwnPassport asserts the agent's archive PERFORMS rather than
// staging — the shape every arm but webhook_subscription now takes.
//
// It asserts the success status and then re-reads the row, because "not 403" is
// not the property. A 404 from a mistyped path, or a 500 from a seam that cannot
// archive this type at all, both pass a refusal check while proving nothing —
// and the arms this covers exist precisely because each is a DIFFERENT target
// type the seam has to route.
func archivesOnItsOwnPassport(t *testing.T, e *apptest.AppEnv, bearer map[string]string, path string) {
	t.Helper()
	if status := e.Call(t, "DELETE", path, nil, bearer, nil); status != http.StatusOK &&
		status != http.StatusNoContent {
		t.Fatalf("agent DELETE %s → %d, want the archive to perform — a passport archives what its "+
			"holder could archive unaided", path, status)
	}
	var row struct {
		ArchivedAt *string `json:"archived_at"`
	}
	// Not every arm HAS a read-by-id route — a tag is reached through its
	// collection, and answers 405. Where the row cannot be re-read the archive's
	// success status is all there is, which is still strictly more than the
	// "not 403" this used to assert.
	switch status := e.Call(t, "GET", path, nil, nil, &row); status {
	case http.StatusOK:
	case http.StatusNotFound, http.StatusMethodNotAllowed:
		return
	default:
		t.Fatalf("re-reading %s after the archive → %d", path, status)
	}
	if row.ArchivedAt == nil {
		t.Errorf("%s answered success but the row is still live — the call was admitted and "+
			"performed nothing", path)
	}
}

func releaseStagedCall(t *testing.T, e *apptest.AppEnv, bearer map[string]string, method, path string, body AnyMap, wantTargetType string) {
	t.Helper()
	var problem struct {
		Code   string `json:"code"`
		Detail string `json:"detail"`
	}
	if status := e.Call(t, method, path, body, bearer, &problem); status != http.StatusForbidden ||
		problem.Code != "approval_required" {
		t.Fatalf("agent %s %s → %d %q, want 403 approval_required", method, path, status, problem.Code)
	}
	approvalID := ExtractStagedApprovalID(t, problem.Detail)

	assertDecidableInTheInbox(t, e, approvalID, wantTargetType)

	if status := e.Call(t, "POST", "/v1/approvals/"+approvalID+"/approve", AnyMap{}, nil, nil); status != http.StatusOK {
		t.Fatalf("human approve → %d, want 200 — a row the inbox lists and cannot decide is the same dead "+
			"end one step later", status)
	}
	withToken := map[string]string{"X-Approval-Token": approvalID}
	for k, v := range bearer {
		withToken[k] = v
	}
	// Every route here answers 200 on release: an archive returns the archived
	// row and the subscription patch the updated one.
	if status := e.Call(t, method, path, body, withToken, nil); status != http.StatusOK {
		t.Fatalf("approved retry → %d, want 200 — the decision must release the staged call", status)
	}
}
