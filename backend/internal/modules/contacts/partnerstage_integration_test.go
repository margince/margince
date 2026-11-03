// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// The partner stage list against Postgres. The tests cover what the catalog
// refuses, a partner on a reshaped ladder, and the activation rule. They also
// cover the meaning filters and a partner holding several kinds.

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// asStageAdmin is a seat that keeps the data model and runs partner programmes.
func asStageAdmin(e *dedupeEnv, objects map[string]principal.ObjectGrant) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.rep.String(), UserID: e.rep,
		Permissions: principal.Permissions{RoleKeys: []string{"admin"}, Objects: objects, RowScope: principal.RowScopeAll},
	})
}

func stageAdminGrants() map[string]principal.ObjectGrant {
	return map[string]principal.ObjectGrant{
		"company":      {Create: true, Read: true, Update: true, Delete: true},
		"partner":      {Create: true, Read: true, Update: true},
		"custom_field": {Create: true, Read: true, Update: true, Delete: true},
		"lead":         {Create: true, Read: true, Update: true},
	}
}

// keepPartnerStagesAsSeeded puts the stage list back when the test ends. A
// reset keeps the list, as it keeps every administered vocabulary, so an edit
// here would otherwise change the next test's ladder.
func keepPartnerStagesAsSeeded(t *testing.T, e *dedupeEnv) {
	t.Helper()
	t.Cleanup(func() {
		ctx := asStageAdmin(e, stageAdminGrants())
		if err := e.store.tx(ctx, func(tx pgx.Tx) error {
			for _, statement := range []string{
				`UPDATE partner SET relationship_stage = 'research'
				  WHERE relationship_stage IN (SELECT key FROM partner_stage WHERE NOT system)`,
				`DELETE FROM partner_stage WHERE NOT system`,
				`UPDATE partner_stage SET active = true, label = shipped_label`,
			} {
				if _, err := tx.Exec(ctx, statement); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			t.Errorf("restoring the partner stage list: %v", err)
		}
	})
}

func stageByKey(ctx context.Context, t *testing.T, e *dedupeEnv, key string) crmcontracts.PartnerStage {
	t.Helper()
	list, err := e.store.ListPartnerStages(ctx)
	if err != nil {
		t.Fatalf("listing partner stages: %v", err)
	}
	for _, stage := range list.Data {
		if stage.Key == key {
			return stage
		}
	}
	t.Fatalf("no stage %q in the list", key)
	return crmcontracts.PartnerStage{}
}

func newPartnerCompany(ctx context.Context, t *testing.T, e *dedupeEnv, name string, in UpsertPartnerInput) (ids.CompanyID, crmcontracts.Partner) {
	t.Helper()
	company, err := e.store.CreateCompany(ctx, CreateCompanyInput{DisplayName: name, Source: "manual"})
	if err != nil {
		t.Fatalf("creating %s: %v", name, err)
	}
	in.CompanyID = ids.From[ids.CompanyKind](ids.UUID(company.Id))
	partner, err := e.store.UpsertPartner(ctx, in)
	if err != nil {
		t.Fatalf("making %s a partner: %v", name, err)
	}
	return in.CompanyID, partner
}

func fieldCode(err error) string {
	var fault interface {
		FieldFault() (string, string, string)
	}
	if errors.As(err, &fault) {
		_, code, _ := fault.FieldFault()
		return code
	}
	if parse, ok := errors.AsType[*values.ParseError](err); ok {
		return parse.Code
	}
	if detailed, ok := errors.AsType[*httperr.DetailedError](err); ok && len(detailed.Fields) > 0 {
		return detailed.Fields[0].Code
	}
	return ""
}

func TestThePartnerStageListRefusesWhatTheContractPromises(t *testing.T) {
	e := setupDedupe(t)
	keepPartnerStagesAsSeeded(t, e)
	ctx := asStageAdmin(e, stageAdminGrants())

	created, err := e.store.CreatePartnerStage(ctx, CreatePartnerStageInput{Label: "Scouting", Meaning: crmcontracts.PartnerStageMeaningCandidate})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Key != "scouting" || *created.System || *created.PartnerCount != 0 {
		t.Fatalf("created %+v, want key scouting, not built-in, no partners", created)
	}
	if _, err := e.store.CreatePartnerStage(ctx, CreatePartnerStageInput{Label: "Scouting", Meaning: "candidate"}); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("a duplicate key: err = %v, want ErrConflict", err)
	}
	for name, in := range map[string]CreatePartnerStageInput{
		"a blank label":      {Label: "  ", Meaning: "candidate"},
		"a bad key":          {Key: "9lives", Label: "Nine", Meaning: "candidate"},
		"an unknown meaning": {Label: "Won", Meaning: "won"},
	} {
		if _, err := e.store.CreatePartnerStage(ctx, in); err == nil || errors.Is(err, apperrors.ErrConflict) {
			t.Errorf("%s: err = %v, want a validation refusal", name, err)
		}
	}
	if err := e.store.DeletePartnerStage(ctx, ids.UUID(stageByKey(ctx, t, e, "dormant").Id)); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("deleting a built-in stage: err = %v, want ErrConflict", err)
	}

	companyID, _ := newPartnerCompany(ctx, t, e, "Scouted GmbH", UpsertPartnerInput{PartnerRole: "hosting", RelationshipStage: &created.Key})
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return e.store.ArchiveCompanyTx(ctx, tx, companyID, nil)
	}); err != nil {
		t.Fatalf("archiving the partner company: %v", err)
	}
	if err := e.store.DeletePartnerStage(ctx, ids.UUID(created.Id)); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("deleting a stage an archived partner carries: err = %v, want ErrConflict", err)
	}
	signed := crmcontracts.PartnerStageMeaningSigned
	if _, err := e.store.UpdatePartnerStage(ctx, ids.UUID(created.Id), UpdatePartnerStageInput{Meaning: &signed}); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("changing the meaning of a stage in use: err = %v, want ErrConflict", err)
	}
	renamed := "Scouted"
	if got, err := e.store.UpdatePartnerStage(ctx, ids.UUID(created.Id), UpdatePartnerStageInput{Label: &renamed}); err != nil || got.Label != renamed {
		t.Errorf("renaming a stage in use: %+v, %v; want the new label", got, err)
	}
	var audits int
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE entity_type = 'partner_stage' AND entity_id = $1`,
			ids.UUID(created.Id)).Scan(&audits)
	}); err != nil {
		t.Fatal(err)
	}
	if audits != 2 {
		t.Errorf("%d audit rows for the stage's create and rename, want 2", audits)
	}
}

func TestTheLastCandidateStageCannotBeTakenAway(t *testing.T) {
	e := setupDedupe(t)
	keepPartnerStagesAsSeeded(t, e)
	ctx := asStageAdmin(e, stageAdminGrants())
	retired := false
	for _, key := range []string{"identified", "contacted", "in_conversation", "fit_confirmed", "agreement_pending"} {
		if _, err := e.store.UpdatePartnerStage(ctx, ids.UUID(stageByKey(ctx, t, e, key).Id), UpdatePartnerStageInput{Active: &retired}); err != nil {
			t.Fatalf("retiring %s: %v", key, err)
		}
	}
	research := stageByKey(ctx, t, e, "research")
	if _, err := e.store.UpdatePartnerStage(ctx, ids.UUID(research.Id), UpdatePartnerStageInput{Active: &retired}); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("retiring the last candidate stage: err = %v, want ErrConflict", err)
	}
	ended := crmcontracts.PartnerStageMeaningEnded
	if _, err := e.store.UpdatePartnerStage(ctx, ids.UUID(research.Id), UpdatePartnerStageInput{Meaning: &ended}); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("re-meaning the last candidate stage: err = %v, want ErrConflict", err)
	}
}

func TestAPartnerMovesOnALadderTheAdministratorReshaped(t *testing.T) {
	e := setupDedupe(t)
	keepPartnerStagesAsSeeded(t, e)
	ctx := asStageAdmin(e, stageAdminGrants())
	if _, err := e.store.CreatePartnerStage(ctx, CreatePartnerStageInput{Label: "Long list", Meaning: "candidate", SortOrder: 5}); err != nil {
		t.Fatalf("adding a first stage: %v", err)
	}
	retired := false
	research := stageByKey(ctx, t, e, "research")
	if _, err := e.store.UpdatePartnerStage(ctx, ids.UUID(research.Id), UpdatePartnerStageInput{Active: &retired}); err != nil {
		t.Fatalf("retiring research: %v", err)
	}

	companyID, partner := newPartnerCompany(ctx, t, e, "Ladder AG", UpsertPartnerInput{PartnerKinds: &[]string{"consulting"}})
	if *partner.RelationshipStage != "long_list" || *partner.StageMeaning != crmcontracts.PartnerStageMeaningCandidate {
		t.Fatalf("a new programme starts on %s (%v), want the first active candidate stage long_list",
			*partner.RelationshipStage, partner.StageMeaning)
	}
	_, err := e.store.UpsertPartner(ctx, UpsertPartnerInput{CompanyID: companyID, PartnerRole: "consulting", RelationshipStage: new("research")})
	if code := fieldCode(err); code != "inactive_partner_stage" {
		t.Errorf("a move to a retired stage: err = %v, want inactive_partner_stage", err)
	}
	_, err = e.store.UpsertPartner(ctx, UpsertPartnerInput{CompanyID: companyID, PartnerRole: "consulting", RelationshipStage: new("won")})
	if code := fieldCode(err); code != "unknown_partner_stage" {
		t.Errorf("a move to an unknown stage: err = %v, want unknown_partner_stage", err)
	}
	moved, err := e.store.UpsertPartner(ctx, UpsertPartnerInput{CompanyID: companyID, PartnerRole: "consulting", RelationshipStage: new("contract_signed")})
	if err != nil || *moved.StageMeaning != crmcontracts.PartnerStageMeaningSigned {
		t.Fatalf("a move to contract_signed: %+v, %v", moved.StageMeaning, err)
	}

	retiredCompany, _ := newPartnerCompany(ctx, t, e, "Kept Stage KG", UpsertPartnerInput{PartnerRole: "hosting", RelationshipStage: new("ready")})
	ready := stageByKey(ctx, t, e, "ready")
	if _, err := e.store.UpdatePartnerStage(ctx, ids.UUID(ready.Id), UpdatePartnerStageInput{Active: &retired}); err != nil {
		t.Fatalf("retiring ready: %v", err)
	}
	step := "send the deck"
	kept, err := e.store.UpsertPartner(ctx, UpsertPartnerInput{CompanyID: retiredCompany, PartnerRole: "hosting", NextStep: &step})
	if err != nil || *kept.RelationshipStage != "ready" {
		t.Errorf("an edit to a partner on a retired stage: stage %v, err %v; want it kept on ready", kept.RelationshipStage, err)
	}
}

func TestAPartnerStandsOnAnActiveStageOnlyOnceActivated(t *testing.T) {
	e := setupDedupe(t)
	keepPartnerStagesAsSeeded(t, e)
	ctx := asStageAdmin(e, stageAdminGrants())
	companyID, _ := newPartnerCompany(ctx, t, e, "Not Yet GmbH", UpsertPartnerInput{PartnerRole: "hosting", RelationshipStage: new("ready")})

	for _, key := range []string{"activated", "active", "active_referring"} {
		_, err := e.store.UpsertPartner(ctx, UpsertPartnerInput{CompanyID: companyID, PartnerRole: "hosting", RelationshipStage: &key})
		if code := fieldCode(err); code != "partner_not_activated" {
			t.Errorf("a move to %s before activation: err = %v, want partner_not_activated", key, err)
		}
	}
	if _, err := e.store.CreatePartnerStage(ctx, CreatePartnerStageInput{Label: "Live", Meaning: "active"}); err != nil {
		t.Fatalf("adding a custom active stage: %v", err)
	}
	_, err := e.store.UpsertPartner(ctx, UpsertPartnerInput{CompanyID: companyID, PartnerRole: "hosting", RelationshipStage: new("live")})
	if code := fieldCode(err); code != "partner_not_activated" {
		t.Errorf("a move to a custom active stage before activation: err = %v, want partner_not_activated", err)
	}

	acceptReferredLead(ctx, t, e, companyID, "Referred Lead")
	moved, err := e.store.UpsertPartner(ctx, UpsertPartnerInput{CompanyID: companyID, PartnerRole: "hosting", RelationshipStage: new("live")})
	if err != nil || *moved.StageMeaning != crmcontracts.PartnerStageMeaningActive || moved.ActivatedAt == nil {
		t.Errorf("an activated partner moving to live: %+v, %v; want the active stage and the stamp", moved.StageMeaning, err)
	}
}

func TestMeaningFiltersHoldAfterEveryStageIsRenamed(t *testing.T) {
	e := setupDedupe(t)
	keepPartnerStagesAsSeeded(t, e)
	ctx := asStageAdmin(e, stageAdminGrants())
	signedID, _ := newPartnerCompany(ctx, t, e, "Signed AG", UpsertPartnerInput{PartnerRole: "hosting", RelationshipStage: new("contract_signed")})
	candidateID, _ := newPartnerCompany(ctx, t, e, "Candidate AG", UpsertPartnerInput{PartnerRole: "strategic"})
	list, err := e.store.ListPartnerStages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, stage := range list.Data {
		label := "Stufe " + stage.Key
		if _, err := e.store.UpdatePartnerStage(ctx, ids.UUID(stage.Id), UpdatePartnerStageInput{Label: &label}); err != nil {
			t.Fatalf("renaming %s: %v", stage.Key, err)
		}
	}

	partners, _, err := e.store.ListPartners(ctx, ListPartnersInput{StageMeaning: new("signed")})
	if err != nil {
		t.Fatal(err)
	}
	if len(partners) != 1 || ids.UUID(partners[0].CompanyId) != signedID.UUID || *partners[0].StageLabel != "Stufe contract_signed" {
		t.Errorf("signed partners after the rename: %+v, want only Signed AG under its new label", partners)
	}
	companies, _, err := e.store.ListCompanies(ctx, ListCompaniesInput{Partner: partnerProgramFilter{Meaning: new("candidate")}})
	if err != nil {
		t.Fatal(err)
	}
	if len(companies) != 1 || ids.UUID(companies[0].Id) != candidateID.UUID || companies[0].Partner == nil {
		t.Errorf("candidate companies after the rename: %d rows, want only Candidate AG with its programme", len(companies))
	}
}

func TestAPartnerHoldsSeveralKinds(t *testing.T) {
	e := setupDedupe(t)
	ctx := asStageAdmin(e, stageAdminGrants())
	companyID, partner := newPartnerCompany(ctx, t, e, "Many Kinds GmbH",
		UpsertPartnerInput{PartnerKinds: &[]string{"referral", "hosting", "referral"}})
	if !slices.Equal(*partner.PartnerKinds, []crmcontracts.PartnerKind{"hosting", "referral"}) ||
		partner.PartnerRole == nil || *partner.PartnerRole != "hosting" {
		t.Errorf("kinds %v, role %v; want [hosting referral] read as role hosting", partner.PartnerKinds, partner.PartnerRole)
	}
	referralOnly, err := e.store.UpsertPartner(ctx, UpsertPartnerInput{CompanyID: companyID, PartnerKinds: &[]string{"referral"}})
	if err != nil || referralOnly.PartnerRole != nil {
		t.Errorf("a referral-only partner: role %v, err %v; want no legacy role", referralOnly.PartnerRole, err)
	}
	for name, in := range map[string]UpsertPartnerInput{
		"implementation": {CompanyID: companyID, PartnerKinds: &[]string{"implementation"}},
		"no kind":        {CompanyID: companyID, PartnerKinds: &[]string{}},
		"nothing sent":   {CompanyID: companyID},
	} {
		if _, err := e.store.UpsertPartner(ctx, in); err == nil {
			t.Errorf("%s: the write was accepted", name)
		}
	}
	found, _, err := e.store.ListPartners(ctx, ListPartnersInput{PartnerKind: new("referral")})
	if err != nil || len(found) != 1 {
		t.Errorf("filtering by the referral kind: %d rows, %v; want the one partner", len(found), err)
	}
}

func TestASeatThatCannotReadPartnersSeesCompaniesWithoutTheirProgramme(t *testing.T) {
	e := setupDedupe(t)
	admin := asStageAdmin(e, stageAdminGrants())
	companyID, _ := newPartnerCompany(admin, t, e, "Hidden Terms AG", UpsertPartnerInput{PartnerRole: "consulting"})

	reader := asStageAdmin(e, map[string]principal.ObjectGrant{"company": {Read: true}})
	company, err := e.store.GetCompany(reader, companyID, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("reading the company: %v", err)
	}
	if company.Partner != nil {
		t.Error("a seat without the partner grant was shown the partner programme")
	}
	if _, _, err := e.store.ListCompanies(reader, ListCompaniesInput{Partner: partnerProgramFilter{Kind: new("consulting")}}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a partner filter without the partner grant: err = %v, want permission denied", err)
	}
	if _, err := e.store.CreatePartnerStage(asStageAdmin(e, map[string]principal.ObjectGrant{"partner": {Read: true}}),
		CreatePartnerStageInput{Label: "Mine", Meaning: "candidate"}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a stage write without the data-model grant: err = %v, want permission denied", err)
	}
	withPartner, err := e.store.GetCompany(admin, companyID, storekit.LiveOnly)
	if err != nil || withPartner.Partner == nil || *withPartner.Partner.StageLabel != "Research" {
		t.Errorf("a partner reader's company read: partner %+v, err %v; want the programme on research", withPartner.Partner, err)
	}
}
