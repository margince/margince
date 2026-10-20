// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A save that changes nothing changes nothing. The answer is the current record
// at its current version, and no audit row or event is written. A stale
// If-Match still answers 409.

import (
	"bytes"
	"net/http"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/webhooks"
)

type versionedWire struct {
	ID      string `json:"id"`
	Version int64  `json:"version"`
}

// assertSaveChangesNothing sends a save at the record's current version and
// checks the version, the audit rows and the outbox all stayed where they were.
func assertSaveChangesNothing(
	t *testing.T, e *apptest.AppEnv, method, path string, body AnyMap, entityID string, version int64,
) {
	t.Helper()
	count := func(query string) int {
		var n int
		if err := e.Owner.QueryRow(t.Context(), query, entityID).Scan(&n); err != nil {
			t.Fatalf("counting for %s: %v", entityID, err)
		}
		return n
	}
	const audits = `SELECT count(*) FROM audit_log WHERE entity_id = $1`
	const events = `SELECT count(*) FROM event_outbox WHERE envelope::text LIKE '%' || $1 || '%'`
	auditsBefore, eventsBefore := count(audits), count(events)

	var answer versionedWire
	headers := map[string]string{"If-Match": strconv.FormatInt(version, 10)}
	if status := e.Call(t, method, path, body, headers, &answer); status != http.StatusOK {
		t.Fatalf("%s %s with the stored values -> %d, want 200", method, path, status)
	}
	if answer.Version != version {
		t.Errorf("%s %s with the stored values answered version %d, want the unchanged %d",
			method, path, answer.Version, version)
	}
	if got := count(audits); got != auditsBefore {
		t.Errorf("%s %s with the stored values wrote %d audit row(s), want none", method, path, got-auditsBefore)
	}
	if got := count(events); got != eventsBefore {
		t.Errorf("%s %s with the stored values emitted %d event(s), want none", method, path, got-eventsBefore)
	}
}

func TestASaveOfTheStoredValuesChangesNothing(t *testing.T) {
	e := setupRelationships(t)

	var contact versionedWire
	if status := e.Call(t, "PATCH", "/v1/contacts/"+e.contactID, AnyMap{"title": "Head of Sales"},
		map[string]string{"If-Match": "1"}, &contact); status != http.StatusOK {
		t.Fatalf("setting the contact's title -> %d", status)
	}
	t.Run("contact", func(t *testing.T) {
		assertSaveChangesNothing(t, e.AppEnv, "PATCH", "/v1/contacts/"+e.contactID,
			AnyMap{"title": "Head of Sales"}, e.contactID, contact.Version)
	})
	t.Run("a stale If-Match on a save of the stored values is still a conflict", func(t *testing.T) {
		status := e.Call(t, "PATCH", "/v1/contacts/"+e.contactID, AnyMap{"title": "Head of Sales"},
			map[string]string{"If-Match": strconv.FormatInt(contact.Version-1, 10)}, nil)
		if status != http.StatusConflict && status != http.StatusPreconditionFailed {
			t.Errorf("a stale If-Match on a no-op save -> %d, want a version conflict", status)
		}
	})

	var company versionedWire
	if status := e.Call(t, "PATCH", "/v1/companies/"+e.companyID, AnyMap{"industry": "Software"},
		map[string]string{"If-Match": "1"}, &company); status != http.StatusOK {
		t.Fatalf("setting the company's industry -> %d", status)
	}
	t.Run("company", func(t *testing.T) {
		assertSaveChangesNothing(t, e.AppEnv, "PATCH", "/v1/companies/"+e.companyID,
			AnyMap{"industry": "Software"}, e.companyID, company.Version)
	})

	var partner versionedWire
	partnerBody := AnyMap{"partner_role": "consulting", "cert_status": "certified"}
	if status := e.Call(t, "PUT", "/v1/companies/"+e.companyID+"/partner", partnerBody, nil, &partner); status != http.StatusOK {
		t.Fatalf("promoting the company -> %d", status)
	}
	t.Run("partner", func(t *testing.T) {
		var standing struct {
			Version int64 `json:"version"`
		}
		if status := e.Call(t, "GET", "/v1/companies/"+e.companyID+"/partner", nil, nil, &standing); status != http.StatusOK {
			t.Fatalf("reading the partner -> %d", status)
		}
		assertSaveChangesNothing(t, e.AppEnv, "PUT", "/v1/companies/"+e.companyID+"/partner",
			partnerBody, e.companyID, standing.Version)
	})

	var edge versionedWire
	if status := e.Call(t, "POST", "/v1/relationships", AnyMap{
		"kind": "employment", "contact_id": e.contactID, "company_id": e.companyID,
		"role": "cto", "is_current_primary": true, "source": "manual",
	}, nil, &edge); status != http.StatusCreated {
		t.Fatalf("creating the employment -> %d", status)
	}
	t.Run("relationship with the stored role", func(t *testing.T) {
		assertSaveChangesNothing(t, e.AppEnv, "PATCH", "/v1/relationships/"+edge.ID,
			AnyMap{"role": "cto"}, edge.ID, edge.Version)
	})

	var tag versionedWire
	if status := e.Call(t, "POST", "/v1/tags", AnyMap{"name": "Product A"}, nil, &tag); status != http.StatusCreated {
		t.Fatalf("creating the tag -> %d", status)
	}
	t.Run("tag with the stored name", func(t *testing.T) {
		assertSaveChangesNothing(t, e.AppEnv, "PATCH", "/v1/tags/"+tag.ID,
			AnyMap{"name": "Product A"}, tag.ID, tag.Version)
	})
	t.Run("tag with no field", func(t *testing.T) {
		assertSaveChangesNothing(t, e.AppEnv, "PATCH", "/v1/tags/"+tag.ID, AnyMap{}, tag.ID, tag.Version)
	})
	t.Run("tag with its stored colour, description and suggestion switch", func(t *testing.T) {
		var set versionedWire
		stored := AnyMap{"color": "teal", "description": "Interest in product A", "suggestible": true}
		if status := e.Call(t, "PATCH", "/v1/tags/"+tag.ID, stored,
			map[string]string{"If-Match": strconv.FormatInt(tag.Version, 10)}, &set); status != http.StatusOK {
			t.Fatalf("setting the tag's fields -> %d", status)
		}
		assertSaveChangesNothing(t, e.AppEnv, "PATCH", "/v1/tags/"+tag.ID, stored, tag.ID, set.Version)
	})
	t.Run("a relationship patch that breaks its dates is still refused", func(t *testing.T) {
		var second, dated versionedWire
		if status := e.Call(t, "POST", "/v1/companies", AnyMap{"source": "manual", "display_name": "Dated Co"}, nil, &second); status != http.StatusCreated {
			t.Fatalf("creating the second company -> %d", status)
		}
		if status := e.Call(t, "POST", "/v1/relationships", AnyMap{
			"kind": "employment", "contact_id": e.contactID, "company_id": second.ID,
			"started_at": "2020-01-01", "source": "manual",
		}, nil, &dated); status != http.StatusCreated {
			t.Fatalf("creating the dated employment -> %d", status)
		}
		status := e.Call(t, "PATCH", "/v1/relationships/"+dated.ID, AnyMap{"ended_at": "2019-01-01"},
			map[string]string{"If-Match": strconv.FormatInt(dated.Version, 10)}, nil)
		if status < http.StatusBadRequest {
			t.Errorf("an end date before the start date -> %d, want a refusal", status)
		}
	})
}

func TestAWebhookSubscriptionSavedAsItAlreadyIsChangesNothing(t *testing.T) {
	cipher, err := webhooks.NewCipher(bytes.Repeat([]byte{0x5a}, webhooks.WebhookKeyBytes))
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	e := apptest.SetupAppWithOptions(t, compose.WithWebhookSigningKey(cipher))
	e.BootstrapWorkspace(t)

	var created struct {
		Subscription versionedWire `json:"subscription"`
	}
	if status := e.Call(t, "POST", "/v1/webhook-subscriptions", AnyMap{
		"target_url": "https://ok.example/hook", "event_types": []string{"deal.created"},
	}, nil, &created); status != http.StatusCreated {
		t.Fatalf("create subscription -> %d, want 201", status)
	}
	path := "/v1/webhook-subscriptions/" + created.Subscription.ID

	var paused struct {
		Subscription versionedWire `json:"subscription"`
	}
	if status := e.Call(t, "PATCH", path, AnyMap{"state": "paused"},
		map[string]string{"If-Match": strconv.FormatInt(created.Subscription.Version, 10)}, &paused); status != http.StatusOK {
		t.Fatalf("pausing -> %d", status)
	}
	version := paused.Subscription.Version
	if version == 0 {
		version = created.Subscription.Version + 1
	}
	assertSaveChangesNothing(t, e, "PATCH", path, AnyMap{"state": "paused"}, created.Subscription.ID, version)
}

func TestADealRoomExpirySetToItsStoredValueChangesNothing(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Room Expiry", "ada@expiry.test", "Ada Admin")
	_, roomID, version := openRoomOnANewDeal(t, e, "Expiry agreement")

	var set versionedWire
	body := AnyMap{"expires_at": "2027-01-15T10:00:00Z"}
	if status := e.Call(t, "PUT", "/v1/deal-rooms/"+roomID+"/expiry", body,
		map[string]string{"If-Match": strconv.FormatInt(version, 10)}, &set); status != http.StatusOK {
		t.Fatalf("setting the expiry -> %d", status)
	}
	assertSaveChangesNothing(t, e, "PUT", "/v1/deal-rooms/"+roomID+"/expiry",
		AnyMap{"expires_at": "2027-01-15T12:00:00+02:00"}, roomID, set.Version)
}

// The forms send every field and every replace-set on every save, so a save of
// an untouched record carries the lot.
func TestTheFullFormPayloadOfAnUntouchedRecordChangesNothing(t *testing.T) {
	e := setupRelationships(t)

	var contact versionedWire
	form := AnyMap{
		"full_name": "Edge Contact", "first_name": "Edge", "last_name": "Contact", "title": "Head of Sales",
		"social": AnyMap{"linkedin": "https://www.linkedin.com/in/edge"},
		"emails": []AnyMap{
			{"email": "edge@example.test", "email_type": "work", "is_primary": true, "position": 0},
			{"email": "edge.home@example.test", "email_type": "personal", "is_primary": false, "position": 1},
		},
		"phones": []AnyMap{{"phone": "+4930123456", "phone_type": "mobile", "is_primary": true, "position": 0}},
	}
	if status := e.Call(t, "PATCH", "/v1/contacts/"+e.contactID, form,
		map[string]string{"If-Match": "1"}, &contact); status != http.StatusOK {
		t.Fatalf("filling in the contact form -> %d", status)
	}
	t.Run("contact", func(t *testing.T) {
		assertSaveChangesNothing(t, e.AppEnv, "PATCH", "/v1/contacts/"+e.contactID, form, e.contactID, contact.Version)
	})

	var company versionedWire
	companyForm := AnyMap{
		"display_name": "Edge Company", "legal_name": "Edge Company GmbH", "industry": "Software",
		"description":        "Builds edges",
		"domains":            []AnyMap{{"domain": "edge.example", "is_primary": true}, {"domain": "edge.test", "is_primary": false}},
		"relationship_types": []string{"customer", "investor"},
		"address":            AnyMap{"city": "Berlin", "country": "DE"},
	}
	if status := e.Call(t, "PATCH", "/v1/companies/"+e.companyID, companyForm,
		map[string]string{"If-Match": "1"}, &company); status != http.StatusOK {
		t.Fatalf("filling in the company form -> %d", status)
	}
	t.Run("company", func(t *testing.T) {
		assertSaveChangesNothing(t, e.AppEnv, "PATCH", "/v1/companies/"+e.companyID, companyForm, e.companyID, company.Version)
	})
}
