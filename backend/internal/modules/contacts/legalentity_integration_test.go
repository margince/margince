// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// The legal entities against Postgres. The suite covers the migration's default,
// the refusals the contract names and which entity a user resolves to. It also
// covers the write shape every entity write commits.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// entityAdmin is the seeded admin's posture on the two objects an entity
// write asks, plus the company grant the company form needs.
var entityAdmin = map[string]principal.ObjectGrant{
	legalEntityObject: {Create: true, Read: true, Update: true, Delete: true},
	userAdminObject:   {Create: true, Read: true, Update: true, Delete: true},
	companyEntity:     {Create: true, Read: true, Update: true, Delete: true},
}

// entityRep is the seeded rep's posture: it reads entities and administers no seat.
var entityRep = map[string]principal.ObjectGrant{legalEntityObject: {Read: true}}

func actingAs(e *promoteConsentEnv, role string, grants map[string]principal.ObjectGrant) context.Context {
	ctx := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.ws), ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.user.String(), UserID: e.user,
		Permissions: principal.Permissions{RoleKeys: []string{role}, Objects: grants, RowScope: principal.RowScopeAll},
	})
}

func defaultEntity(t *testing.T, e *promoteConsentEnv) ids.LegalEntityID {
	t.Helper()
	var id ids.LegalEntityID
	if err := e.owner.QueryRow(context.Background(), `SELECT id FROM legal_entity WHERE is_default`).Scan(&id); err != nil {
		t.Fatalf("read the default entity: %v", err)
	}
	return id
}

// The test reset deletes the entities a test described. It returns the default
// to its seeded state, so no test here depends on the order the suite ran in.
func TestTheResetLeavesOnlyTheSeededDefault(t *testing.T) {
	e := setupPromoteConsent(t)
	describeEntity(t, e)
	if _, err := e.owner.Exec(context.Background(), `UPDATE legal_entity SET country = 'de' WHERE is_default`); err != nil {
		t.Fatalf("set the default's country: %v", err)
	}
	e = setupPromoteConsent(t)
	var rows int
	var country *string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT (SELECT count(*) FROM legal_entity), (SELECT country FROM legal_entity WHERE is_default)`).
		Scan(&rows, &country); err != nil {
		t.Fatalf("read the entities after a reset: %v", err)
	}
	if rows != 1 || country != nil {
		t.Errorf("after a reset: %d entities and default country %v, want the seeded default alone", rows, country)
	}
}

func TestNoEntityTakesTheCompanysName(t *testing.T) {
	e := setupPromoteConsent(t)
	admin := actingAs(e, "admin", entityAdmin)
	if _, err := e.store.WithInstallationRenamer(ignoreInstallationName).SaveCompany(admin,
		SaveCompanyInput{DisplayName: "Acme"}); err != nil {
		t.Fatalf("save the company: %v", err)
	}
	_, err := e.store.CreateLegalEntity(admin, LegalEntityInput{
		DisplayName: new("ACME"), LegalName: new("Acme GmbH"), Country: new("de"),
	})
	if refusal, ok := errors.AsType[*LegalEntityRefusal](err); !ok || refusal.code != codeNameTaken {
		t.Errorf("an entity named after the company answered %v, want %s", err, codeNameTaken)
	}
	vienna := describeEntity(t, e)
	_, err = e.store.UpdateLegalEntity(admin, vienna.ID, LegalEntityInput{DisplayName: new("acme")}, nil)
	if refusal, ok := errors.AsType[*LegalEntityRefusal](err); !ok || refusal.code != codeNameTaken {
		t.Errorf("renaming an entity to the company's name answered %v, want %s", err, codeNameTaken)
	}
}

func TestArchiveRefusesAStaleVersion(t *testing.T) {
	e := setupPromoteConsent(t)
	admin := actingAs(e, "admin", entityAdmin)
	vienna := describeEntity(t, e)
	stale := vienna.Version - 1
	if _, err := e.store.ArchiveLegalEntity(admin, vienna.ID, &stale); !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Errorf("archiving at a stale version answered %v, want version skew", err)
	}
	if _, err := e.store.ArchiveLegalEntity(admin, vienna.ID, &vienna.Version); err != nil {
		t.Errorf("archiving at the current version: %v", err)
	}
}

func describeEntity(t *testing.T, e *promoteConsentEnv) LegalEntity {
	const name = "Wien"
	t.Helper()
	entity, err := e.store.CreateLegalEntity(actingAs(e, "admin", entityAdmin), LegalEntityInput{
		DisplayName: new(name), LegalName: new(name + " GmbH"), Country: new("AT"),
	})
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	return entity
}

func TestTheMigrationLeavesOneDefaultEntityThatCannotBeRetired(t *testing.T) {
	e := setupPromoteConsent(t)
	ctx := context.Background()
	var count int
	if err := e.owner.QueryRow(ctx, `SELECT count(*) FROM legal_entity WHERE is_default`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("default entities = %d (%v), want exactly one", count, err)
	}
	id := defaultEntity(t, e)

	_, err := e.store.ArchiveLegalEntity(actingAs(e, "admin", entityAdmin), id, nil)
	if refusal, ok := errors.AsType[*LegalEntityRefusal](err); !ok || refusal.code != "legal_entity_default_is_permanent" {
		t.Errorf("archiving the default answered %v, want the legal_entity_default_is_permanent refusal", err)
	}
	for name, statement := range map[string]string{
		"archive":     `UPDATE legal_entity SET archived_at = now() WHERE is_default`,
		"demote":      `UPDATE legal_entity SET is_default = false WHERE is_default`,
		"delete":      `DELETE FROM legal_entity WHERE is_default`,
		"second":      `INSERT INTO legal_entity (is_default, captured_by) VALUES (true, 'human:x')`,
		"anchor name": `UPDATE legal_entity SET display_name = 'Own name' WHERE is_default`,
	} {
		if _, err := e.owner.Exec(ctx, statement); err == nil {
			t.Errorf("%s of the default entity was accepted by the schema", name)
		}
	}
}

func TestAnEntityNeedsItsRequiredFieldsAndCleanValues(t *testing.T) {
	e := setupPromoteConsent(t)
	admin := actingAs(e, "admin", entityAdmin)
	for name, tc := range map[string]struct {
		in   LegalEntityInput
		code string
	}{
		"no legal name": {LegalEntityInput{DisplayName: new("Wien"), Country: new("at")}, codeRequired},
		"no country":    {LegalEntityInput{DisplayName: new("Wien"), LegalName: new("Wien GmbH")}, codeRequired},
		"unknown country": {LegalEntityInput{
			DisplayName: new("Wien"), LegalName: new("Wien GmbH"),
			Country: new("uk"),
		}, "invalid_country"},
		"bad check digits": {LegalEntityInput{
			DisplayName: new("Wien"), LegalName: new("Wien GmbH"),
			Country: new("at"), IBAN: new("DE89 3704 0044 0532 0130 01"),
		}, "invalid_iban"},
	} {
		_, err := e.store.CreateLegalEntity(admin, tc.in)
		if parse, ok := errors.AsType[*values.ParseError](err); !ok || parse.Code != tc.code {
			t.Errorf("%s: err = %v, want a %s refusal", name, err, tc.code)
		}
	}
	created, err := e.store.CreateLegalEntity(admin, LegalEntityInput{
		DisplayName: new("Wien"),
		LegalName:   new("Wien GmbH"), Country: new("AT"), IBAN: new("de89 3704 0044 0532 0130 00"),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if *created.Country != "at" || *created.IBAN != "DE89370400440532013000" {
		t.Errorf("stored country %q and IBAN %q, want at and the IBAN without spaces", *created.Country, *created.IBAN)
	}
	if _, err := e.store.CreateLegalEntity(admin, LegalEntityInput{
		DisplayName: new("wien"),
		LegalName:   new("Other GmbH"), Country: new("at"),
	}); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("a second live entity named wien answered %v, want a conflict", err)
	}
	cleared := ""
	if _, err := e.store.UpdateLegalEntity(admin, created.ID, LegalEntityInput{LegalName: &cleared}, nil); err == nil {
		t.Error("clearing the legal name of a further entity was accepted")
	}
}

func TestAUserWorksForTheDefaultEntityUntilAssigned(t *testing.T) {
	e := setupPromoteConsent(t)
	admin := actingAs(e, "admin", entityAdmin)
	user := ids.From[ids.UserKind](e.user)
	vienna := describeEntity(t, e)
	resolve := func() ids.LegalEntityID {
		var got ids.LegalEntityID
		if err := e.store.tx(admin, func(tx pgx.Tx) (err error) {
			got, err = UserLegalEntity(admin, tx, user)
			return err
		}); err != nil {
			t.Fatalf("resolve: %v", err)
		}
		return got
	}
	if got := resolve(); got != defaultEntity(t, e) {
		t.Errorf("an unassigned user resolves to %s, want the default entity", got)
	}
	if err := e.store.AssignLegalEntityMember(admin, vienna.ID, user); err != nil {
		t.Fatalf("assign: %v", err)
	}
	if got := resolve(); got != vienna.ID {
		t.Errorf("an assigned user resolves to %s, want their entity %s", got, vienna.ID)
	}
	if _, err := e.store.ArchiveLegalEntity(admin, vienna.ID, nil); err == nil {
		t.Error("an entity with a member was archived")
	} else if refusal, ok := errors.AsType[*LegalEntityRefusal](err); !ok || refusal.code != "legal_entity_has_members" {
		t.Errorf("archiving an entity with a member answered %v, want legal_entity_has_members", err)
	}
	if err := e.store.AssignLegalEntityMember(admin, defaultEntity(t, e), user); err != nil {
		t.Fatalf("move back to the default: %v", err)
	}
	var rows int
	if err := e.owner.QueryRow(context.Background(), `SELECT count(*) FROM legal_entity_member`).Scan(&rows); err != nil || rows != 0 {
		t.Errorf("assignment rows after moving to the default = %d (%v), want none", rows, err)
	}
	if _, err := e.store.ArchiveLegalEntity(admin, vienna.ID, nil); err != nil {
		t.Errorf("archiving the emptied entity: %v", err)
	}
	if err := e.store.AssignLegalEntityMember(admin, vienna.ID, user); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("assigning to an archived entity answered %v, want not found", err)
	}
}

func TestARepReadsTheEntitiesAndChangesNone(t *testing.T) {
	e := setupPromoteConsent(t)
	h := Handlers{store: e.store}
	rep := actingAs(e, "rep", entityRep)

	get := httptest.NewRecorder()
	h.ListLegalEntities(get, httptest.NewRequestWithContext(rep, http.MethodGet, "/v1/legal-entities", nil), crmcontracts.ListLegalEntitiesParams{})
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), `"is_default":true`) {
		t.Errorf("a rep listing entities got %d %s, want 200 with the default entity", get.Code, get.Body)
	}
	post := httptest.NewRecorder()
	h.CreateLegalEntity(post, httptest.NewRequestWithContext(rep, http.MethodPost, "/v1/legal-entities",
		strings.NewReader(`{"display_name":"Wien","legal_name":"Wien GmbH","country":"at"}`)))
	if post.Code != http.StatusForbidden {
		t.Errorf("a rep creating an entity got %d, want 403", post.Code)
	}

	// Holding the entity grant is not enough to move a user: that is seat
	// administration, and it asks user_admin as well.
	entityOnly := actingAs(e, "manager", map[string]principal.ObjectGrant{legalEntityObject: {Read: true, Update: true}})
	vienna := describeEntity(t, e)
	err := e.store.AssignLegalEntityMember(entityOnly, vienna.ID, ids.From[ids.UserKind](e.user))
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("an assignment without user_admin answered %v, want permission denied", err)
	}
}

func TestEveryEntityWriteCommitsItsAuditAndItsEvent(t *testing.T) {
	e := setupPromoteConsent(t)
	admin := actingAs(e, "admin", entityAdmin)
	vienna := describeEntity(t, e)
	if _, err := e.store.UpdateLegalEntity(admin, vienna.ID, LegalEntityInput{
		IBAN: new("DE89370400440532013000"), Imprint: new("Geschäftsführer: A. Example"),
	}, &vienna.Version); err != nil {
		t.Fatalf("update: %v", err)
	}
	if _, err := e.store.UpdateLegalEntity(admin, vienna.ID, LegalEntityInput{Imprint: new("x")}, &vienna.Version); !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Errorf("an update at a stale version answered %v, want version skew", err)
	}
	if err := e.store.AssignLegalEntityMember(admin, vienna.ID, ids.From[ids.UserKind](e.user)); err != nil {
		t.Fatalf("assign: %v", err)
	}
	if err := e.store.AssignLegalEntityMember(admin, defaultEntity(t, e), ids.From[ids.UserKind](e.user)); err != nil {
		t.Fatalf("unassign: %v", err)
	}
	if _, err := e.store.ArchiveLegalEntity(admin, vienna.ID, nil); err != nil {
		t.Fatalf("archive: %v", err)
	}
	for action, eventType := range map[string]string{
		"create": "legal_entity.created", "update": "legal_entity.updated", "archive": "legal_entity.archived",
	} {
		assertAuditAndEvent(t, e, vienna.ID.UUID, action, eventType, 1)
	}
	assertAuditAndEvent(t, e, vienna.ID.UUID, "assign", "legal_entity.member_added", 1)
	assertAuditAndEvent(t, e, defaultEntity(t, e).UUID, "assign", "legal_entity.member_added", 1)

	var payload string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT envelope->>'payload' FROM event_outbox WHERE envelope->>'type' = 'legal_entity.updated'`).Scan(&payload); err != nil {
		t.Fatalf("read the update event: %v", err)
	}
	if strings.Contains(payload, "DE89") || strings.Contains(payload, "Example") ||
		!strings.Contains(payload, `"iban"`) || !strings.Contains(payload, `"imprint"`) {
		t.Errorf("legal_entity.updated carries %s, want the changed field names and none of their values", payload)
	}
}

func assertAuditAndEvent(t *testing.T, e *promoteConsentEnv, entity ids.UUID, action, eventType string, want int) {
	t.Helper()
	var audits, events int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT (SELECT count(*) FROM audit_log WHERE entity_type = 'legal_entity' AND entity_id = $1 AND action = $2),
		        (SELECT count(*) FROM event_outbox WHERE envelope->>'type' = $3 AND envelope->'entity'->>'id' = $1::text)`,
		entity, action, eventType).Scan(&audits, &events); err != nil {
		t.Fatalf("count %s rows: %v", action, err)
	}
	if audits != want || events != want {
		t.Errorf("%s left %d audit rows and %d %s events, want %d of each", action, audits, events, eventType, want)
	}
}

func TestTheDefaultEntityReadsTheAnchorCompany(t *testing.T) {
	e := setupPromoteConsent(t)
	admin := actingAs(e, "admin", entityAdmin)
	if _, err := e.store.WithInstallationRenamer(ignoreInstallationName).SaveCompany(admin, SaveCompanyInput{DisplayName: "Acme", Fields: map[string]*string{
		fieldRegisteredAddress: new("Torstraße 1, 10119 Berlin"), fieldRegisterVat: new("DE123456789"),
	}}); err != nil {
		t.Fatalf("save the company: %v", err)
	}
	var identity EntityIdentity
	if err := e.store.tx(admin, func(tx pgx.Tx) (err error) {
		identity, err = ResolveEntityIdentity(admin, tx, defaultEntity(t, e))
		return err
	}); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if identity.DisplayName != "Acme" || identity.VatID == nil || *identity.VatID != "DE123456789" ||
		pointerValue(identity.Address.Line1) != "Torstraße 1" || pointerValue(identity.Address.PostalCode) != "10119" ||
		pointerValue(identity.Address.City) != "Berlin" || identity.AnchorCompanyID.IsZero() {
		t.Errorf("the default entity resolves to %+v, want the anchor's name, address and VAT ID", identity)
	}
	_, err := e.store.UpdateLegalEntity(admin, identity.ID, LegalEntityInput{DisplayName: new("Other")}, nil)
	if parse, ok := errors.AsType[*values.ParseError](err); !ok || parse.Code != codeFieldOnCompanyForm {
		t.Errorf("renaming the default entity answered %v, want %s", err, codeFieldOnCompanyForm)
	}
	updated, err := e.store.UpdateLegalEntity(admin, identity.ID, LegalEntityInput{Country: new("de")}, nil)
	if err != nil || updated.Country == nil || *updated.Country != "de" || updated.DisplayName != "Acme" {
		t.Errorf("setting the default entity's own country gave %+v (%v), want de beside the anchor's name", updated, err)
	}
}
