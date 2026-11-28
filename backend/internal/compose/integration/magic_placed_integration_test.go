// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// An offer, a contract or a file has no page of its own. Done for you names and
// links the record it belongs to, on Home and in the Worklist alike. It does so
// only for a reader who may see both.

import (
	"context"
	"maps"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/completedchanges"
	"github.com/margince/margince/backend/internal/compose/magic"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/assignments"
	"github.com/margince/margince/backend/internal/modules/contracts"
	"github.com/margince/margince/backend/internal/modules/dealrooms"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/introductions"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// placedFixture is one deal, its company and a contact, all Rep1's, and the
// agent acting for Rep1 that writes the children.
type placedFixture struct {
	e                      *Env
	deal, company, contact ids.UUID
	stage                  ids.StageID
	ledger                 *ledgerSync
}

// placedGrants are the child grants a placed reader or writer holds on top of
// a fixture's own.
var placedGrants = []string{"offer", "contract", "deal_room", "introduction", grantFinanceObject, objCompany, objActivity}

func withPlacedGrants(perms principal.Permissions, grant principal.ObjectGrant) principal.Permissions {
	perms.Objects = maps.Clone(perms.Objects)
	for _, object := range placedGrants {
		perms.Objects[object] = grant
	}
	return perms
}

func newPlacedFixture(t *testing.T) placedFixture {
	t.Helper()
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	company := e.SeedCompany(t, "Placed Co", &e.Rep1)
	companyID, owner := ids.From[ids.CompanyKind](company), ids.From[ids.UserKind](e.Rep1)
	deal, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Source: "manual",
		Name:   "Placed rollout", PipelineID: pipeline, StageID: open, CompanyID: &companyID, OwnerID: &owner,
	})
	if err != nil {
		t.Fatal(err)
	}
	return placedFixture{e: e, deal: ids.UUID(deal.Id), company: company, stage: open, contact: e.SeedContact(t, "Placed Contact", &e.Rep1), ledger: &ledgerSync{}}
}

func (f placedFixture) writer(t *testing.T) context.Context {
	t.Helper()
	return f.e.AgentFor(t, f.e.Rep1, []ids.UUID{f.e.Team1}, withPlacedGrants(AdminPerms, principal.ObjectGrant{Create: true, Read: true, Update: true}))
}

// placedWriters write each placed child through its own store, as an agent,
// and name the sentence and the parent the line must show.
func placedWriters(f placedFixture) map[string]func(t *testing.T) (key string, parent ids.UUID) {
	e := f.e
	writers := map[string]func(t *testing.T) (string, ids.UUID){
		"offer": func(t *testing.T) (string, ids.UUID) {
			description, price, tax := "Rollout", int64(4_000_000), "0.00"
			_, err := e.Deals.CreateOffer(f.writer(t), ids.From[ids.DealKind](f.deal), deals.CreateOfferInput{
				Currency: "EUR", Source: "manual",
				LineItems: []deals.OfferLineInputRow{{Description: &description, Quantity: "1", UnitPriceMinor: &price, TaxRate: &tax}},
			})
			mustWrite(t, err)
			return "magic.action.create_offer", f.deal
		},
		"contract": func(t *testing.T) (string, ids.UUID) {
			dealID := ids.From[ids.DealKind](f.deal)
			_, err := e.Contracts.CreateContract(f.writer(t), contracts.CreateContractInput{
				CompanyID: ids.From[ids.CompanyKind](f.company), DealID: &dealID, Title: "Master agreement", ValueBasis: "total",
			})
			mustWrite(t, err)
			return "magic.action.create_contract", f.deal
		},
		"deal_room": func(t *testing.T) (string, ids.UUID) {
			_, err := dealrooms.NewStore(e.DB()).CreateRoom(f.writer(t), dealrooms.CreateRoomInput{
				DealID: ids.From[ids.DealKind](f.deal), Title: "Rollout room", Source: "manual",
			})
			mustWrite(t, err)
			return "magic.action.create_deal_room", f.deal
		},
		"record_assignment": func(t *testing.T) (string, ids.UUID) {
			role, err := ids.Parse(e.WsScalar(t, `SELECT id::text FROM record_role WHERE 'deal' = ANY(record_types) AND active ORDER BY sort_order LIMIT 1`))
			mustWrite(t, err)
			_, err = assignments.NewStore(e.DB()).CreateAssignment(f.writer(t), assignments.CreateAssignmentInput{
				RecordType: crmcontracts.AssignmentRecordTypeDeal, RecordID: f.deal,
				SubjectKind: crmcontracts.AssignmentSubjectKindUser, SubjectID: e.Rep2, RoleID: role,
			})
			mustWrite(t, err)
			return "magic.action.create_assignment", f.deal
		},
		"attachment": func(t *testing.T) (string, ids.UUID) {
			_, err := e.Activities.AddAttachmentLink(f.writer(t), activities.LinkInput{
				EntityType: "company", EntityID: f.company, URL: "https://example.com/brief", Title: "Brief",
			})
			mustWrite(t, err)
			return "magic.action.create_attachment", f.company
		},
		"attachment on a deal": func(t *testing.T) (string, ids.UUID) {
			_, err := e.Activities.AddAttachmentLink(f.writer(t), activities.LinkInput{
				EntityType: "deal", EntityID: f.deal, URL: "https://example.com/plan", Title: "Plan",
			})
			mustWrite(t, err)
			return "magic.action.create_attachment", f.deal
		},
		"record_assignment on a company": func(t *testing.T) (string, ids.UUID) {
			role, err := ids.Parse(e.WsScalar(t, `SELECT id::text FROM record_role WHERE 'company' = ANY(record_types) AND active ORDER BY sort_order LIMIT 1`))
			mustWrite(t, err)
			_, err = assignments.NewStore(e.DB()).CreateAssignment(f.writer(t), assignments.CreateAssignmentInput{
				RecordType: crmcontracts.AssignmentRecordTypeCompany, RecordID: f.company,
				SubjectKind: crmcontracts.AssignmentSubjectKindUser, SubjectID: e.Rep2, RoleID: role,
			})
			mustWrite(t, err)
			return "magic.action.create_assignment", f.company
		},
		"deal_stage_evidence": func(t *testing.T) (string, ids.UUID) {
			return "magic.action.create_stage_evidence", f.writeEvidence(t)
		},
		"intro_request": func(t *testing.T) (string, ids.UUID) {
			_, err := introductions.NewStore(e.DB(), func() time.Time { return clocktest.Now(t) }).Record(f.writer(t), introductions.NewRecorded{
				ContactID: f.contact, IntroducerUser: e.Rep1, IntroducedUser: e.Rep2,
				On: clocktest.Now(t), Channel: string(crmcontracts.IntroRecordedChannelIntroRecordedChannelEmail),
			})
			mustWrite(t, err)
			return "magic.action.create_introduction", f.contact
		},
	}
	maps.Copy(writers, ledgerWriters(f))
	return writers
}

// writeEvidence records one stage-evidence claim on the deal through the
// ledger's writer, from a mail linked to the deal, and answers the deal.
func (f placedFixture) writeEvidence(t *testing.T) ids.UUID {
	t.Helper()
	criterion, err := f.e.Deals.CreateStageExitCriterion(f.e.Admin(), deals.CreateCriterionInput{
		StageID: f.stage, Key: "budget_named", Label: "Budget named", Kind: string(deals.CriterionCustom),
	})
	mustWrite(t, err)
	mail := ids.NewV7()
	f.e.WsExec(t, `INSERT INTO activity (id, kind, subject, direction, occurred_at, source, captured_by)
		VALUES ($1, 'email', 'Budget', 'inbound', $2, 'gmail:seed', 'connector:gmail')`, mail, clocktest.Now(t).Add(-time.Hour))
	f.e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, deal_id) VALUES ($1, 'deal', $2)`, mail, f.deal)
	_, err = f.e.Deals.RecordStageEvidence(f.writer(t), deals.EvidenceInput{
		DealID: ids.From[ids.DealKind](f.deal), CriterionID: ids.From[ids.ExitCriterionKind](ids.UUID(criterion.Id)),
		SourceType: deals.SourceActivity, SourceID: mail, AuthorSide: deals.AuthorUnknown,
		Commitment: deals.CommitmentNone, ObservedAt: clocktest.Now(t).Add(-time.Hour), ExtractedBy: deals.ExtractedByDeterministic,
	})
	mustWrite(t, err)
	return f.deal
}

func mustWrite(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("writing the child: %v", err)
	}
}

// placedLines answers, per sentence, the record each placed line names on
// Home and in the Worklist's change history.
func placedLines(ctx context.Context, t *testing.T, e *Env) (home, worklist map[string]ids.UUID) {
	t.Helper()
	since := clocktest.Now(t).Add(-time.Hour)
	svc := magic.NewService(e.Pool, nil, func() time.Time { return clocktest.Now(t) })
	receipt, err := svc.Read(ctx, &since, 100)
	if err != nil {
		t.Fatalf("reading Home's receipt: %v", err)
	}
	home = map[string]ids.UUID{}
	for _, line := range receipt.Done {
		if line.Entity != nil {
			home[line.Summary.Key] = ids.UUID(line.Entity.Id)
			if line.Before != nil || line.After != nil {
				t.Errorf("%s shows the child's images, which no field mask governs", line.Summary.Key)
			}
		}
	}
	read, err := completedchanges.Reader{Magic: svc}.RecentWithState(ctx, since, time.Time{}, 100)
	if err != nil {
		t.Fatalf("reading the Worklist's change history: %v", err)
	}
	worklist = map[string]ids.UUID{}
	for _, receipt := range read.Receipts {
		worklist[receipt.Kind] = receipt.TargetID
	}
	return home, worklist
}

// placedChildren are the child types placedWriters writes.
var placedChildren = []string{
	"offer", "contract", "deal_room", "record_assignment", "record_assignment on a company",
	"attachment", "attachment on a deal", "deal_stage_evidence", "intro_request",
	"deal_suggestion", "finance_invoice", "finance_payment",
}

func TestDoneForYouPlacesEachChildOnTheRecordItBelongsTo(t *testing.T) {
	for _, child := range placedChildren {
		t.Run(child, func(t *testing.T) {
			f := newPlacedFixture(t)
			key, parent := placedWriters(f)[child](t)
			reader := f.e.As(f.e.Rep1, []ids.UUID{f.e.Team1}, withPlacedGrants(RepPerms, principal.ObjectGrant{Read: true}))
			home, worklist := placedLines(reader, t, f.e)
			if home[key] != parent {
				t.Errorf("Home names %v for %s, want the parent %v (lines %v)", home[key], key, parent, home)
			}
			if worklist[key] != parent {
				t.Errorf("the Worklist names %v for %s, want the parent %v (receipts %v)", worklist[key], key, parent, worklist)
			}
		})
	}
}

// writeEveryChild writes one of each placed child and answers their sentences.
func writeEveryChild(t *testing.T, f placedFixture) []string {
	t.Helper()
	writers := placedWriters(f)
	keys := make([]string, 0, len(placedChildren))
	for _, child := range placedChildren {
		key, _ := writers[child](t)
		keys = append(keys, key)
	}
	return keys
}

// A seat on another team does not work on the records, so it is shown none of
// the children.
func TestDoneForYouHidesAChildOnARecordTheReaderDoesNotWorkOn(t *testing.T) {
	f := newPlacedFixture(t)
	keys := writeEveryChild(t, f)
	home, worklist := placedLines(f.e.As(f.e.Rep3, []ids.UUID{f.e.Team2}, withPlacedGrants(RepPerms, principal.ObjectGrant{Read: true})), t, f.e)
	for _, key := range keys {
		if _, shown := home[key]; shown {
			t.Errorf("Home shows %s on another team's record", key)
		}
		if _, shown := worklist[key]; shown {
			t.Errorf("the Worklist shows %s on another team's record", key)
		}
	}
}

// A teammate works on the records but may not see a private company or contact.
// The file and the introduction on them stay hidden from that teammate.
func TestDoneForYouHidesAChildOnARecordKeptPrivateFromTheReader(t *testing.T) {
	f := newPlacedFixture(t)
	writers := placedWriters(f)
	writers["attachment"](t)
	writers["intro_request"](t)
	f.e.WsExec(t, `UPDATE company SET visibility = 'owner' WHERE id = $1`, f.company)
	f.e.WsExec(t, `UPDATE contact SET visibility = 'owner' WHERE id = $1`, f.contact)
	reader := f.e.As(f.e.Rep2, []ids.UUID{f.e.Team1}, withPlacedGrants(RepPerms, principal.ObjectGrant{Read: true}))
	home, worklist := placedLines(reader, t, f.e)
	for _, key := range []string{"magic.action.create_attachment", "magic.action.create_introduction"} {
		if _, shown := home[key]; shown {
			t.Errorf("Home shows %s on a record kept private from the reader", key)
		}
		if _, shown := worklist[key]; shown {
			t.Errorf("the Worklist shows %s on a record kept private from the reader", key)
		}
	}
	owner, _ := placedLines(f.e.As(f.e.Rep1, []ids.UUID{f.e.Team1}, withPlacedGrants(RepPerms, principal.ObjectGrant{Read: true})), t, f.e)
	if owner["magic.action.create_attachment"] != f.company || owner["magic.action.create_introduction"] != f.contact {
		t.Fatalf("the owner lost the lines on their own private records: %v", owner)
	}
}

// Each child's own grant is asked as well as its parent's. A seat missing one
// loses those lines and keeps the rest. Without the deal grant, nothing placed
// on a deal shows.
func TestDoneForYouHidesAChildWhoseGrantTheReaderLacks(t *testing.T) {
	f := newPlacedFixture(t)
	writeEveryChild(t, f)
	onDeal := []string{"magic.action.create_offer", "magic.action.create_contract", "magic.action.create_deal_room", "magic.action.create_stage_evidence"}
	for grant, hidden := range map[string][]string{
		"offer":        {"magic.action.create_offer"},
		"contract":     {"magic.action.create_contract"},
		"deal_room":    {"magic.action.create_deal_room"},
		"introduction": {"magic.action.create_introduction"},
		objDeal:        onDeal,
	} {
		perms := withPlacedGrants(RepPerms, principal.ObjectGrant{Read: true})
		delete(perms.Objects, grant)
		home, worklist := placedLines(f.e.As(f.e.Rep1, []ids.UUID{f.e.Team1}, perms), t, f.e)
		for _, key := range hidden {
			if _, shown := home[key]; shown {
				t.Errorf("without %s, Home shows %s", grant, key)
			}
			if _, shown := worklist[key]; shown {
				t.Errorf("without %s, the Worklist shows %s", grant, key)
			}
		}
		if _, kept := home["magic.action.create_attachment"]; !kept {
			t.Errorf("a missing %s grant took the file lines with it: %v", grant, home)
		}
	}
}

// A contract on an archived deal is anchored on its own company. A teammate
// who sees the deal but not that private company is not shown the contract.
// The deal room on the same deal is the control that the deal is seen.
func TestDoneForYouHidesAContractAnchoredOnACompanyTheReaderMayNotSee(t *testing.T) {
	f := newPlacedFixture(t)
	writers := placedWriters(f)
	writers["contract"](t)
	writers["deal_room"](t)
	_, err := f.e.Deals.ArchiveDeal(f.e.Admin(), ids.From[ids.DealKind](f.deal), nil)
	mustWrite(t, err)
	f.e.WsExec(t, `UPDATE company SET visibility = 'owner' WHERE id = $1`, f.company)
	reader := f.e.As(f.e.Rep2, []ids.UUID{f.e.Team1}, withPlacedGrants(RepPerms, principal.ObjectGrant{Read: true}))
	home, worklist := placedLines(reader, t, f.e)
	if home["magic.action.create_deal_room"] != f.deal {
		t.Fatalf("the reader does not see the deal at all, so the case proves nothing: %v", home)
	}
	if _, shown := home["magic.action.create_contract"]; shown {
		t.Error("Home shows a contract anchored on a company kept private from the reader")
	}
	if _, shown := worklist["magic.action.create_contract"]; shown {
		t.Error("the Worklist shows a contract anchored on a company kept private from the reader")
	}
}

// A placed change leaves System health's count; one on a child no placement
// can reach stays in it.
func TestAPlacedChildLeavesTheUnplacedCount(t *testing.T) {
	f := newPlacedFixture(t)
	offer, err := f.e.Deals.CreateOffer(f.writer(t), ids.From[ids.DealKind](f.deal), deals.CreateOfferInput{Currency: "EUR", Source: "manual"})
	mustWrite(t, err)
	intro, version := "Updated by the agent", int64(*offer.Version)
	_, _, err = f.e.Deals.UpdateOffer(f.writer(t), ids.From[ids.OfferKind](ids.UUID(offer.Id)), deals.UpdateOfferInput{IntroText: &intro, IfVersion: &version})
	mustWrite(t, err)
	seedUnplaceableMachineAction(t, f.e)
	// No writer assigns an offer today. The row stands for a verb a future
	// writer adds before it has a sentence.
	err = database.WithTx(f.e.Admin(), f.e.Pool, func(tx pgx.Tx) error {
		_, err := storekit.Audit(principal.SystemActing(f.e.Admin(), "offer-sync"), tx, "assign", "offer", ids.UUID(offer.Id), nil, nil)
		return err
	})
	mustWrite(t, err)

	svc := magic.NewService(f.e.Pool, nil, func() time.Time { return clocktest.Now(t) })
	changes, err := svc.UnplacedChanges(f.e.Admin(), clocktest.Now(t).Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, change := range changes {
		got[change.EntityType+"/"+change.Action] = change.Count
	}
	if got["offer/update"] != 0 || got["offer/assign"] != 1 || got["relationship/update"] != 1 {
		t.Fatalf("the breakdown is %v, want the relationship update and the offer's unsentenced verb counted and the placed offer update not", got)
	}
	home, _ := placedLines(f.e.As(f.e.Rep1, []ids.UUID{f.e.Team1}, withPlacedGrants(RepPerms, principal.ObjectGrant{Read: true})), t, f.e)
	if home["magic.action.update_offer"] != f.deal {
		t.Fatalf("the offer update is not placed on its deal: %v", home)
	}
}

// An ask for an introduction is its two colleagues' alone, a rule of the
// introduction's own beyond its contact's scope. A teammate who works on the
// contact is not shown it.
func TestDoneForYouHidesAnIntroductionAskFromAColleagueOutsideIt(t *testing.T) {
	f := newPlacedFixture(t)
	_, err := introductions.NewStore(f.e.DB(), func() time.Time { return clocktest.Now(t) }).Create(f.writer(t), introductions.NewRequest{
		ContactID: f.contact, IntroducerUser: f.e.AdminUser, RouteType: "direct",
		InternalReason: "they run procurement", DueAt: clocktest.Now(t).AddDate(0, 0, 7),
	})
	mustWrite(t, err)
	perms := withPlacedGrants(RepPerms, principal.ObjectGrant{Read: true})
	asker, _ := placedLines(f.e.As(f.e.Rep1, []ids.UUID{f.e.Team1}, perms), t, f.e)
	if asker["magic.action.create_introduction"] != f.contact {
		t.Fatalf("the colleague who asked is not shown the ask: %v", asker)
	}
	teammate, worklist := placedLines(f.e.As(f.e.Rep2, []ids.UUID{f.e.Team1}, perms), t, f.e)
	if _, shown := teammate["magic.action.create_introduction"]; shown {
		t.Error("Home shows an ask to a colleague outside it")
	}
	if _, shown := worklist["magic.action.create_introduction"]; shown {
		t.Error("the Worklist shows an ask to a colleague outside it")
	}
}
