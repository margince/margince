// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// Saving several per-purpose choices at once, from the public preference
// centre.
//
// Two rules govern the order things land in, and both exist because the save
// is a LOOP that stops at the first refusal — so what a refusal costs depends
// entirely on what has already been written when it happens.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gradionhq/margince/backend/internal/platform/database"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/principal"
)

// preferenceSave posts one body to UpdatePreferences and returns the response.
func preferenceSave(t *testing.T, e *channelConsentEnv, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	h := NewHandlers(database.BindTo(e.store.db.Pool(), ids.From[ids.WorkspaceKind](e.ws)))
	// The context the public edge hands the handler: the installation's
	// workspace from the identity middleware, and the system principal
	// compose/publicpreferences.go binds — the token IS the authorization on
	// this edge, so there is no user behind it. Built here rather than by
	// calling the middleware, which lives in compose and cannot be imported by
	// a module test.
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem,
		ID:   "system:public_preferences",
	})
	req := httptest.NewRequest(http.MethodPut, "/v1/public/preferences/"+token, strings.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.UpdatePreferences(rec, req, token)
	return rec
}

// seedPreferenceToken mints the emailed capability the centre is reached with.
func seedPreferenceToken(t *testing.T, e *channelConsentEnv) string {
	t.Helper()
	token := "pref-" + ids.NewV7().String()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO preference_token (person_id, token, expires_at)
		 VALUES ($1, $2, now() + interval '30 days')`, e.person, token); err != nil {
		t.Fatalf("seed preference token: %v", err)
	}
	return token
}

func consentStateOf(t *testing.T, e *channelConsentEnv, purpose ids.PurposeID) string {
	t.Helper()
	var state string
	err := e.owner.QueryRow(context.Background(),
		`SELECT coalesce(max(state), '') FROM person_consent WHERE person_id = $1 AND purpose_id = $2`,
		e.person, purpose).Scan(&state)
	if err != nil {
		t.Fatalf("read person_consent: %v", err)
	}
	return state
}

// A refused GRANT must not cost the WITHDRAWAL saved beside it. The subject is
// archived, so the grant is refused — and a save that recorded choices in
// request order would stop there and drop the suppression, which is the one
// thing somebody in that state most needs this page to do.
func TestASaveRecordsItsWithdrawalsEvenWhenAGrantIsRefused(t *testing.T) {
	e := setupChannelConsent(t)
	archiveConsentSubject(t, e.owner, "person", e.person.UUID)
	token := seedPreferenceToken(t, e)

	// The grant is listed FIRST on purpose: in request order it refuses before
	// the withdrawal is ever reached.
	rec := preferenceSave(t, e, token, `{"choices":[
		{"purpose_key":"doi_newsletter","state":"granted"},
		{"purpose_key":"newsletter","state":"withdrawn"}]}`)

	if rec.Code == http.StatusOK {
		t.Errorf("status = %d — a grant for an archived subject must still be refused", rec.Code)
	}
	if got := consentStateOf(t, e, e.newsletter); got != string(StateWithdrawn) {
		t.Errorf("newsletter = %q, want withdrawn — the refused grant swallowed the withdrawal beside it", got)
	}
	if got := consentStateOf(t, e, e.doiNews); got == string(StateGranted) {
		t.Error("the grant landed for an archived subject")
	}
}

// One purpose named twice in a single body is a client bug, and on a consent
// surface the safe reading of it is the suppressing one. Request order used to
// decide it by accident; the withdrawals-first pass would decide it the other
// way, so it is settled explicitly before anything is written.
func TestAPurposeNamedTwiceInOneSaveSettlesOnTheWithdrawal(t *testing.T) {
	e := setupChannelConsent(t)
	token := seedPreferenceToken(t, e)

	// Withdrawn first, granted second: in either request order or a naive
	// withdrawals-first pass, the grant would be the one left standing.
	rec := preferenceSave(t, e, token, `{"choices":[
		{"purpose_key":"newsletter","state":"withdrawn"},
		{"purpose_key":"NEWSLETTER ","state":"granted"}]}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	if got := consentStateOf(t, e, e.newsletter); got != string(StateWithdrawn) {
		t.Errorf("newsletter = %q, want withdrawn — a purpose named twice must settle on the suppression, "+
			"and the second spelling differs only in case and trailing space", got)
	}
}

// The ordinary case still works: a save of unambiguous choices records them all.
func TestASaveRecordsEveryDistinctChoice(t *testing.T) {
	e := setupChannelConsent(t)
	token := seedPreferenceToken(t, e)

	rec := preferenceSave(t, e, token, `{"choices":[
		{"purpose_key":"newsletter","state":"granted"},
		{"purpose_key":"doi_newsletter","state":"withdrawn"}]}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := consentStateOf(t, e, e.newsletter); got != string(StateGranted) {
		t.Errorf("newsletter = %q, want granted", got)
	}
	if got := consentStateOf(t, e, e.doiNews); got != string(StateWithdrawn) {
		t.Errorf("doi_newsletter = %q, want withdrawn", got)
	}
}
