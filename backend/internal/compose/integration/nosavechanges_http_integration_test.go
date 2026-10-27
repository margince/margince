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
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/webhooks"
)

type versionedWire struct {
	ID      string `json:"id"`
	Version int64  `json:"version"`
}

func TestASaveOfTheStoredValuesChangesNothing(t *testing.T) {
	e := setupRelationships(t)

	var contact versionedWire
	if status := e.Call(t, "PATCH", "/v1/contacts/"+e.contactID, AnyMap{"title": "Head of Sales"},
		map[string]string{"If-Match": "1"}, &contact); status != http.StatusOK {
		t.Fatalf("setting the contact's title -> %d", status)
	}
	t.Run("contact", func(t *testing.T) {
		apptest.AssertSaveChangesNothing(t, e.AppEnv, "PATCH", "/v1/contacts/"+e.contactID,
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
		apptest.AssertSaveChangesNothing(t, e.AppEnv, "PATCH", "/v1/companies/"+e.companyID,
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
		apptest.AssertSaveChangesNothing(t, e.AppEnv, "PUT", "/v1/companies/"+e.companyID+"/partner",
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
		apptest.AssertSaveChangesNothing(t, e.AppEnv, "PATCH", "/v1/relationships/"+edge.ID,
			AnyMap{"role": "cto"}, edge.ID, edge.Version)
	})

	var tag versionedWire
	if status := e.Call(t, "POST", "/v1/tags", AnyMap{"name": "Product A"}, nil, &tag); status != http.StatusCreated {
		t.Fatalf("creating the tag -> %d", status)
	}
	t.Run("tag with the stored name", func(t *testing.T) {
		apptest.AssertSaveChangesNothing(t, e.AppEnv, "PATCH", "/v1/tags/"+tag.ID,
			AnyMap{"name": "Product A"}, tag.ID, tag.Version)
	})
	t.Run("tag with no field", func(t *testing.T) {
		apptest.AssertSaveChangesNothing(t, e.AppEnv, "PATCH", "/v1/tags/"+tag.ID, AnyMap{}, tag.ID, tag.Version)
	})
	t.Run("tag with its stored colour, description and suggestion switch", func(t *testing.T) {
		var set versionedWire
		stored := AnyMap{"color": "teal", "description": "Interest in product A", "suggestible": true}
		if status := e.Call(t, "PATCH", "/v1/tags/"+tag.ID, stored,
			map[string]string{"If-Match": strconv.FormatInt(tag.Version, 10)}, &set); status != http.StatusOK {
			t.Fatalf("setting the tag's fields -> %d", status)
		}
		apptest.AssertSaveChangesNothing(t, e.AppEnv, "PATCH", "/v1/tags/"+tag.ID, stored, tag.ID, set.Version)
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

	var paused versionedWire
	if status := e.Call(t, "PATCH", path, AnyMap{"state": "paused"},
		map[string]string{"If-Match": strconv.FormatInt(created.Subscription.Version, 10)}, &paused); status != http.StatusOK {
		t.Fatalf("pausing -> %d", status)
	}
	version := paused.Version
	apptest.AssertSaveChangesNothing(t, e, "PATCH", path, AnyMap{"state": "paused"}, created.Subscription.ID, version)
}

// Clearing the headers a subscription sends is a change. The tokens they carry
// must stop being sent, so an empty list is not "nothing to do".
func TestClearingAWebhooksCustomHeadersIsAChange(t *testing.T) {
	cipher, err := webhooks.NewCipher(bytes.Repeat([]byte{0x5a}, webhooks.WebhookKeyBytes))
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	e := apptest.SetupAppWithOptions(t, compose.WithWebhookSigningKey(cipher))
	e.BootstrapWorkspace(t)

	var created struct {
		Subscription struct {
			ID      string `json:"id"`
			Version int64  `json:"version"`
		} `json:"subscription"`
	}
	if status := e.Call(t, "POST", "/v1/webhook-subscriptions", AnyMap{
		"target_url": "https://ok.example/hook", "event_types": []string{"deal.created"},
		"custom_headers": []AnyMap{{"name": "X-Token", "value": "secret"}},
	}, nil, &created); status != http.StatusCreated {
		t.Fatalf("create subscription -> %d, want 201", status)
	}
	path := "/v1/webhook-subscriptions/" + created.Subscription.ID
	var cleared struct {
		Version int64 `json:"version"`
		Headers []struct {
			Name string `json:"name"`
		} `json:"custom_headers"`
	}
	if status := e.Call(t, "PATCH", path, AnyMap{"custom_headers": []AnyMap{}},
		map[string]string{"If-Match": strconv.FormatInt(created.Subscription.Version, 10)}, &cleared); status != http.StatusOK {
		t.Fatalf("clearing the headers -> %d", status)
	}
	if cleared.Version != created.Subscription.Version+1 || len(cleared.Headers) != 0 {
		t.Errorf("clearing the headers answered version %d with headers %v, want version %d and none",
			cleared.Version, cleared.Headers, created.Subscription.Version+1)
	}
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
	apptest.AssertSaveChangesNothing(t, e, "PUT", "/v1/deal-rooms/"+roomID+"/expiry",
		AnyMap{"expires_at": "2027-01-15T12:00:00+02:00"}, roomID, set.Version)

	stale := map[string]string{"If-Match": strconv.FormatInt(set.Version-1, 10)}
	if status := e.Call(t, "PUT", "/v1/deal-rooms/"+roomID+"/expiry", body, stale, nil); status != http.StatusConflict &&
		status != http.StatusPreconditionFailed {
		t.Errorf("a stale If-Match on a save of the stored expiry -> %d, want a version conflict", status)
	}
}

// noOpProbedRoutes names the test that sends each listed route a save of the
// values its record already holds. The gate that reads it derives the routes
// from the contract. A new versioned PATCH or PUT joins this table or is
// ratified there with a reason.
//
// gatekit:fixture the routes an HTTP probe covers and the test that probes each, expected data checked against the contract
var noOpProbedRoutes = map[string]string{
	"PATCH /contacts/{id}":              "TestTheFullFormPayloadOfAnUntouchedRecordChangesNothing",
	"PATCH /companies/{id}":             "TestTheFullFormPayloadOfAnUntouchedRecordChangesNothing",
	"PUT /companies/{id}/partner":       "TestASaveOfTheStoredValuesChangesNothing",
	"PATCH /relationships/{id}":         "TestASaveOfTheStoredValuesChangesNothing",
	"PATCH /tags/{id}":                  "TestASaveOfTheStoredValuesChangesNothing",
	"PATCH /webhook-subscriptions/{id}": "TestAWebhookSubscriptionSavedAsItAlreadyIsChangesNothing",
	"PATCH /web-forms/{id}":             "TestAWebsiteFormSavedAsItAlreadyIsChangesNothing",
	"PUT /deal-rooms/{id}/expiry":       "TestADealRoomExpirySetToItsStoredValueChangesNothing",
	"PUT /offer-templates/{id}":         "TestOtherRecordsSavedAsTheyAlreadyAreChangeNothing",
	"PATCH /products/{id}":              "TestOtherRecordsSavedAsTheyAlreadyAreChangeNothing",
	"PATCH /activities/{id}":            "TestOtherRecordsSavedAsTheyAlreadyAreChangeNothing",
	"PATCH /leads/{id}":                 "TestOtherRecordsSavedAsTheyAlreadyAreChangeNothing",
	"PATCH /deals/{id}":                 "TestOtherRecordsSavedAsTheyAlreadyAreChangeNothing",
	"PATCH /projects/{id}":              "TestOtherRecordsSavedAsTheyAlreadyAreChangeNothing",
	"PATCH /contracts/{id}":             "TestOtherRecordsSavedAsTheyAlreadyAreChangeNothing",
	"PATCH /legal-entities/{id}":        "TestOtherRecordsSavedAsTheyAlreadyAreChangeNothing",
}

// Every probed route is a PATCH or a PUT. The contract-side check lives in the
// gates package, which reads this table from source.
func TestTheProbeTableListsOnlySaveRoutes(t *testing.T) {
	for route, probe := range noOpProbedRoutes {
		if !strings.HasPrefix(route, "PATCH /") && !strings.HasPrefix(route, "PUT /") {
			t.Errorf("%q is not a PATCH or PUT route", route)
		}
		if !strings.HasPrefix(probe, "Test") {
			t.Errorf("%s names %q, which is not a test", route, probe)
		}
	}
}

func TestOtherRecordsSavedAsTheyAlreadyAreChangeNothing(t *testing.T) {
	e := setupRelationships(t)

	t.Run("offer template", func(t *testing.T) {
		var tpl versionedWire
		body := AnyMap{"name": "Standard", "locale": "en-US", "is_default": false, "layout": AnyMap{"footer": "Thanks"}}
		if status := e.Call(t, "POST", "/v1/offer-templates", body, nil, &tpl); status != http.StatusCreated {
			t.Fatalf("creating the template -> %d", status)
		}
		apptest.AssertSaveChangesNothing(t, e.AppEnv, "PUT", "/v1/offer-templates/"+tpl.ID, body, tpl.ID, tpl.Version)
		renamed := AnyMap{"name": "Standard v2", "locale": "en-US", "is_default": false, "layout": AnyMap{"footer": "Thanks"}}
		var moved versionedWire
		if status := e.Call(t, "PUT", "/v1/offer-templates/"+tpl.ID, renamed,
			map[string]string{"If-Match": strconv.FormatInt(tpl.Version, 10)}, &moved); status != http.StatusOK ||
			moved.Version != tpl.Version+1 {
			t.Errorf("renaming the template -> %d at version %d, want 200 at version %d", status, moved.Version, tpl.Version+1)
		}
		if status := e.Call(t, "PUT", "/v1/offer-templates/"+tpl.ID, renamed,
			map[string]string{"If-Match": strconv.FormatInt(tpl.Version, 10)}, nil); status != http.StatusConflict &&
			status != http.StatusPreconditionFailed {
			t.Errorf("a stale If-Match on a save of the stored template -> %d, want a version conflict", status)
		}
	})
	t.Run("product", func(t *testing.T) {
		var product versionedWire
		body := AnyMap{"name": "Seat", "currency": "EUR", "unit_price_minor": 1000, "default_tax_rate": 19, "source": "manual"}
		if status := e.Call(t, "POST", "/v1/products", body, nil, &product); status != http.StatusCreated {
			t.Fatalf("creating the product -> %d", status)
		}
		apptest.AssertSaveChangesNothing(t, e.AppEnv, "PATCH", "/v1/products/"+product.ID,
			AnyMap{"name": "Seat", "currency": "EUR", "unit_price_minor": 1000, "default_tax_rate": 19.0}, product.ID, product.Version)
	})
	t.Run("activity", func(t *testing.T) {
		var note versionedWire
		if status := e.Call(t, "POST", "/v1/activities", AnyMap{
			"kind": "note", "source": "manual", "subject": "Call recap", "body": "We agreed on Friday.",
			"links": []AnyMap{{"entity_type": "contact", "entity_id": e.contactID}},
		}, nil, &note); status != http.StatusCreated {
			t.Fatalf("logging the note -> %d", status)
		}
		apptest.AssertSaveChangesNothing(t, e.AppEnv, "PATCH", "/v1/activities/"+note.ID,
			AnyMap{"subject": "Call recap", "body": "We agreed on Friday."}, note.ID, note.Version)
	})
	t.Run("lead", func(t *testing.T) {
		var lead versionedWire
		if status := e.Call(t, "POST", "/v1/leads", AnyMap{"full_name": "Probe Lead", "source": "manual"}, nil, &lead); status != http.StatusCreated {
			t.Fatalf("creating the lead -> %d", status)
		}
		apptest.AssertSaveChangesNothing(t, e.AppEnv, "PATCH", "/v1/leads/"+lead.ID,
			AnyMap{"full_name": "Probe Lead"}, lead.ID, lead.Version)
	})
	t.Run("project", func(t *testing.T) {
		var project versionedWire
		if status := e.Call(t, "POST", "/v1/projects", AnyMap{
			"name": "Joint rollout", "company_id": e.companyID, "source": "manual",
		}, nil, &project); status != http.StatusCreated {
			t.Fatalf("creating the project -> %d", status)
		}
		apptest.AssertSaveChangesNothing(t, e.AppEnv, "PATCH", "/v1/projects/"+project.ID,
			AnyMap{"name": "Joint rollout"}, project.ID, project.Version)
	})
	t.Run("contract with its dates", func(t *testing.T) {
		var contract versionedWire
		if status := e.Call(t, "POST", "/v1/contracts", AnyMap{
			"company_id": e.companyID, "title": "MSA 2026", "value_basis": "total",
			"starts_on": "2026-01-01", "ends_on": "2026-12-31", "renewal_on": "2026-10-01",
		}, nil, &contract); status != http.StatusCreated {
			t.Fatalf("creating the contract -> %d", status)
		}
		apptest.AssertSaveChangesNothing(t, e.AppEnv, "PATCH", "/v1/contracts/"+contract.ID,
			AnyMap{"title": "MSA 2026", "starts_on": "2026-01-01", "ends_on": "2026-12-31", "renewal_on": "2026-10-01"},
			contract.ID, contract.Version)
	})
	t.Run("deal", func(t *testing.T) {
		stages := apptest.DiscoverSeededPipeline(t, e.AppEnv)
		dealID := apptest.CreateOpenDeal(t, e.AppEnv, stages)
		var deal versionedWire
		if status := e.Call(t, "GET", "/v1/deals/"+dealID, nil, nil, &deal); status != http.StatusOK {
			t.Fatalf("reading the deal -> %d", status)
		}
		apptest.AssertSaveChangesNothing(t, e.AppEnv, "PATCH", "/v1/deals/"+dealID,
			AnyMap{"name": "Acme rollout", "amount_minor": 250_000_00, "currency": "EUR"}, dealID, deal.Version)
	})
	t.Run("legal entity", func(t *testing.T) {
		body := AnyMap{"display_name": "Acme Wien", "legal_name": "Acme Wien GmbH", "country": "at"}
		var entity versionedWire
		if status := e.Call(t, "POST", "/v1/legal-entities", body, nil, &entity); status != http.StatusCreated {
			t.Fatalf("creating the legal entity -> %d", status)
		}
		apptest.AssertSaveChangesNothing(t, e.AppEnv, "PATCH", "/v1/legal-entities/"+entity.ID, body, entity.ID, entity.Version)
	})
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
		apptest.AssertSaveChangesNothing(t, e.AppEnv, "PATCH", "/v1/contacts/"+e.contactID, form, e.contactID, contact.Version)
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
		apptest.AssertSaveChangesNothing(t, e.AppEnv, "PATCH", "/v1/companies/"+e.companyID, companyForm, e.companyID, company.Version)
	})
	t.Run("a company whose sets changed is a change", func(t *testing.T) {
		changed := AnyMap{
			"display_name": "Edge Company",
			"domains":      []AnyMap{{"domain": "edge.example", "is_primary": true}, {"domain": "edge.org", "is_primary": false}},
		}
		var moved versionedWire
		if status := e.Call(t, "PATCH", "/v1/companies/"+e.companyID, changed,
			map[string]string{"If-Match": strconv.FormatInt(company.Version, 10)}, &moved); status != http.StatusOK ||
			moved.Version != company.Version+1 {
			t.Errorf("adding a domain -> %d at version %d, want 200 at version %d", status, moved.Version, company.Version+1)
		}
		types := AnyMap{"relationship_types": []string{"customer"}}
		var retyped versionedWire
		if status := e.Call(t, "PATCH", "/v1/companies/"+e.companyID, types,
			map[string]string{"If-Match": strconv.FormatInt(moved.Version, 10)}, &retyped); status != http.StatusOK ||
			retyped.Version != moved.Version+1 {
			t.Errorf("dropping a relationship type -> %d at version %d, want 200 at version %d", status, retyped.Version, moved.Version+1)
		}
	})
}
