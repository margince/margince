// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A unit landing companies and deals through Runtime.Ingest.
//
// Every case runs on the member's live authority over a real database. The
// stores below are the product's own, so what these tests read back is what the
// HTTP surface would show for the same row.

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension"
)

const (
	crmSystem = "crm-x"
	crmSource = "ext:" + ingressUnit + ":" + crmSystem
	// crmPolicy is the narrowest grant that lets a unit land all three kinds.
	crmPolicy = `{"objects":{
		"activity":{"read":true,"create":true,"update":true},
		"contact":{"read":true,"create":true,"update":true},
		"company":{"read":true,"create":true,"update":true},
		"deal":{"read":true,"create":true,"update":true}},
		"row_scope":"all"}`
)

type crmEnv struct {
	*ingressEnv
	pipeline        ids.PipelineID
	open, won, lost ids.StageID
}

func setupCRMIngress(t *testing.T) *crmEnv {
	t.Helper()
	e := setupExtRuntime(t)
	composeCapturingUnit(t, ingressUnit, nil, extension.IngressSource{
		System: crmSystem,
		Lands:  []extension.RecordKind{extension.KindActivity, extension.KindCompany, extension.KindDeal, extension.KindSignal},
	})
	bindCaptureForTest(t, e)
	grantPolicy(t, e.Rep1, "crm-probe", crmPolicy)
	depositCredential(t, e, e.Rep1)
	pipeline, open, won := integration.DealFixture(t, e.Env)
	p, err := e.Deals.DefaultPipeline(e.Admin())
	if err != nil {
		t.Fatalf("reading the default pipeline: %v", err)
	}
	var lost ids.StageID
	for _, st := range *p.Stages {
		if st.Semantic == "lost" {
			lost = ids.From[ids.StageKind](ids.UUID(st.Id))
		}
	}
	if lost.IsZero() {
		t.Fatal("the default pipeline has no lost stage")
	}
	return &crmEnv{ingressEnv: &ingressEnv{extRuntimeEnv: e, member: e.Rep1}, pipeline: pipeline, open: open, won: won, lost: lost}
}

func (e *crmEnv) ingest(t *testing.T, member ids.UUID, rec extension.Record) (extension.Result, error) {
	t.Helper()
	return e.ingestingRuntime().Ingest(context.Background(), extension.UserID(member.String()), rec)
}

func (e *crmEnv) mustIngest(t *testing.T, rec extension.Record) extension.Result {
	t.Helper()
	res, err := e.ingest(t, e.member, rec)
	if err != nil || res.Disposition != extension.DispositionAccepted {
		t.Fatalf("ingest = %+v, %v, want accepted", res, err)
	}
	return res
}

func companyRecord(key string, fields extension.CompanyFields) extension.Record {
	return extension.Record{System: crmSystem, Key: key, Company: &fields}
}

func (e *crmEnv) dealRecord(key string, edit func(*extension.DealFields)) extension.Record {
	fields := extension.DealFields{Name: "Renewal", PipelineID: e.pipeline.String(), StageID: e.open.String()}
	if edit != nil {
		edit(&fields)
	}
	return extension.Record{System: crmSystem, Key: key, Deal: &fields}
}

func (e *crmEnv) companyColumns(t *testing.T, id string) (industry, description, system, key *string) {
	t.Helper()
	e.readAsWorkspace(t, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT industry, description, source_system, source_id FROM company WHERE id = $1`, ids.MustParse(id)).
			Scan(&industry, &description, &system, &key)
	})
	return
}

func TestAnUndeclaredKindIsRefusedAtTheCall(t *testing.T) {
	e := setupIngress(t)
	composeCapturingUnit(t, ingressUnit, nil, extension.IngressSource{
		System: crmSystem, Lands: []extension.RecordKind{extension.KindActivity},
	})
	_, err := e.ingestingRuntime().Ingest(context.Background(), extension.UserID(e.member.String()),
		companyRecord("a-1", extension.CompanyFields{DisplayName: "Acme"}))
	if !errors.Is(err, extension.ErrIngressNotDeclared) {
		t.Fatalf("err = %v, want ErrIngressNotDeclared: the source declared activities only", err)
	}
	if got := e.countAsWorkspace(t, `SELECT count(*) FROM company WHERE source_id = 'a-1'`); got != 0 {
		t.Errorf("companies = %d after a refused kind, want none", got)
	}
}

func TestACompanyIsCreatedThenFilledByItsOutsideIdAndKeepsWhatAUserTyped(t *testing.T) {
	e := setupCRMIngress(t)
	first := e.mustIngest(t, companyRecord("acct-1", extension.CompanyFields{DisplayName: "Acme"}))
	if first.Ref.Type != "company" {
		t.Fatalf("ref type = %q, want company", first.Ref.Type)
	}
	// A user types an industry, then the feed sends a different one and a description.
	if _, err := e.Contacts.UpdateCompany(e.Admin(), ids.From[ids.CompanyKind](ids.MustParse(first.Ref.ID)),
		contacts.UpdateCompanyInput{Industry: new("automotive")}); err != nil {
		t.Fatalf("editing as a user: %v", err)
	}
	second := e.mustIngest(t, companyRecord("acct-1", extension.CompanyFields{
		DisplayName: "Acme Renamed", Industry: "education", Description: "from the feed",
	}))
	if second.Ref.ID != first.Ref.ID {
		t.Fatalf("second ingest made %s, want the same company %s", second.Ref.ID, first.Ref.ID)
	}
	industry, description, system, key := e.companyColumns(t, first.Ref.ID)
	if industry == nil || *industry != "automotive" {
		t.Errorf("industry = %v, want the user's value kept", industry)
	}
	if description == nil || *description != "from the feed" {
		t.Errorf("description = %v, want an empty field filled", description)
	}
	if system == nil || *system != crmSource || key == nil || *key != "acct-1" {
		t.Errorf("outside id = %v/%v, want %s/acct-1", system, key, crmSource)
	}
	if got := e.countAsWorkspace(t, `SELECT count(*) FROM company WHERE source_id = 'acct-1'`); got != 1 {
		t.Errorf("companies = %d, want 1", got)
	}
}

func TestAHandMadeCompanyIsAdoptedByDomainAndKeepsWhatAUserTyped(t *testing.T) {
	e := setupCRMIngress(t)
	made, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{
		DisplayName: "Hand made", Industry: new("manufacturing"),
		Domains: []contacts.CompanyDomainInput{{Domain: "acme.test", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("seeding the hand-made company: %v", err)
	}
	res := e.mustIngest(t, companyRecord("acct-9", extension.CompanyFields{
		DisplayName: "Acme", Industry: "software", Description: "adopted", Domain: "acme.test", AdoptOn: extension.AdoptKeyDomain,
	}))
	if res.Ref.ID != made.Id.String() {
		t.Fatalf("adopt made %s, want the hand-made %s", res.Ref.ID, made.Id)
	}
	industry, description, _, key := e.companyColumns(t, res.Ref.ID)
	if industry == nil || *industry != "manufacturing" || description == nil || *description != "adopted" {
		t.Errorf("industry/description = %v/%v, want the user's industry kept and the empty description filled", industry, description)
	}
	if key == nil || *key != "acct-9" {
		t.Errorf("source_id = %v, want the outside id stamped", key)
	}
}

func TestAClashWithARecordTheMemberCannotSeeIsTheFixedConflict(t *testing.T) {
	e := setupCRMIngress(t)
	held, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{
		DisplayName: "Private", SourceSystem: new(crmSource), SourceID: new("acct-7"),
	})
	if err != nil {
		t.Fatalf("seeding the holder: %v", err)
	}
	e.MakeCapturePrivate(t, "company", ids.UUID(held.Id), e.Rep2)

	_, err = e.ingest(t, e.member, companyRecord("acct-7", extension.CompanyFields{DisplayName: "Acme"}))
	if err != extension.ErrConflict {
		t.Fatalf("err = %v, want exactly extension.ErrConflict, a message that names nothing about the holder", err)
	}

	// The same create, made by someone who can see the holder and by someone
	// who cannot, answers the same words.
	create := contacts.CreateCompanyInput{DisplayName: "Acme", SourceSystem: new(crmSource), SourceID: new("acct-7")}
	_, seen := e.Contacts.CreateCompany(e.As(e.Rep2, nil, integration.AdminPerms), create)
	_, hidden := e.Contacts.CreateCompany(e.As(e.Rep1, nil, integration.AdminPerms), create)
	if seen == nil || hidden == nil || seen.Error() != hidden.Error() {
		t.Fatalf("clash answers differ: seen = %v, hidden = %v", seen, hidden)
	}
}

func TestAClashWithAVisibleRecordUpdatesIt(t *testing.T) {
	e := setupCRMIngress(t)
	held, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{
		DisplayName: "Visible", SourceSystem: new(crmSource), SourceID: new("acct-8"),
	})
	if err != nil {
		t.Fatalf("seeding the holder: %v", err)
	}
	res := e.mustIngest(t, companyRecord("acct-8", extension.CompanyFields{DisplayName: "Acme", Industry: "software"}))
	if res.Ref.ID != held.Id.String() {
		t.Fatalf("ingest made %s, want the visible holder %s", res.Ref.ID, held.Id)
	}
}

func TestADealIsCreatedAndMovedThroughWonAndLostByItsOutsideId(t *testing.T) {
	e := setupCRMIngress(t)
	made := e.mustIngest(t, e.dealRecord("opp-1", nil))
	if made.Ref.Type != "deal" {
		t.Fatalf("ref type = %q, want deal", made.Ref.Type)
	}
	stage := func() (stage ids.UUID, reason *string) {
		e.readAsWorkspace(t, func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT stage_id, lost_reason FROM deal WHERE id = $1`, ids.MustParse(made.Ref.ID)).Scan(&stage, &reason)
		})
		return
	}

	e.mustIngest(t, e.dealRecord("opp-1", func(d *extension.DealFields) { d.StageID = e.won.String() }))
	if got, _ := stage(); got != e.won.UUID {
		t.Fatalf("stage = %s after the win, want %s", got, e.won)
	}

	lostWithoutReason := e.dealRecord("opp-1", func(d *extension.DealFields) { d.StageID = e.lost.String() })
	if _, err := e.ingest(t, e.member, lostWithoutReason); !errors.Is(err, extension.ErrInvalid) {
		t.Fatalf("lost with no reason: err = %v, want ErrInvalid", err)
	}
	e.mustIngest(t, e.dealRecord("opp-1", func(d *extension.DealFields) { d.StageID = e.lost.String(); d.LostReason = "price" }))
	if got, reason := stage(); got != e.lost.UUID || reason == nil || *reason != "price" {
		t.Errorf("stage/reason = %s/%v, want lost for price", got, reason)
	}
	if got := e.countAsWorkspace(t, `SELECT count(*) FROM deal WHERE source_id = 'opp-1'`); got != 1 {
		t.Errorf("deals = %d, want 1", got)
	}
}

func TestEveryCreateAdoptAndMoveIsCheckedAgainstTheMembersRights(t *testing.T) {
	e := setupCRMIngress(t)
	dealed := func(member ids.UUID, role, policy string) {
		grantPolicy(t, member, role, policy)
		depositCredential(t, e.extRuntimeEnv, member)
	}

	// No deal grant at all: the create is refused and nothing lands.
	dealed(e.Rep2, "no-deal", capturePolicy)
	if _, err := e.ingest(t, e.Rep2, e.dealRecord("opp-2", nil)); !errors.Is(err, extension.ErrForbidden) {
		t.Fatalf("deal create without the grant: err = %v, want ErrForbidden", err)
	}

	// Create but not update: a move is refused.
	dealed(e.Rep3, "deal-create-only", `{"objects":{"deal":{"read":true,"create":true}},"row_scope":"all"}`)
	made, err := e.ingest(t, e.Rep3, e.dealRecord("opp-3", nil))
	if err != nil {
		t.Fatalf("deal create with the grant: %v", err)
	}
	if _, err := e.ingest(t, e.Rep3, e.dealRecord("opp-3", func(d *extension.DealFields) { d.StageID = e.won.String() })); !errors.Is(err, extension.ErrForbidden) {
		t.Fatalf("move without update: err = %v, want ErrForbidden (deal %s)", err, made.Ref.ID)
	}

	// Adopt needs update on the company it takes over.
	if _, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{
		DisplayName: "Hand made", Domains: []contacts.CompanyDomainInput{{Domain: "adopt.test", IsPrimary: true}},
	}); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	adopt := companyRecord("acct-3", extension.CompanyFields{DisplayName: "A", Domain: "adopt.test", AdoptOn: extension.AdoptKeyDomain})
	dealed(e.AdminUser, "company-create-only", `{"objects":{"company":{"read":true,"create":true}},"row_scope":"all"}`)
	if _, err := e.ingest(t, e.AdminUser, adopt); !errors.Is(err, extension.ErrForbidden) {
		t.Fatalf("adopt without update: err = %v, want ErrForbidden", err)
	}
}

func TestAnActivityIsRelinkedOntoACompanyByItsKey(t *testing.T) {
	e := setupCRMIngress(t)
	activity := aProviderRecord("msg-1", "outside@example.test")
	activity.System = crmSystem
	landed := e.mustIngest(t, activity)

	company := e.mustIngest(t, companyRecord("acct-5", extension.CompanyFields{
		DisplayName: "Acme", RelinkActivities: []string{"msg-1", "no-such-message"},
	}))
	got := e.countAsWorkspace(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1 AND company_id = $2`,
		ids.MustParse(landed.Ref.ID), ids.MustParse(company.Ref.ID))
	if got != 1 {
		t.Fatalf("links from the activity to the company = %d, want 1 (an unknown key is skipped, not an error)", got)
	}
}

func TestReplayedAndRacingRecordsLeaveOneRowAndOneAuditEntryPerChange(t *testing.T) {
	e := setupCRMIngress(t)
	rec := companyRecord("acct-race", extension.CompanyFields{DisplayName: "Acme", Industry: "software"})

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Go(func() {
			if _, err := e.ingest(t, e.member, rec); err != nil {
				errs <- err
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("a racing ingest of one outside id failed: %v", err)
	}
	e.mustIngest(t, rec)
	if got := e.countAsWorkspace(t, `SELECT count(*) FROM company WHERE source_id = 'acct-race'`); got != 1 {
		t.Fatalf("companies = %d after the race, want 1", got)
	}
	creates := e.countAsWorkspace(t, `
		SELECT count(*) FROM audit_log a JOIN company c ON c.id = a.entity_id
		 WHERE c.source_id = 'acct-race' AND a.action = 'create'`)
	if creates != 1 {
		t.Errorf("create audit rows = %d, want exactly 1", creates)
	}
}

// A win a unit reports is a stage move like any other. The deal's audit trail
// must name it, so the deal shows who moved it to won.
func TestAStageMoveALandedRecordMakesIsInTheDealsAuditTrail(t *testing.T) {
	e := setupCRMIngress(t)
	made := e.mustIngest(t, e.dealRecord("opp-audit", nil))
	e.mustIngest(t, e.dealRecord("opp-audit", func(d *extension.DealFields) { d.StageID = e.won.String() }))

	rows := e.countAsWorkspace(t, `SELECT count(*) FROM audit_log WHERE entity_id = $1 AND action = 'advance_stage'`,
		ids.MustParse(made.Ref.ID))
	if rows != 1 {
		t.Errorf("advance_stage audit rows for the deal = %d, want 1", rows)
	}
}
