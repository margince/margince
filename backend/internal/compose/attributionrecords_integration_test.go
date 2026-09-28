// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Recording who created a CONTACT, COMPANY, LEAD, DEAL or PROJECT in the system
// it was imported from.
//
// The activity half of this route is covered next door, in
// attributionrepair_integration_test.go, and those tests exercise the ledger,
// the revision gate and the batch-level refusals once for all six types. What
// is NOT shared is which store a row reaches and what that store does when it
// gets there — five tables, four modules, five different sets of triggers and
// constraints — and none of that had a test.
//
// So these are the record-path questions:
//
//   - each type reaches the store that owns its table, and the ledger records
//     it under its OWN object_type rather than the activity's;
//   - a record that names no source system is refused BEFORE the write, not by
//     the NOT VALID check firing mid-batch as a 500;
//   - a record already carrying the answer is left alone, version included,
//     because every one of these tables bumps `version` on any UPDATE;
//   - attributing a deal records no forecast movement, which is the thing
//     routing the write through applyDealPatchLocked is supposed to get right.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// recordHandlers builds the route with every store wired, which is what
// distinguishes these tests from the activity suite's handler literal: a
// missing store here is a nil dereference on dispatch rather than a refusal,
// so the wiring is part of what is under test.
func recordHandlers(e *integration.Env) attributionHandlers {
	db := InstallationDB(e.Pool)
	return attributionHandlers{
		db:         db,
		activities: activities.NewStore(db),
		contacts:   contacts.NewStore(db),
		deals:      deals.NewStore(db, DealsInstallation()),
		projects:   ProjectsStoreOver(db),
	}
}

// importedRecords is one seeded row of every record type, each carrying a
// source system so it is attributable, and each captured by the seat that ran
// the import rather than by whoever wrote it.
type importedRecords struct {
	contact, company, lead, deal, project ids.UUID
}

// seedImportedRecords writes one of each, the way the HubSpot importer leaves
// them: `source_system` set, `captured_by` naming the administrator who ran the
// import, and both author columns empty.
//
// RAW INSERTS rather than the module stores, because the stores stamp
// `captured_by` from the caller and these fixtures need it to name the seat
// that ran the import.
//
// The stores CAN now carry `source_system` — the four record create wires
// gained it — and recordsourcesystem_integration_test.go drives them that way
// end to end. These fixtures stay raw so this suite keeps reaching all five
// types from one seeding shape, the lead included.
func seedImportedRecords(t *testing.T, e *integration.Env) importedRecords {
	t.Helper()
	r := importedRecords{
		contact: ids.NewV7(), company: ids.NewV7(), lead: ids.NewV7(),
		deal: ids.NewV7(), project: ids.NewV7(),
	}
	e.WsExec(t, `
		INSERT INTO contact (id, full_name, source, captured_by, source_system)
		VALUES ($1, 'Imported Contact', 'hubspot_import', 'human:'||$2, 'hubspot')`,
		r.contact, e.AdminUser)
	e.WsExec(t, `
		INSERT INTO company (id, display_name, source, captured_by, source_system)
		VALUES ($1, 'Imported Company', 'hubspot_import', 'human:'||$2, 'hubspot')`,
		r.company, e.AdminUser)
	e.WsExec(t, `
		INSERT INTO lead (id, full_name, source, captured_by, source_system)
		VALUES ($1, 'Imported Lead', 'hubspot_import', 'human:'||$2, 'hubspot')`,
		r.lead, e.AdminUser)
	// The project hangs off the company above. `company_id` is nullable, so this
	// is not forced — an imported project that named a company in HubSpot is
	// simply the ordinary case, and a fixture without the edge would model the
	// rarer one.
	e.WsExec(t, `
		INSERT INTO project (id, name, company_id, source, captured_by, source_system)
		VALUES ($1, 'Imported Project', $2, 'hubspot_import', 'human:'||$3, 'hubspot')`,
		r.project, r.company, e.AdminUser)
	pipeline, stage := ids.NewV7(), ids.NewV7()
	e.WsExec(t, `INSERT INTO pipeline (id, name, is_default, position) VALUES ($1, 'Sales', true, 0)`, pipeline)
	e.WsExec(t, `
		INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability)
		VALUES ($1, $2, 'Qualify', 0, 'open', 10)`, stage, pipeline)
	e.WsExec(t, `
		INSERT INTO deal (id, name, pipeline_id, stage_id, source, captured_by, source_system)
		VALUES ($1, 'Imported Deal', $2, $3, 'hubspot_import', 'human:'||$4, 'hubspot')`,
		r.deal, pipeline, stage, e.AdminUser)
	return r
}

// recordRow is one row of a batch, spelled per type so a test reads as the list
// of records it attributes.
func recordRow(objectType crmcontracts.SourceAttributionRowObjectType, id ids.UUID, name string, revision int64) crmcontracts.SourceAttributionRow {
	return crmcontracts.SourceAttributionRow{
		ObjectType:       objectType,
		ObjectId:         openapi_types.UUID(id),
		SourceRevision:   revision,
		SourceAuthorName: &name,
	}
}

// authorOnTable reads an author back off one of the five record tables.
//
// The table is a LITERAL at every call below, for the reason the stores spell
// theirs out: a helper interpolating a caller's string into SQL is the shape
// this branch spent a review round removing.
func authorOnTable(t *testing.T, e *integration.Env, query string, id ids.UUID) *string {
	t.Helper()
	var name *string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), query, id).Scan(&name)
	}); err != nil {
		t.Fatalf("reading the author back: %v", err)
	}
	return name
}

// ledgerRevisionFor reads the ledger under a GIVEN object type, which is the
// half the activity suite cannot check: it only ever reads `'activity'`.
func ledgerRevisionFor(t *testing.T, e *integration.Env, objectType string, id ids.UUID) *int64 {
	t.Helper()
	var revision *int64
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(context.Background(), `
			SELECT source_revision FROM source_attribution_repair
			 WHERE object_type = $1 AND object_id = $2`, objectType, id).Scan(&revision)
		if err == pgx.ErrNoRows {
			return nil
		}
		return err
	}); err != nil {
		t.Fatalf("reading the ledger for a %s: %v", objectType, err)
	}
	return revision
}

// TestEachRecordTypeIsAttributedThroughItsOwnStore is the dispatch test, and it
// sends all five in ONE batch on purpose.
//
// Five separate batches would pass with every arm of the switch pointing at the
// same store, because each type would still find its own row by id. One batch
// naming five different tables does not: a mis-pointed arm looks for a contact
// id in the deal table, finds nothing, and skips.
//
// It also reads the LEDGER per type. The ledger is keyed (object_type,
// object_id), so a route that stamped `'activity'` for every row would attribute
// the records correctly and then record all five revisions under one key —
// after which the revision gate stops protecting four of them.
func TestEachRecordTypeIsAttributedThroughItsOwnStore(t *testing.T) {
	e := integration.Setup(t)
	h := recordHandlers(e)
	r := seedImportedRecords(t, e)

	out := repair(t, e, h, crmcontracts.SourceAttributionRequest{
		BatchRef: "hubspot-mirror-2026-09-18",
		Rows: []crmcontracts.SourceAttributionRow{
			recordRow(crmcontracts.SourceAttributionRowObjectTypeContact, r.contact, "Mutaz Suleiman", 1),
			recordRow(crmcontracts.SourceAttributionRowObjectTypeCompany, r.company, "Shayne Ahsan", 1),
			recordRow(crmcontracts.SourceAttributionRowObjectTypeLead, r.lead, "Renée Fischer", 1),
			recordRow(crmcontracts.SourceAttributionRowObjectTypeDeal, r.deal, "Mutaz Suleiman", 1),
			recordRow(crmcontracts.SourceAttributionRowObjectTypeProject, r.project, "Shayne Ahsan", 1),
		},
	})

	if out.Applied != 5 {
		t.Fatalf("the five-record batch answered %+v, want all five applied", out)
	}

	for _, want := range []struct {
		objectType, query, author string
		id                        ids.UUID
	}{
		{"contact", `SELECT source_author_name FROM contact WHERE id = $1`, "Mutaz Suleiman", r.contact},
		{"company", `SELECT source_author_name FROM company WHERE id = $1`, "Shayne Ahsan", r.company},
		{"lead", `SELECT source_author_name FROM lead WHERE id = $1`, "Renée Fischer", r.lead},
		{"deal", `SELECT source_author_name FROM deal WHERE id = $1`, "Mutaz Suleiman", r.deal},
		{"project", `SELECT source_author_name FROM project WHERE id = $1`, "Shayne Ahsan", r.project},
	} {
		if got := authorOnTable(t, e, want.query, want.id); got == nil || *got != want.author {
			t.Errorf("the %s reads author %v, want %q — its batch row reached the wrong store",
				want.objectType, got, want.author)
		}
		if got := ledgerRevisionFor(t, e, want.objectType, want.id); got == nil || *got != 1 {
			t.Errorf("the ledger holds revision %v under object_type %q, want 1 — a revision "+
				"recorded under the wrong key stops guarding this record", got, want.objectType)
		}
	}
}

// TestARecordFromNowhereIsRefusedBeforeTheWrite covers the constraint this
// branch's migration added, from the side that matters.
//
// `contact_source_author_needs_a_source` is NOT VALID, so it does not bite on
// the rows already there — it bites on the next UPDATE. If the store did not
// refuse first, a batch containing one hand-typed contact would reach the
// column, take a constraint violation mid-transaction, and answer 500 for a
// case that is ordinary: five thousand records walked from HubSpot will include
// some a rep typed here last week.
//
// The imported row in the same batch is the positive control. Without it this
// passes on a route that refused everything.
func TestARecordFromNowhereIsRefusedBeforeTheWrite(t *testing.T) {
	e := integration.Setup(t)
	h := recordHandlers(e)
	r := seedImportedRecords(t, e)

	// Typed here: no source system, so there is no "there" for an author to have
	// written it in.
	typedHere := ids.NewV7()
	e.WsExec(t, `
		INSERT INTO contact (id, full_name, source, captured_by)
		VALUES ($1, 'Typed Here', 'manual', 'human:'||$2)`, typedHere, e.AdminUser)

	out := repair(t, e, h, crmcontracts.SourceAttributionRequest{
		BatchRef: "mixed-origins",
		Rows: []crmcontracts.SourceAttributionRow{
			recordRow(crmcontracts.SourceAttributionRowObjectTypeContact, typedHere, "Mutaz Suleiman", 1),
			recordRow(crmcontracts.SourceAttributionRowObjectTypeContact, r.contact, "Mutaz Suleiman", 1),
		},
	})

	if out.Skipped != 1 || out.Applied != 1 {
		t.Fatalf("the mixed batch answered %+v, want one skipped and one applied — a record "+
			"from nowhere must not abort the rows beside it", out)
	}
	if out.Rows[0].Outcome != "skipped" || out.Rows[0].Reason == nil {
		t.Errorf("the hand-typed contact answered %+v, want a skip carrying its reason", out.Rows[0])
	}
	if got := authorOnTable(t, e, `SELECT source_author_name FROM contact WHERE id = $1`, typedHere); got != nil {
		t.Errorf("a contact that came from nowhere was attributed to %v", got)
	}
	if got := ledgerRevisionFor(t, e, "contact", typedHere); got != nil {
		t.Errorf("the refused contact left revision %v in the ledger — the next run would read "+
			"it and skip the record forever", got)
	}
}

// TestARecordAlreadyCarryingTheAnswerIsNotRewritten is about `version`, not
// about the answer.
//
// Every one of these five tables carries a BEFORE UPDATE trigger that bumps
// `version` and `updated_at` on any write at all. A repair that re-applied an
// identical author would restamp five thousand companies and eight thousand
// contacts for no change — and `version` is what optimistic concurrency reads,
// so every open editor in the product would be told its copy is stale.
func TestARecordAlreadyCarryingTheAnswerIsNotRewritten(t *testing.T) {
	e := integration.Setup(t)
	h := recordHandlers(e)
	r := seedImportedRecords(t, e)

	first := crmcontracts.SourceAttributionRequest{
		BatchRef: "first-pass",
		Rows: []crmcontracts.SourceAttributionRow{
			recordRow(crmcontracts.SourceAttributionRowObjectTypeContact, r.contact, "Mutaz Suleiman", 1),
			recordRow(crmcontracts.SourceAttributionRowObjectTypeCompany, r.company, "Shayne Ahsan", 1),
		},
	}
	if out := repair(t, e, h, first); out.Applied != 2 {
		t.Fatalf("the first pass answered %+v, want two applied", out)
	}
	contactVersion := recordVersion(t, e, `SELECT version FROM contact WHERE id = $1`, r.contact)
	companyVersion := recordVersion(t, e, `SELECT version FROM company WHERE id = $1`, r.company)

	// A resumed run: the same answer, a higher counter.
	again := repair(t, e, h, crmcontracts.SourceAttributionRequest{
		BatchRef: "resumed",
		Rows: []crmcontracts.SourceAttributionRow{
			recordRow(crmcontracts.SourceAttributionRowObjectTypeContact, r.contact, "Mutaz Suleiman", 2),
			recordRow(crmcontracts.SourceAttributionRowObjectTypeCompany, r.company, "Shayne Ahsan", 2),
		},
	})
	if again.Unchanged != 2 {
		t.Errorf("re-sending the same two answers reported %+v, want both unchanged", again)
	}
	if got := recordVersion(t, e, `SELECT version FROM contact WHERE id = $1`, r.contact); got != contactVersion {
		t.Errorf("the contact's version moved %d -> %d on an identical answer", contactVersion, got)
	}
	if got := recordVersion(t, e, `SELECT version FROM company WHERE id = $1`, r.company); got != companyVersion {
		t.Errorf("the company's version moved %d -> %d on an identical answer", companyVersion, got)
	}
	// The ledger advances anyway, for the reason the activity suite pins: an
	// older revision left on record lets a delayed, DIFFERENT answer win.
	if got := ledgerRevisionFor(t, e, "contact", r.contact); got == nil || *got != 2 {
		t.Errorf("the contact's ledger reads revision %v after an unchanged answer at 2", got)
	}
}

// recordVersion reads the optimistic-concurrency counter every record table
// bumps on any UPDATE. The query is a literal at each call, like authorOnTable's.
func recordVersion(t *testing.T, e *integration.Env, query string, id ids.UUID) int64 {
	t.Helper()
	var v int64
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), query, id).Scan(&v)
	}); err != nil {
		t.Fatalf("reading the record's version: %v", err)
	}
	return v
}

// TestAttributingADealMovesNoForecast is why SetDealSourceAuthorTx goes through
// applyDealPatchLocked instead of calling storekit.ApplyLocked itself.
//
// The deal module routes every write through that seam so a change to amount,
// stage or close date lands a `deal_forecast_history` row. The gate
// (gates/dealforecastmovement_test.go) enforces the routing; it cannot enforce
// what the seam then does with a patch that touches neither. This asserts the
// answer: a byline is not a forecast change, and recording one as movement would
// put a spurious entry in front of whoever reads the deal's forecast history.
func TestAttributingADealMovesNoForecast(t *testing.T) {
	e := integration.Setup(t)
	h := recordHandlers(e)
	r := seedImportedRecords(t, e)

	before := e.WsCount(t, `SELECT count(*) FROM deal_forecast_history WHERE deal_id = $1`, r.deal)

	if out := repair(t, e, h, crmcontracts.SourceAttributionRequest{
		BatchRef: "deal-byline",
		Rows: []crmcontracts.SourceAttributionRow{
			recordRow(crmcontracts.SourceAttributionRowObjectTypeDeal, r.deal, "Mutaz Suleiman", 1),
		},
	}); out.Applied != 1 {
		t.Fatalf("attributing the deal answered %+v, want one applied", out)
	}

	if after := e.WsCount(t, `SELECT count(*) FROM deal_forecast_history WHERE deal_id = $1`, r.deal); after != before {
		t.Errorf("attributing a deal wrote %d forecast rows (was %d) — recording who wrote it in "+
			"HubSpot is not a forecast change, and every imported deal would carry a false entry",
			after-before, before)
	}
}
