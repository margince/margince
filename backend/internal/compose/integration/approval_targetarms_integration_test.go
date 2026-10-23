// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// An agent's archive, once per target-visibility arm. Each arm is a target type
// the seam routes differently, so each is driven over a real passport.
//
// A tag and an offer template are ordinary work, so the archive performs. A
// saved view and a webhook subscription are human-only configuration, so the
// agent is refused and nothing is staged.

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/webhooks"
)

func TestAnAgentArchivePerformsOrIsRefusedForEveryTargetArm(t *testing.T) {
	// The webhook-subscription route seals a signing secret, so the suite boots
	// with the deployment key the create path needs.
	cipher, err := webhooks.NewCipher(bytes.Repeat([]byte{0x5a}, webhooks.WebhookKeyBytes))
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	e := apptest.SetupAppWithOptions(t, compose.WithWebhookSigningKey(cipher))
	e.BootstrapWorkspace(t)
	bearer := agentBearer(t, e, "target-arm agent")

	t.Run("tag", func(t *testing.T) {
		id := createdID(t, e, "/v1/tags", AnyMap{"name": "Champion"})
		archivesOnItsOwnPassport(t, e, bearer, "/v1/tags/"+id)
	})

	t.Run("saved_view", func(t *testing.T) {
		id := createdID(t, e, "/v1/views", AnyMap{
			"resource": "contacts", "name": "My contacts", "query": AnyMap{"columns": []any{"full_name"}},
		})
		archiveIsRefusedAsHumanOnly(t, e, bearer, "/v1/views/"+id, "saved_view", id)
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
		id := created.Subscription.ID
		archiveIsRefusedAsHumanOnly(t, e, bearer, "/v1/webhook-subscriptions/"+id, "webhook_subscription", id)
	})
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

// archivesOnItsOwnPassport asserts the agent's archive performs rather than
// staging. It re-reads the row after the success status, because "not 403" is
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

// archiveIsRefusedAsHumanOnly asserts the human-only refusal on an archive
// route: permission_denied, no approval staged, and the row still live. A staged
// row would ask a human a question the agent may never ask.
func archiveIsRefusedAsHumanOnly(t *testing.T, e *apptest.AppEnv, bearer map[string]string, path, table, id string) {
	t.Helper()
	var problem struct {
		Code string `json:"code"`
	}
	if status := e.Call(t, "DELETE", path, nil, bearer, &problem); status != http.StatusForbidden ||
		problem.Code != "permission_denied" {
		t.Fatalf("agent DELETE %s → %d %q, want 403 permission_denied (the contract declares it human-only)",
			path, status, problem.Code)
	}
	var staged int
	if err := e.Owner.QueryRow(t.Context(),
		`SELECT count(*) FROM approval WHERE target_entity_id = $1`, id).Scan(&staged); err != nil {
		t.Fatalf("counting approvals: %v", err)
	}
	if staged != 0 {
		t.Errorf("a human-only refusal staged %d approval(s) on %s, want 0", staged, path)
	}
	var archived bool
	if err := e.Owner.QueryRow(t.Context(),
		`SELECT archived_at IS NOT NULL FROM `+pgx.Identifier{table}.Sanitize()+` WHERE id = $1`, id).Scan(&archived); err != nil {
		t.Fatalf("reading %s back: %v", path, err)
	}
	if archived {
		t.Errorf("%s is archived after the agent was refused", path)
	}
}
