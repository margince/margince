// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// An automatic confirmation performs the WHOLE write or it is not a
// confirmation (DECISIONS A143, amending ADR-0078 §8). What has to hold:
//
//   - a confirm on either tier leaves the connection's profile URL on the
//     contact, an audit row on the connection AND on the contact, and the
//     linkedin_match.decided and person.updated events — the same write a
//     human's approval releases;
//   - a caller who may read a contact but not edit one confirms NOTHING: its
//     matches degrade to suggestions and it never touches a contact;
//   - two of the owner's ghosts of the same exact name and employer pointing at
//     one contact are ambiguous — neither auto-confirms, both stay suggestions.

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAnAutomaticEmailConfirmPerformsTheWholeWrite(t *testing.T) {
	e := setupDedupe(t)
	dana := e.seedContact(t, "Dana Buyer")
	e.seedEmail(t, dana, "dana@acme.test")
	const url = "https://www.linkedin.com/in/danabuyer"
	e.seedEmailGhost(t, "Dana Buyer", "dana@acme.test", url)

	res, err := e.store.MatchLinkedInConnections(e.as(), e.rep)
	if err != nil {
		t.Fatalf("matching: %v", err)
	}
	if res.Confirmed != 1 || res.Suggested != 0 {
		t.Fatalf("the pass reported %+v, want 1 confirmed and 0 suggested", res)
	}
	if status, person := e.ghostStatus(t, "Dana Buyer"); status != "confirmed" || person == nil || *person != dana.UUID {
		t.Fatalf("the address match is %q → %v, want confirmed → %s", status, person, dana)
	}
	assertConfirmWroteEverything(t, e, dana, e.ghostConnectionID(t, "Dana Buyer"), url)
}

func TestAnAutomaticExactNameConfirmPerformsTheWholeWrite(t *testing.T) {
	e := setupDedupe(t)
	org := e.seedOrgNamed(t, "Acme GmbH")
	andreas := e.seedContact(t, "Andreas Müller")
	e.employ(t, andreas, org)
	const url = "https://www.linkedin.com/in/amueller"
	e.seedNameGhost(t, "Andreas Müller", "andreas muller", url, org, "2023-02-02")

	res, err := e.store.MatchLinkedInConnections(e.as(), e.rep)
	if err != nil {
		t.Fatalf("matching: %v", err)
	}
	if res.Confirmed != 1 || res.Suggested != 0 {
		t.Fatalf("the pass reported %+v, want 1 confirmed and 0 suggested", res)
	}
	if status, person := e.ghostStatus(t, "Andreas Müller"); status != "confirmed" || person == nil || *person != andreas.UUID {
		t.Fatalf("the exact-name match is %q → %v, want confirmed → %s", status, person, andreas)
	}
	assertConfirmWroteEverything(t, e, andreas, e.ghostConnectionID(t, "Andreas Müller"), url)
}

// A read-only member still sweeps — its network is matched — but a confirm
// edits a contact, so its matches can only ever be suggestions. Anything else
// would let a grant that may not edit a contact cause a contact edit.
func TestAReadOnlyCallerConfirmsNothingAndEditsNoContact(t *testing.T) {
	e := setupDedupe(t)
	org := e.seedOrgNamed(t, "Acme GmbH")

	// Both a case that WOULD confirm under an update grant: an address match and
	// an exact name at a matched employer.
	dana := e.seedContact(t, "Dana Buyer")
	e.seedEmail(t, dana, "dana@acme.test")
	e.seedEmailGhost(t, "Dana Buyer", "dana@acme.test", "https://www.linkedin.com/in/danabuyer")
	bruno := e.seedContact(t, "Bruno Weber")
	e.employ(t, bruno, org)
	e.seedNameGhost(t, "Bruno Weber", "bruno weber", "https://www.linkedin.com/in/bweber", org, "2023-02-02")

	res, err := e.store.MatchLinkedInConnections(e.asPersonReadOnly(), e.rep)
	if err != nil {
		t.Fatalf("matching read-only: %v", err)
	}
	if res.Confirmed != 0 || res.Suggested != 2 {
		t.Fatalf("the read-only pass reported %+v, want 0 confirmed and 2 suggested", res)
	}
	for _, name := range []string{"Dana Buyer", "Bruno Weber"} {
		if status, person := e.ghostStatus(t, name); status != "suggested" || person == nil {
			t.Errorf("%s is %q → %v, want suggested with a contact named", name, status, person)
		}
	}
	// The contact edit that a confirm would have made must be entirely absent.
	for _, p := range []ids.PersonID{dana, bruno} {
		if _, found := e.linkedInHandle(t, p); found {
			t.Errorf("%s gained a LinkedIn handle from a read-only sweep — a member who may not edit a contact caused a contact edit", p)
		}
		if n := e.auditCount(t, "person", p.UUID); n != 0 {
			t.Errorf("%s gained %d person audit rows from a read-only sweep, want 0", p, n)
		}
	}
	if n := e.eventCount(t, "linkedin_match.decided"); n != 0 {
		t.Errorf("a read-only sweep emitted %d decided events, want 0 — it confirmed nothing", n)
	}
}

// Two of the owner's ghosts of one exact name at one employer, pointing at one
// contact, are as ambiguous as one ghost pointing at two contacts: a confirm
// would guess which connection is really that person. Neither confirms, and
// each stays a suggestion a human can judge.
func TestTwoGhostsOntoOneContactBothStaySuggestions(t *testing.T) {
	e := setupDedupe(t)
	org := e.seedOrgNamed(t, "Acme GmbH")
	andreas := e.seedContact(t, "Andreas Müller")
	e.employ(t, andreas, org)
	// Same owner, same exact name, same employer — distinct only by the date the
	// connection was made, which is what keeps them two rows under the natural
	// key rather than one.
	e.seedNameGhost(t, "Andreas Müller", "andreas muller", "https://www.linkedin.com/in/amueller-1", org, "2023-02-02")
	e.seedNameGhost(t, "Andreas Müller", "andreas muller", "https://www.linkedin.com/in/amueller-2", org, "2024-05-05")

	res, err := e.store.MatchLinkedInConnections(e.as(), e.rep)
	if err != nil {
		t.Fatalf("matching: %v", err)
	}
	if res.Confirmed != 0 || res.Suggested != 2 {
		t.Fatalf("the pass reported %+v, want 0 confirmed and 2 suggested", res)
	}
	var confirmed int
	if err := e.store.tx(e.as(), func(tx pgx.Tx) error {
		return tx.QueryRow(e.as(),
			`SELECT count(*) FROM linkedin_connection WHERE full_name = 'Andreas Müller' AND match_status = 'confirmed'`).
			Scan(&confirmed)
	}); err != nil {
		t.Fatal(err)
	}
	if confirmed != 0 {
		t.Errorf("%d of the two same-named ghosts auto-confirmed; picking one is a guess wearing a confirmation's clothes", confirmed)
	}
	if _, found := e.linkedInHandle(t, andreas); found {
		t.Error("the contact gained a handle from an ambiguous match that confirmed nothing")
	}
}

// One contact is not two of a colleague's connections. When the address tier
// confirms a contact in a pass, a name+employer ghost of the owner's pointing at
// that same contact is withheld — the in-pass form of the guard the name SELECT
// applies to confirms from earlier passes.
func TestAContactTheAddressTierConfirmsIsNotAlsoNameSuggested(t *testing.T) {
	e := setupDedupe(t)
	org := e.seedOrgNamed(t, "Acme GmbH")
	carla := e.seedContact(t, "Carla Stein")
	e.seedEmail(t, carla, "carla@acme.test")
	e.employ(t, carla, org)
	// Two of the owner's connections resolving to Carla: one carries her
	// address, one her exact name at her employer.
	e.seedEmailGhost(t, "Carla Stein", "carla@acme.test", "https://www.linkedin.com/in/carla-mail")
	e.seedNameGhost(t, "Carla Stein", "carla stein", "https://www.linkedin.com/in/carla-name", org, "2022-04-04")

	res, err := e.store.MatchLinkedInConnections(e.as(), e.rep)
	if err != nil {
		t.Fatalf("matching: %v", err)
	}
	if res.Confirmed != 1 || res.Suggested != 0 {
		t.Fatalf("the pass reported %+v, want 1 confirmed and 0 suggested — the name ghost must be withheld", res)
	}
	var emailGhost, nameGhost string
	if err := e.store.tx(e.as(), func(tx pgx.Tx) error {
		if err := tx.QueryRow(e.as(),
			`SELECT match_status FROM linkedin_connection WHERE full_name = 'Carla Stein' AND email IS NOT NULL`).
			Scan(&emailGhost); err != nil {
			return err
		}
		return tx.QueryRow(e.as(),
			`SELECT match_status FROM linkedin_connection WHERE full_name = 'Carla Stein' AND email IS NULL`).
			Scan(&nameGhost)
	}); err != nil {
		t.Fatal(err)
	}
	if emailGhost != "confirmed" {
		t.Errorf("the address ghost is %q, want confirmed", emailGhost)
	}
	if nameGhost != "unmatched" {
		t.Errorf("the name ghost is %q, want unmatched — a contact just confirmed is not also proposed", nameGhost)
	}
}

// One contact is not two of a colleague's connections, on the address tier as
// on the name tier: a second connection carrying an address already confirmed
// against a contact is not confirmed onto them again — it would double-count the
// colleague's reach into that contact's account.
func TestAnAddressIsNotConfirmedOntoAContactAlreadyMet(t *testing.T) {
	e := setupDedupe(t)
	dana := e.seedContact(t, "Dana Buyer")
	e.seedEmail(t, dana, "dana@acme.test")
	// The owner already has a confirmed connection to Dana.
	if err := e.store.tx(e.as(), func(tx pgx.Tx) error {
		_, err := tx.Exec(e.as(), `
			INSERT INTO linkedin_connection
			  (owner_user_id, full_name, normalized_name, matched_person_id, match_status, source)
			VALUES ($1, 'Dana Buyer', 'dana buyer', $2, 'confirmed', 'csv_export')`,
			e.rep, dana)
		return err
	}); err != nil {
		t.Fatalf("seeding the confirmed connection: %v", err)
	}
	// A second connection of the owner's carrying the same address.
	e.seedEmailGhost(t, "Dana B Buyer", "dana@acme.test", "https://www.linkedin.com/in/dana-2")

	res, err := e.store.MatchLinkedInConnections(e.as(), e.rep)
	if err != nil {
		t.Fatalf("matching: %v", err)
	}
	if res.Confirmed != 0 {
		t.Errorf("the pass reported %+v, want nothing confirmed — the contact is already met", res)
	}
	if status, _ := e.ghostStatus(t, "Dana B Buyer"); status != "unmatched" {
		t.Errorf("the duplicate connection is %q, want unmatched — one contact is not two of a colleague's connections", status)
	}
}

// assertConfirmWroteEverything checks the whole write shape a confirmation owes:
// the handle on the contact, an audit row on both the connection and the
// contact, and the two events.
func assertConfirmWroteEverything(t *testing.T, e *dedupeEnv, person ids.PersonID, connection ids.UUID, wantURL string) {
	t.Helper()
	handle, found := e.linkedInHandle(t, person)
	if !found || handle != wantURL {
		t.Errorf("the contact carries handle %q (found=%v), want the connection's own URL %q", handle, found, wantURL)
	}
	if n := e.auditCount(t, "linkedin_connection", connection); n != 1 {
		t.Errorf("the confirmed connection has %d update audit rows, want 1", n)
	}
	if n := e.auditCount(t, "person", person.UUID); n != 1 {
		t.Errorf("the contact has %d update audit rows for the handle it gained, want 1", n)
	}
	if n := e.eventCount(t, "linkedin_match.decided"); n != 1 {
		t.Errorf("the confirm emitted %d linkedin_match.decided events, want 1", n)
	}
	if n := e.eventCount(t, "person.updated"); n < 1 {
		t.Errorf("the confirm emitted %d person.updated events, want at least the handle gain", n)
	}
}

// asPersonReadOnly is the same member, granted to READ contacts but not edit
// them. A confirm edits a contact, so this caller must only ever suggest.
func (e *dedupeEnv) asPersonReadOnly() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.rep.String(), UserID: e.rep,
		Permissions: principal.Permissions{
			RoleKeys: []string{"viewer"},
			Objects: map[string]principal.ObjectGrant{
				"person":       {Read: true},
				"organization": {Read: true},
				"relationship": {Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}

// seedEmailGhost writes an unmatched connection carrying an address — the one
// evidence the address tier confirms on.
func (e *dedupeEnv) seedEmailGhost(t *testing.T, fullName, email, profileURL string) {
	t.Helper()
	ctx := e.as()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO linkedin_connection
			  (owner_user_id, full_name, normalized_name, email, profile_url, match_status, source)
			VALUES ($1, $2, $3, $4, $5, 'unmatched', 'csv_export')`,
			e.rep, fullName, strings.ToLower(fullName), email, profileURL)
		return err
	}); err != nil {
		t.Fatalf("seeding email ghost %s: %v", fullName, err)
	}
}

// seedNameGhost writes an unmatched connection already resolved to an employer,
// so the name-and-employer tier matches it without the org-name normalizer in
// the loop. connectedOn keeps two same-named ghosts distinct under the natural
// key.
func (e *dedupeEnv) seedNameGhost(t *testing.T, fullName, normalizedName, profileURL string, org ids.OrganizationID, connectedOn string) {
	t.Helper()
	ctx := e.as()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO linkedin_connection
			  (owner_user_id, full_name, normalized_name, company_name, normalized_company,
			   matched_org_id, connected_on, profile_url, match_status, source)
			VALUES ($1, $2, $3, 'Acme GmbH', 'acme gmbh', $4, $5::date, $6, 'unmatched', 'csv_export')`,
			e.rep, fullName, normalizedName, org, connectedOn, profileURL)
		return err
	}); err != nil {
		t.Fatalf("seeding name ghost %s: %v", fullName, err)
	}
}

// ghostConnectionID reads back one connection's id by the name it carries.
func (e *dedupeEnv) ghostConnectionID(t *testing.T, name string) ids.UUID {
	t.Helper()
	ctx := e.as()
	var id ids.UUID
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT id FROM linkedin_connection WHERE full_name = $1 ORDER BY connected_on NULLS FIRST LIMIT 1`,
			name).Scan(&id)
	}); err != nil {
		t.Fatalf("reading connection id for %q: %v", name, err)
	}
	return id
}

// auditCount counts the update audit rows on one entity.
func (e *dedupeEnv) auditCount(t *testing.T, entityType string, entityID ids.UUID) int {
	t.Helper()
	ctx := e.as()
	var n int
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT count(*) FROM audit_log WHERE entity_type = $1 AND entity_id = $2 AND action = 'update'`,
			entityType, entityID).Scan(&n)
	}); err != nil {
		t.Fatalf("counting %s audit rows: %v", entityType, err)
	}
	return n
}

// eventCount counts the outbox events of one type.
func (e *dedupeEnv) eventCount(t *testing.T, eventType string) int {
	t.Helper()
	ctx := e.as()
	var n int
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT count(*) FROM event_outbox WHERE envelope->>'type' = $1`, eventType).Scan(&n)
	}); err != nil {
		t.Fatalf("counting %s events: %v", eventType, err)
	}
	return n
}
