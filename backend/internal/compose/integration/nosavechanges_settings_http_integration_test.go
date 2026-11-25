// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The probes for the settings, rooms, claims and messages a screen sends back
// as it read them. The route table they serve is noOpProbedRoutes.

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/clocktest"
)

// resend is one save of the values a record already holds.
type resend struct {
	method, path string
	body         AnyMap
	// version is the record's version, sent as If-Match and expected back.
	version int64
	// nested names the object of the answer that holds the record, for a
	// route that answers it inside a wrapper.
	nested string
	// about are the ids whose audit rows and events must not grow.
	about []string
}

// assertResendChangesNothing sends the save and checks the version, the audit
// rows and the outbox all stayed where they were.
func assertResendChangesNothing(t *testing.T, e *apptest.AppEnv, save resend) {
	t.Helper()
	count := func(query string) int {
		var n int
		if err := e.Owner.QueryRow(t.Context(), query, save.about).Scan(&n); err != nil {
			t.Fatalf("counting for %v: %v", save.about, err)
		}
		return n
	}
	const audits = `SELECT count(*) FROM audit_log WHERE entity_id::text = ANY($1::text[])`
	const events = `SELECT count(*) FROM event_outbox
		WHERE EXISTS (SELECT 1 FROM unnest($1::text[]) AS about(id) WHERE envelope::text LIKE '%' || about.id || '%')`
	auditsBefore, eventsBefore := count(audits), count(events)

	var answer map[string]json.RawMessage
	headers := map[string]string{"If-Match": strconv.FormatInt(save.version, 10)}
	if status := e.Call(t, save.method, save.path, save.body, headers, &answer); status != http.StatusOK {
		t.Fatalf("%s %s with the stored values -> %d, want 200", save.method, save.path, status)
	}
	if got := versionIn(t, answer, save.nested); got != save.version {
		t.Errorf("%s %s with the stored values answered version %d, want the unchanged %d",
			save.method, save.path, got, save.version)
	}
	if got := count(audits); got != auditsBefore {
		t.Errorf("%s %s with the stored values wrote %d audit row(s), want none", save.method, save.path, got-auditsBefore)
	}
	if got := count(events); got != eventsBefore {
		t.Errorf("%s %s with the stored values emitted %d event(s), want none", save.method, save.path, got-eventsBefore)
	}
}

func versionIn(t *testing.T, answer map[string]json.RawMessage, nested string) int64 {
	t.Helper()
	if nested != "" {
		var inner map[string]json.RawMessage
		if err := json.Unmarshal(answer[nested], &inner); err != nil {
			t.Fatalf("the answer has no %q object: %v", nested, err)
		}
		answer = inner
	}
	var version int64
	if err := json.Unmarshal(answer["version"], &version); err != nil {
		t.Fatalf("the answer carries no version: %v", err)
	}
	return version
}

// rowIDOf reads one row id the wire does not name, for the audit rows keyed by it.
func rowIDOf(t *testing.T, e *apptest.AppEnv, query, key string) string {
	t.Helper()
	var id string
	if err := e.Owner.QueryRow(t.Context(), query, key).Scan(&id); err != nil {
		t.Fatalf("reading the row id: %v", err)
	}
	return id
}

func TestSettingsSavedAsTheyAlreadyAreChangeNothing(t *testing.T) {
	e := setupRelationships(t)
	t.Run("stage, stage order and exit criterion", func(t *testing.T) { probeStageSaves(t, e.AppEnv) })
	t.Run("role and object grant", func(t *testing.T) { probeRoleSaves(t, e.AppEnv) })
	t.Run("saved view", func(t *testing.T) {
		query := AnyMap{"filter": AnyMap{"field": "created_at", "op": "gte", "value": "2000-01-01"}}
		var view versionedWire
		mustCall(t, e.AppEnv, "POST", "/v1/views", AnyMap{"resource": "contacts", "name": "Probe view", "query": query},
			http.StatusCreated, &view)
		assertResendChangesNothing(t, e.AppEnv, resend{method: "PATCH", path: "/v1/views/" + view.ID,
			body: AnyMap{"name": "Probe view", "query": query}, version: view.Version, about: []string{view.ID}})
	})
	t.Run("automation", func(t *testing.T) {
		var rule versionedWire
		mustCall(t, e.AppEnv, "POST", "/v1/automations", AnyMap{"key": "assign_lead_owner", "name": "Probe rule", "params": AnyMap{}},
			http.StatusCreated, &rule)
		assertResendChangesNothing(t, e.AppEnv, resend{method: "PATCH", path: "/v1/automations/" + rule.ID,
			body: AnyMap{"name": "Probe rule"}, version: rule.Version, about: []string{rule.ID}})
	})
	t.Run("signal", func(t *testing.T) {
		var signal versionedWire
		mustCall(t, e.AppEnv, "POST", "/v1/signals", AnyMap{
			"kind": "reengagement", "summary": "They wrote back", "source": "manual", "entity_type": "company", "entity_id": e.companyID,
		}, http.StatusCreated, &signal)
		assertResendChangesNothing(t, e.AppEnv, resend{method: "PATCH", path: "/v1/signals/" + signal.ID,
			body: AnyMap{"kind": "reengagement"}, version: signal.Version, about: []string{signal.ID}})
	})
	t.Run("sales initiative", func(t *testing.T) {
		definition := AnyMap{"name": "Probe drive", "objective_metric": "bookings_won", "deadline": "2099-12-31"}
		var drive versionedWire
		mustCall(t, e.AppEnv, "POST", "/v1/analytics/initiatives", definition, http.StatusCreated, &drive)
		assertResendChangesNothing(t, e.AppEnv, resend{method: "PATCH", path: "/v1/analytics/initiatives/" + drive.ID,
			body: definition, version: drive.Version, about: []string{drive.ID}})
	})
}

func probeStageSaves(t *testing.T, e *apptest.AppEnv) {
	t.Helper()
	var pipeline versionedWire
	mustCall(t, e, "POST", "/v1/pipelines", AnyMap{"name": "Probe ladder", "stages": []AnyMap{
		{"name": "Talking", "position": 1, "semantic": "open"}, {"name": "Agreeing", "position": 2, "semantic": "open"},
	}}, http.StatusCreated, &pipeline)
	var ladder struct {
		Version int64           `json:"version"`
		Stages  []versionedWire `json:"stages"`
	}
	mustCall(t, e, "GET", "/v1/pipelines/"+pipeline.ID, nil, http.StatusOK, &ladder)
	if len(ladder.Stages) != 2 {
		t.Fatalf("the probe ladder reads %d stages, want 2", len(ladder.Stages))
	}
	talking := ladder.Stages[0]
	assertResendChangesNothing(t, e, resend{method: "PATCH", path: "/v1/stages/" + talking.ID,
		body: AnyMap{"name": "Talking"}, version: talking.Version, about: []string{talking.ID}})
	assertResendChangesNothing(t, e, resend{method: "PUT", path: "/v1/pipelines/" + pipeline.ID + "/stage-order",
		body: AnyMap{"stage_ids": []string{ladder.Stages[0].ID, ladder.Stages[1].ID}}, version: ladder.Version,
		about: []string{pipeline.ID}})

	var criterion versionedWire
	mustCall(t, e, "POST", "/v1/stages/"+talking.ID+"/exit-criteria",
		AnyMap{"key": "budget_named", "label": "Budget named", "kind": "custom"}, http.StatusCreated, &criterion)
	assertResendChangesNothing(t, e, resend{method: "PATCH", path: "/v1/stages/" + talking.ID + "/exit-criteria/" + criterion.ID,
		body: AnyMap{"label": "Budget named"}, version: criterion.Version, about: []string{criterion.ID}})
}

func probeRoleSaves(t *testing.T, e *apptest.AppEnv) {
	t.Helper()
	var role struct {
		Key     string `json:"key"`
		Version int64  `json:"version"`
	}
	mustCall(t, e, "POST", "/v1/roles", AnyMap{"copy_from": "rep", "name": "Probe role"}, http.StatusCreated, &role)
	roleID := rowIDOf(t, e, `SELECT id::text FROM role WHERE key = $1`, role.Key)
	assertResendChangesNothing(t, e, resend{method: "PATCH", path: "/v1/roles/" + role.Key,
		body: AnyMap{"name": "Probe role"}, version: role.Version, about: []string{roleID}})

	grant := AnyMap{"create": false, "read": true, "update": false, "delete": false}
	var granted versionedWire
	if status := e.Call(t, "PATCH", "/v1/roles/"+role.Key+"/objects/contact", grant,
		withIfMatch(nil, role.Version), &granted); status != http.StatusOK {
		t.Fatalf("narrowing the role's contact grant -> %d", status)
	}
	assertResendChangesNothing(t, e, resend{method: "PATCH", path: "/v1/roles/" + role.Key + "/objects/contact",
		body: grant, version: granted.Version, about: []string{roleID}})
}

func TestACustomFieldSavedAsItAlreadyIsChangesNothing(t *testing.T) {
	e := apptest.SetupAppWithOptions(t, compose.WithSchemaPool(SchemaPool(t)))
	e.BootstrapWorkspace(t)
	var field versionedWire
	mustCall(t, e, "POST", "/v1/custom-fields", AnyMap{"object": "deal", "label": "Briefing", "type": "text", "source": "manual"},
		http.StatusCreated, &field)
	assertResendChangesNothing(t, e, resend{method: "PATCH", path: "/v1/custom-fields/" + field.ID,
		body: AnyMap{"label": "Briefing"}, version: field.Version, about: []string{field.ID}})
}

// A voice is the owner's own words and switches, saved from one settings page.
func TestAVoiceSavedAsItAlreadyIsChangesNothing(t *testing.T) {
	e := setupRelationships(t)
	var profile versionedWire
	mustCall(t, e.AppEnv, "POST", "/v1/voice-profiles", AnyMap{}, http.StatusCreated, &profile)
	path := "/v1/voice-profiles/" + profile.ID
	preferences := AnyMap{"personality_md": "Short and warm."}
	var styled versionedWire
	if status := e.Call(t, "PATCH", path, preferences, withIfMatch(nil, profile.Version), &styled); status != http.StatusOK {
		t.Fatalf("writing the preferences -> %d", status)
	}
	assertResendChangesNothing(t, e.AppEnv, resend{method: "PATCH", path: path, body: preferences,
		version: styled.Version, about: []string{profile.ID}})

	var filed struct {
		Source versionedWire `json:"source"`
	}
	mustCall(t, e.AppEnv, "POST", path+"/sources", AnyMap{
		"kind": "other", "source_label": "A note", "register": "general", "source_ref": "probe", "format": "text",
		"content": "These are the words I wrote myself and want kept exactly so.",
	}, http.StatusCreated, &filed)
	assertResendChangesNothing(t, e.AppEnv, resend{method: "PATCH", path: path + "/sources/" + filed.Source.ID,
		body: AnyMap{"included": true}, version: filed.Source.Version, nested: "source", about: []string{filed.Source.ID}})
}

// A correction that sends a claim back as a human already stated it verifies
// nothing new, so it stamps no second verification.
func TestACompanyClaimResentAsItStandsChangesNothing(t *testing.T) {
	e := setupRelationships(t)
	var fact struct {
		ValueKey string `json:"value_key"`
		Version  int64  `json:"version"`
	}
	mustCall(t, e.AppEnv, "POST", "/v1/companies/"+e.companyID+"/facts",
		AnyMap{"category": "company", "field": "employee_range", "value": "11-50"}, http.StatusCreated, &fact)
	factPath := "/v1/companies/" + e.companyID + "/facts/employee_range:" + fact.ValueKey
	var corrected versionedWire
	if status := e.Call(t, "PATCH", factPath, AnyMap{"value": "51-200"}, withIfMatch(nil, fact.Version), &corrected); status != http.StatusOK {
		t.Fatalf("correcting the fact -> %d", status)
	}
	factID := rowIDOf(t, e.AppEnv, `SELECT id::text FROM company_fact WHERE company_id = $1 AND field = 'employee_range'`, e.companyID)
	assertResendChangesNothing(t, e.AppEnv, resend{method: "PATCH", path: factPath, body: AnyMap{"value": "51-200"},
		version: corrected.Version, about: []string{factID, e.companyID}})

	fieldPath := "/v1/companies/" + e.companyID + "/profile-fields/legal_form"
	var stated versionedWire
	if status := e.Call(t, "PATCH", fieldPath, AnyMap{"value": "GmbH"}, nil, &stated); status != http.StatusOK {
		t.Fatalf("stating the legal form -> %d", status)
	}
	fieldID := rowIDOf(t, e.AppEnv, `SELECT id::text FROM company_profile_field WHERE company_id = $1 AND field = 'legal_form'`, e.companyID)
	assertResendChangesNothing(t, e.AppEnv, resend{method: "PATCH", path: fieldPath, body: AnyMap{"value": "GmbH"},
		version: stated.Version, about: []string{fieldID, e.companyID}})
}

func TestADealRoomAndAnOfferSavedAsTheyAlreadyAreChangeNothing(t *testing.T) {
	blob := blobstore.NewMemory()
	e := apptest.SetupAppWithOptions(t, compose.WithBlobstore(blob))
	e.BootstrapWorkspace(t)
	dealID, roomID, version := openRoomOnANewDeal(t, e, "Probe agreement")
	assertResendChangesNothing(t, e, resend{method: "PATCH", path: "/v1/deal-rooms/" + roomID,
		body: AnyMap{"title": "Probe agreement"}, version: version, about: []string{roomID}})

	_, attachmentID := captureFileOnEmail(t, e, blob, dealID)
	var document versionedWire
	mustCall(t, e, "POST", "/v1/deal-rooms/"+roomID+"/documents",
		AnyMap{"attachment_id": attachmentID, "group_key": "legal", "source": "manual"}, http.StatusCreated, &document)
	assertResendChangesNothing(t, e, resend{method: "PATCH", path: "/v1/deal-rooms/" + roomID + "/documents/" + document.ID,
		body: AnyMap{"group_key": "legal"}, version: document.Version, about: []string{document.ID}})

	offerID := offerValidUntil(t, e, offerFixture(t, e), "2099-12-31")
	var offer versionedWire
	mustCall(t, e, "GET", "/v1/offers/"+offerID, nil, http.StatusOK, &offer)
	assertResendChangesNothing(t, e, resend{method: "PATCH", path: "/v1/offers/" + offerID,
		body: AnyMap{"currency": "EUR", "valid_until": "2099-12-31"}, version: offer.Version, about: []string{offerID}})
}

// A composer saves the whole message every few seconds, and a rep may move a
// waiting message to the moment it already has.
func TestAMessageSavedAsItAlreadyIsChangesNothing(t *testing.T) {
	p := setupPreflight(t)
	t.Run("mail draft", func(t *testing.T) {
		status, saved := p.saveReplyDraft(t, "Half a thought", nil)
		if status != http.StatusOK {
			t.Fatalf("saving the first draft -> %d", status)
		}
		assertResendChangesNothing(t, p.AppEnv, resend{method: "PUT", path: "/v1/mail-drafts", body: AnyMap{
			"anchor_type": "activity", "anchor_id": p.activityID,
			"to": []string{"buyer@preflight.test"}, "subject": "Re: pricing", "body": "Half a thought",
		}, version: saved.Version, about: []string{saved.ID}})
	})
	t.Run("scheduled send", func(t *testing.T) {
		at := clocktest.Now(t).Add(48 * time.Hour).Truncate(time.Second)
		id := p.scheduleFor(t, at)
		var waiting versionedWire
		mustCall(t, p.AppEnv, "GET", "/v1/scheduled-sends/"+id.String(), nil, http.StatusOK, &waiting)
		assertResendChangesNothing(t, p.AppEnv, resend{method: "PATCH", path: "/v1/scheduled-sends/" + id.String(),
			body:    AnyMap{"scheduled_at": at.UTC().Format(time.RFC3339), "scheduled_tz": "Europe/Berlin"},
			version: waiting.Version, about: []string{id.String()}})
	})
	t.Run("audience, its members named in another order", func(t *testing.T) { probeAudienceSave(t, p) })
}

func probeAudienceSave(t *testing.T, p *preflightEnv) {
	t.Helper()
	var note versionedWire
	mustCall(t, p.AppEnv, "POST", "/v1/activities", AnyMap{
		"kind": "note", "source": "manual", "subject": "Pricing notes", "body": "Only the two of us.",
		"links": []AnyMap{{"entity_type": "contact", "entity_id": p.contactID}},
	}, http.StatusCreated, &note)
	var team versionedWire
	mustCall(t, p.AppEnv, "POST", "/v1/teams", AnyMap{"name": "Probe team"}, http.StatusCreated, &team)
	me := apptest.SessionUserID(t, p.AppEnv)
	path := "/v1/activities/" + note.ID + "/audience"
	var limited versionedWire
	if status := p.Call(t, "PATCH", path, AnyMap{"audience": "selected", "members": []AnyMap{
		{"subject_type": "user", "subject_id": me}, {"subject_type": "team", "subject_id": team.ID},
	}}, withIfMatch(nil, note.Version), &limited); status != http.StatusOK {
		t.Fatalf("limiting the note -> %d", status)
	}
	assertResendChangesNothing(t, p.AppEnv, resend{method: "PATCH", path: path, body: AnyMap{"audience": "selected", "members": []AnyMap{
		{"subject_type": "team", "subject_id": team.ID}, {"subject_type": "user", "subject_id": me},
	}}, version: limited.Version, about: []string{note.ID}})
}
