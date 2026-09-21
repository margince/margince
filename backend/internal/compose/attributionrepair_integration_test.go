// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Recording who wrote an imported activity in the system it came from.
//
// The behaviours that matter, in the order they matter:
//
//   - an attribution lands on the row and on the ledger beside it, in one
//     commit — a ledger that could say "done" about a write that rolled back
//     would tell the next run to skip the record forever;
//   - a replay at the same revision changes nothing, because a batch resumed
//     after an interruption walks records it has already reached;
//   - a LOWER revision after a correction does not overwrite it, which is the
//     one failure a payload hash alone cannot prevent: apply A, correct with B,
//     then a delayed retry of A arrives carrying a different hash;
//   - a row that came from nowhere is skipped with a reason rather than erroring
//     the batch, because five hundred records walked from another system will
//     always contain a few this installation cannot attribute.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seedImportedActivity writes an activity the way an importer leaves one:
// carrying a source system, and captured_by naming whoever ran the import
// rather than whoever wrote the message.
func seedImportedActivity(t *testing.T, e *integration.Env, sourceSystem string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO activity (id, kind, subject, body, direction, source_system, source_id,
			                      source, captured_by, occurred_at)
			VALUES ($1, 'email', 'Betreff', 'body', 'outbound', NULLIF($2,''), $3,
			        'hubspot_import', 'human:'||$4, now())`,
			id, sourceSystem, "attr-"+id.String(), e.AdminUser)
		return err
	}); err != nil {
		t.Fatalf("seeding an imported activity: %v", err)
	}
	return id
}

// repair drives the route the way the router would, and answers what it said
// about each row.
func repair(t *testing.T, e *integration.Env, h attributionHandlers, body crmcontracts.SourceAttributionRequest) crmcontracts.SourceAttributionResult {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("encoding the batch: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/records/attribution", bytes.NewReader(raw)).
		WithContext(e.Admin())
	rec := httptest.NewRecorder()
	h.RepairSourceAttribution(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("the repair answered %d: %s", rec.Code, rec.Body.String())
	}
	var out crmcontracts.SourceAttributionResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decoding the result: %v", err)
	}
	return out
}

// authorOn reads back what the row and its ledger now say, which is the pair a
// caller of this route is actually changing.
func authorOn(t *testing.T, e *integration.Env, id ids.UUID) (rowName *string, ledgerRevision *int64) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(context.Background(),
			`SELECT source_author_name FROM activity WHERE id = $1`, id).Scan(&rowName); err != nil {
			return err
		}
		return tx.QueryRow(context.Background(),
			`SELECT source_revision FROM source_attribution_repair
			  WHERE object_type = 'activity' AND object_id = $1`, id).Scan(&ledgerRevision)
	}); err != nil && err != pgx.ErrNoRows {
		t.Fatalf("reading the attribution back: %v", err)
	}
	return rowName, ledgerRevision
}

func row(id ids.UUID, name string, revision int64) crmcontracts.SourceAttributionRow {
	return crmcontracts.SourceAttributionRow{
		ObjectType:       crmcontracts.SourceAttributionRowObjectTypeActivity,
		ObjectId:         openapi_types.UUID(id),
		SourceRevision:   revision,
		SourceAuthorName: &name,
	}
}

func TestTheRepairRecordsAnAuthorAndItsLedgerTogether(t *testing.T) {
	e := integration.Setup(t)
	h := attributionHandlers{db: InstallationDB(e.Pool), activities: activities.NewStore(InstallationDB(e.Pool))}
	id := seedImportedActivity(t, e, "hubspot")

	out := repair(t, e, h, crmcontracts.SourceAttributionRequest{
		BatchRef: "test", Rows: []crmcontracts.SourceAttributionRow{row(id, "Mutaz Suleiman", 1)},
	})

	if out.Applied != 1 || out.Rows[0].Outcome != "applied" {
		t.Fatalf("the first attribution answered %+v, want one applied", out)
	}
	name, revision := authorOn(t, e, id)
	if name == nil || *name != "Mutaz Suleiman" {
		t.Errorf("the row's author reads %v, want the name the source carried", name)
	}
	if revision == nil || *revision != 1 {
		t.Errorf("the ledger reads revision %v, want 1 — the write and its record must land together", revision)
	}
}

func TestTheRepairIsANoOpAtTheSameRevision(t *testing.T) {
	e := integration.Setup(t)
	h := attributionHandlers{db: InstallationDB(e.Pool), activities: activities.NewStore(InstallationDB(e.Pool))}
	id := seedImportedActivity(t, e, "hubspot")

	first := crmcontracts.SourceAttributionRequest{
		BatchRef: "test", Rows: []crmcontracts.SourceAttributionRow{row(id, "Mutaz Suleiman", 1)},
	}
	repair(t, e, h, first)
	again := repair(t, e, h, first)

	if again.Unchanged != 1 || again.Rows[0].Outcome != "unchanged" {
		t.Errorf("replaying the batch answered %+v, want one unchanged — a resumed run walks records it already reached", again)
	}
}

func TestAStaleRetryDoesNotOverwriteACorrection(t *testing.T) {
	e := integration.Setup(t)
	h := attributionHandlers{db: InstallationDB(e.Pool), activities: activities.NewStore(InstallationDB(e.Pool))}
	id := seedImportedActivity(t, e, "hubspot")

	// The first answer, then the correction that supersedes it.
	repair(t, e, h, crmcontracts.SourceAttributionRequest{
		BatchRef: "first", Rows: []crmcontracts.SourceAttributionRow{row(id, "Wrong Name", 1)},
	})
	repair(t, e, h, crmcontracts.SourceAttributionRequest{
		BatchRef: "correction", Rows: []crmcontracts.SourceAttributionRow{row(id, "Mutaz Suleiman", 2)},
	})

	// The delayed retry of the FIRST batch. Its payload differs from the
	// correction's, so a hash comparison would admit it; only the revision
	// refuses it.
	stale := repair(t, e, h, crmcontracts.SourceAttributionRequest{
		BatchRef: "first", Rows: []crmcontracts.SourceAttributionRow{row(id, "Wrong Name", 1)},
	})

	if stale.Unchanged != 1 {
		t.Errorf("the stale retry answered %+v, want one unchanged", stale)
	}
	name, revision := authorOn(t, e, id)
	if name == nil || *name != "Mutaz Suleiman" {
		t.Errorf("the row's author reads %v after a stale retry — the correction was overwritten by an answer it already replaced", name)
	}
	if revision == nil || *revision != 2 {
		t.Errorf("the ledger reads revision %v, want the correction's 2", revision)
	}
}

func TestARowThatCameFromNowhereIsSkippedWithItsReason(t *testing.T) {
	e := integration.Setup(t)
	h := attributionHandlers{db: InstallationDB(e.Pool), activities: activities.NewStore(InstallationDB(e.Pool))}
	// No source system: typed here rather than imported, so there is no "there"
	// for an author to have written it in.
	typedHere := seedImportedActivity(t, e, "")
	imported := seedImportedActivity(t, e, "hubspot")

	out := repair(t, e, h, crmcontracts.SourceAttributionRequest{
		BatchRef: "test",
		Rows: []crmcontracts.SourceAttributionRow{
			row(typedHere, "Mutaz Suleiman", 1),
			row(imported, "Mutaz Suleiman", 1),
		},
	})

	if out.Skipped != 1 || out.Applied != 1 {
		t.Fatalf("the mixed batch answered %+v, want one skipped and one applied — a bad row may not fail its neighbours", out)
	}
	if out.Rows[0].Outcome != "skipped" || out.Rows[0].Reason == nil {
		t.Errorf("the unimportable row answered %+v, want a skip carrying its reason", out.Rows[0])
	}
	if name, _ := authorOn(t, e, typedHere); name != nil {
		t.Errorf("a row that came from nowhere was attributed to %v", name)
	}
}

// repairAs drives the route as a named seat, and answers the status rather than
// insisting on 200 — these are the refusal tests.
func repairAs(as context.Context, t *testing.T, h attributionHandlers, body crmcontracts.SourceAttributionRequest) int {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("encoding the batch: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/records/attribution", bytes.NewReader(raw)).WithContext(as)
	rec := httptest.NewRecorder()
	h.RepairSourceAttribution(rec, req)
	return rec.Code
}

// TestOnlyAnAdminMayRestateAuthorship is the proof that the admin gate is the
// thing refusing, and not the object grant standing in for it.
//
// The seat below is a rep who HAS `activity: update` — the grant the store
// itself takes — so the only authority left to refuse them is the admin check
// in the handler. Without it this seat would rewrite the recorded authorship of
// any activity it may edit, which on an imported timeline is most of them.
func TestOnlyAnAdminMayRestateAuthorship(t *testing.T) {
	e := integration.Setup(t)
	h := attributionHandlers{db: InstallationDB(e.Pool), activities: activities.NewStore(InstallationDB(e.Pool))}
	id := seedImportedActivity(t, e, "hubspot")

	// A rep who may edit activities, and nothing more.
	perms := principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects: map[string]principal.ObjectGrant{
			"activity": {Create: true, Read: true, Update: true},
		},
		RowScope: principal.RowScopeTeam,
	}
	batch := crmcontracts.SourceAttributionRequest{
		BatchRef: "test", Rows: []crmcontracts.SourceAttributionRow{row(id, "Mutaz Suleiman", 1)},
	}

	if code := repairAs(e.As(e.Rep1, nil, perms), t, h, batch); code != http.StatusForbidden {
		t.Errorf("a rep with activity:update reached the repair and got %d, want 403 — restating who wrote "+
			"somebody else's correspondence is not an ordinary edit", code)
	}
	if name, revision := authorOn(t, e, id); name != nil || revision != nil {
		t.Errorf("the refused call still wrote author=%v revision=%v", name, revision)
	}
	// The same batch, from an admin, lands — so the refusal above is the gate
	// and not a broken fixture.
	if code := repairAs(e.Admin(), t, h, batch); code != http.StatusOK {
		t.Fatalf("an admin got %d, want 200", code)
	}
}

// TestASkippedRowLeavesNoClaimBehind proves the ledger claim is released.
//
// The claim is taken BEFORE the activity write, so that two concurrent batches
// cannot both decide they are newer. That ordering means a row the writer then
// refuses has already been recorded — and a ledger entry for a record nothing
// was written to would tell the next run there is nothing left to try.
func TestASkippedRowLeavesNoClaimBehind(t *testing.T) {
	e := integration.Setup(t)
	h := attributionHandlers{db: InstallationDB(e.Pool), activities: activities.NewStore(InstallationDB(e.Pool))}
	typedHere := seedImportedActivity(t, e, "")

	repair(t, e, h, crmcontracts.SourceAttributionRequest{
		BatchRef: "test", Rows: []crmcontracts.SourceAttributionRow{row(typedHere, "Mutaz Suleiman", 1)},
	})

	if _, revision := authorOn(t, e, typedHere); revision != nil {
		t.Errorf("a skipped row left revision %v in the ledger — the next run would read it and skip the record forever", revision)
	}
}

// TestASkipDoesNotDestroyAnEarlierAnswer is the regression for the bug the
// claim-first ordering introduced and this one removes.
//
// The sequence is the whole point, and it needs no concurrency:
//
//	revision 10 lands a good author;
//	revision 11 arrives naming NOBODY, so the writer refuses it;
//	revision 9 — a delayed retry of a batch older than either — arrives last.
//
// Under claim-first, step 2 wrote its own revision over 10's ledger row and
// then deleted it on the way out, leaving nothing for step 3 to lose to. The
// stale author went in. Writing the activity BEFORE the ledger means a refused
// row records nothing and destroys nothing, so 10 is still on record when 9
// asks.
func TestASkipDoesNotDestroyAnEarlierAnswer(t *testing.T) {
	e := integration.Setup(t)
	h := attributionHandlers{db: InstallationDB(e.Pool), activities: activities.NewStore(InstallationDB(e.Pool))}
	id := seedImportedActivity(t, e, "hubspot")

	repair(t, e, h, crmcontracts.SourceAttributionRequest{
		BatchRef: "good", Rows: []crmcontracts.SourceAttributionRow{row(id, "Mutaz Suleiman", 10)},
	})

	// Neither author field: the writer refuses it rather than treating an
	// absent answer as "clear the attribution".
	skipped := repair(t, e, h, crmcontracts.SourceAttributionRequest{
		BatchRef: "empty",
		Rows: []crmcontracts.SourceAttributionRow{{
			ObjectType:     crmcontracts.SourceAttributionRowObjectTypeActivity,
			ObjectId:       openapi_types.UUID(id),
			SourceRevision: 11,
		}},
	})
	if skipped.Skipped != 1 {
		t.Fatalf("the empty-author row answered %+v, want one skipped", skipped)
	}

	stale := repair(t, e, h, crmcontracts.SourceAttributionRequest{
		BatchRef: "stale", Rows: []crmcontracts.SourceAttributionRow{row(id, "Wrong Name", 9)},
	})
	if stale.Unchanged != 1 {
		t.Errorf("the stale retry answered %+v, want one unchanged", stale)
	}

	name, revision := authorOn(t, e, id)
	if name == nil || *name != "Mutaz Suleiman" {
		t.Errorf("the author reads %v — a skipped row in between let a stale retry overwrite a good answer", name)
	}
	if revision == nil || *revision != 10 {
		t.Errorf("the ledger reads revision %v, want 10 still on record", revision)
	}
}

// TestAHeldRowSkipsWithoutTakingTheBatchDown proves what the SAVEPOINT in
// SetSourceAuthorTx is for.
//
// A row under a statutory retention hold is refused by a trigger, not by a
// predicate, so the refusal arrives at the UPDATE rather than at the lock. A
// failed statement aborts the whole transaction in Postgres: catching the error
// is not enough, because every statement after it — the ledger write, and every
// later row in the batch — fails with "current transaction is aborted". Without
// the savepoint one held activity takes down the other four hundred and
// ninety-nine while reporting itself as a mere skip.
//
// So the batch here is deliberately two rows, held first, and BOTH halves are
// assertions: the held row is skipped with a reason, and the ordinary row
// behind it still lands.
func TestAHeldRowSkipsWithoutTakingTheBatchDown(t *testing.T) {
	e := integration.Setup(t)
	h := attributionHandlers{db: InstallationDB(e.Pool), activities: activities.NewStore(InstallationDB(e.Pool))}
	held := seedImportedActivity(t, e, "hubspot")
	ordinary := seedImportedActivity(t, e, "hubspot")
	restrict(t, e, held)

	out := repair(t, e, h, crmcontracts.SourceAttributionRequest{
		BatchRef: "mixed",
		Rows: []crmcontracts.SourceAttributionRow{
			row(held, "Mutaz Suleiman", 1),
			row(ordinary, "Shayne Ahsan", 1),
		},
	})

	if out.Applied != 1 || out.Skipped != 1 {
		t.Fatalf("the mixed batch answered %+v, want one applied and one skipped — "+
			"a held row must not abort the rows behind it", out)
	}
	if out.Rows[0].Outcome != "skipped" || out.Rows[0].Reason == nil {
		t.Errorf("the held row answered %+v, want a skip carrying its reason", out.Rows[0])
	}

	if name, _ := authorOn(t, e, ordinary); name == nil || *name != "Shayne Ahsan" {
		t.Errorf("the row behind the held one reads %v, want its author committed", name)
	}
	if name, revision := authorOn(t, e, held); name != nil || revision != nil {
		t.Errorf("the held row reads author %v revision %v, want neither — "+
			"a skipped row writes nothing, ledger included", name, revision)
	}
}

// TestABatchLabelIsALabelAndNotASentence is hygiene at the wire, and is
// deliberately NOT the privacy control it was once written as.
//
// The pattern keeps a paragraph out of the column. It does NOT keep a human
// name out of it — `alice-smith` and `A.Smith` both satisfy it — and this
// branch twice shipped a census declaration claiming otherwise, on two
// different arguments, both false. What actually protects the label is that
// the Art. 17 erasure clears it, proved by
// TestAnErasureClearsWhatTheLedgerHoldsAboutTheErasedRecord.
//
// So what this test pins is narrow and true: the contract carries a pattern,
// the generated wrapper enforces no pattern, and the handler is the only thing
// that binds it.
func TestABatchLabelIsALabelAndNotASentence(t *testing.T) {
	e := integration.Setup(t)
	h := attributionHandlers{db: InstallationDB(e.Pool), activities: activities.NewStore(InstallationDB(e.Pool))}
	id := seedImportedActivity(t, e, "hubspot")

	prose := crmcontracts.SourceAttributionRequest{
		BatchRef: "Alice Smith authorship correction",
		Rows:     []crmcontracts.SourceAttributionRow{row(id, "Mutaz Suleiman", 1)},
	}
	// 422, the status httperr.Validation carries: the field is well-formed JSON
	// and the wrong value, which is what this refusal is about.
	if code := repairAs(e.Admin(), t, h, prose); code != http.StatusUnprocessableEntity {
		t.Errorf("a batch labelled with somebody's name answered %d, want 422 — this label "+
			"survives an erasure, so it must not be able to hold what an erasure removes", code)
	}
	if name, revision := authorOn(t, e, id); name != nil || revision != nil {
		t.Errorf("the refused batch still wrote author=%v revision=%v", name, revision)
	}

	// The same batch under a label that is a label.
	if code := repairAs(e.Admin(), t, h, crmcontracts.SourceAttributionRequest{
		BatchRef: "hubspot-mirror-2026-09-17",
		Rows:     prose.Rows,
	}); code != http.StatusOK {
		t.Fatalf("a well-formed label answered %d, want 200 — the refusal above must be the "+
			"pattern and not a broken fixture", code)
	}
}

// TestAnIdenticalAnswerAtAHigherRevisionRewritesNothing is what payload_hash is
// for, and the contract promises it.
//
// The revision orders two answers; it cannot say whether they DIFFER. A resumed
// run that raises its counter and re-sends the same attribution would otherwise
// rewrite identical values on every record it walks — and because
// trg_activity_updated forces updated_at and version on every UPDATE, thirty-four
// thousand activities would be restamped for no change at all.
func TestAnIdenticalAnswerAtAHigherRevisionRewritesNothing(t *testing.T) {
	e := integration.Setup(t)
	h := attributionHandlers{db: InstallationDB(e.Pool), activities: activities.NewStore(InstallationDB(e.Pool))}
	id := seedImportedActivity(t, e, "hubspot")

	repair(t, e, h, crmcontracts.SourceAttributionRequest{
		BatchRef: "first", Rows: []crmcontracts.SourceAttributionRow{row(id, "Mutaz Suleiman", 1)},
	})
	before := activityVersion(t, e, id)

	// A resumed run: same answer, higher counter.
	again := repair(t, e, h, crmcontracts.SourceAttributionRequest{
		BatchRef: "resumed", Rows: []crmcontracts.SourceAttributionRow{row(id, "Mutaz Suleiman", 2)},
	})
	if again.Unchanged != 1 {
		t.Errorf("re-sending the same answer at revision 2 reported %+v, want one unchanged", again)
	}
	if after := activityVersion(t, e, id); after != before {
		t.Errorf("the activity's version moved %d -> %d on an identical answer — "+
			"a resumed run would restamp every record it walks", before, after)
	}
	// AND THE LEDGER MOVED ANYWAY. Skipping the activity must not mean skipping
	// the bookkeeping: leaving revision 1 on record lets a delayed, DIFFERENT
	// answer at any revision above it win a comparison it should already have
	// lost. The assertion below is what the sequence after it depends on.
	if _, _, _, revision := ledgerRow(t, e, id); revision != 2 {
		t.Fatalf("the ledger still reads revision %d after an unchanged answer at 2 — "+
			"a stale answer in between would now overwrite a correct one", revision)
	}

	// The delayed batch that the advance above must now refuse: an older
	// revision than the one just recorded, carrying a different author.
	stale := repair(t, e, h, crmcontracts.SourceAttributionRequest{
		BatchRef: "delayed", Rows: []crmcontracts.SourceAttributionRow{row(id, "Wrong Name", 2)},
	})
	if stale.Unchanged != 1 {
		t.Errorf("the delayed answer at revision 2 reported %+v, want one unchanged", stale)
	}
	if name, _ := authorOn(t, e, id); name == nil || *name != "Mutaz Suleiman" {
		t.Errorf("the author reads %v — an unchanged answer that left the revision behind "+
			"let a stale one overwrite it", name)
	}
}

// The version read above comes from activityVersion in
// captureverdictreview_integration_test.go — the same package, the same
// question, and the column trg_activity_updated bumps on every UPDATE.

// ledgerRow reads every column of the repair's bookkeeping that an erasure has
// an opinion about — the two it must clear, and the two it must leave standing.
func ledgerRow(t *testing.T, e *integration.Env, id ids.UUID) (authorName *string, hash, batchRef string, revision int64) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT source_author_name, payload_hash, batch_ref, source_revision
			  FROM source_attribution_repair
			 WHERE object_type = 'activity' AND object_id = $1`, id).
			Scan(&authorName, &hash, &batchRef, &revision)
	}); err != nil {
		t.Fatalf("reading the ledger row: %v", err)
	}
	return authorName, hash, batchRef, revision
}

// TestAnErasureClearsWhatTheLedgerHoldsAboutTheErasedRecord is the proof behind
// this table's census declarations, none of which had a test until round three.
//
// The ledger holds the author's name a second time, and `payload_hash` holds it
// a third — an unkeyed SHA-256 over it. A digest is not anonymity when the
// candidate set is a staff list: a few hundred guesses re-identify it. So the
// erasure clears both, and this asserts both.
//
// The batch label goes with them. It is free text an operator types, and the
// wire pattern that once stood in for protecting it admits `alice-smith` —
// see TestABatchLabelIsALabelAndNotASentence. Nothing reads the column back,
// so clearing it costs a label on a record whose content is already destroyed.
//
// It asserts the OTHER half just as hard. `source_revision` survives on
// purpose, and that survival is what makes the rest safe: it is how the next
// repair run knows this record was already reached. Destroy it and a later
// batch re-attributes the erased message and writes the name back.
func TestAnErasureClearsWhatTheLedgerHoldsAboutTheErasedRecord(t *testing.T) {
	e := integration.Setup(t)
	h := attributionHandlers{db: InstallationDB(e.Pool), activities: activities.NewStore(InstallationDB(e.Pool))}

	subject := e.SeedContact(t, "Mara Kessler", nil)
	e.WsExec(t, `
		INSERT INTO contact_email (contact_id, email, is_primary, source, captured_by)
		VALUES ($1, 'mara.kessler@example.com', true, 'test', 'human:seed')`, subject)
	id := seedImportedActivity(t, e, "hubspot")
	e.WsExec(t, `
		INSERT INTO activity_link (activity_id, entity_type, contact_id)
		VALUES ($1, 'contact', $2)`, id, subject)

	repair(t, e, h, crmcontracts.SourceAttributionRequest{
		BatchRef: "hubspot-mirror-2026-09-17",
		Rows:     []crmcontracts.SourceAttributionRow{row(id, "Mutaz Suleiman", 7)},
	})
	if _, hash, _, _ := ledgerRow(t, e, id); hash == "" {
		t.Fatal("the attribution recorded no digest, so the assertion below would prove nothing")
	}

	if err := privacy.NewEraser(e.DB()).EraseContact(e.Admin(), subject, "subject request"); err != nil {
		t.Fatalf("EraseContact → %v", err)
	}

	authorName, hash, batchRef, revision := ledgerRow(t, e, id)
	if authorName != nil {
		t.Errorf("the ledger still names %q after the erasure — the name was cleared from the "+
			"message and left readable in the bookkeeping beside it", *authorName)
	}
	if hash != "" {
		t.Errorf("the ledger kept the digest %q — an unkeyed hash of a name drawn from a staff "+
			"list is re-identifiable, so it goes with the name it was taken over", hash)
	}
	if revision != 7 {
		t.Errorf("the erasure moved the revision to %d, want 7 kept — without it the next repair "+
			"run reads the record as unattributed and writes the erased name back", revision)
	}
	if batchRef != "" {
		t.Errorf("the erasure kept the batch label %q — it is free text an operator types, and no "+
			"wire pattern makes free text safe: `alice-smith` passes the one this route enforces", batchRef)
	}
}

// TestTheUnchangedAnswerIsNotAnOracle closes the confirmation channel that a
// caller-side "did anything change" comparison opened.
//
// The repair skips the write when the row already carries the offered answer.
// Decided in the HANDLER, as two rounds of this branch did decide it, that skip
// happens before the store's own visibility check — so an admin outside a
// limited message's audience could offer a guessed author and read the reply as
// an answer: a refusal means wrong, `unchanged` means right. They never read
// the row, and they learn what it says.
//
// Deciding it inside SetSourceAuthorTx, after EnsureActivityWritableIn, means
// there is nothing to read: the refusal comes first, and it is the same refusal
// — same outcome, same words — that a wrong guess gets.
//
// A POSITIVE CONTROL runs first, because every assertion here is about two
// calls answering alike, and two calls that both fail for an unrelated reason
// answer alike too. Review caught this test passing on that weaker property.
func TestTheUnchangedAnswerIsNotAnOracle(t *testing.T) {
	e := integration.Setup(t)
	h := attributionHandlers{db: InstallationDB(e.Pool), activities: activities.NewStore(InstallationDB(e.Pool))}
	owner := integration.OwnerConn(t)

	id := seedImportedActivity(t, e, "hubspot")
	repair(t, e, h, crmcontracts.SourceAttributionRequest{
		BatchRef: "seed", Rows: []crmcontracts.SourceAttributionRow{row(id, "Mutaz Suleiman", 1)},
	})
	if _, err := owner.Exec(context.Background(),
		`UPDATE activity SET audience = 'participants' WHERE id = $1`, id); err != nil {
		t.Fatalf("limiting the message: %v", err)
	}

	// An admin who is not on the message. Admin rights are the point: they are
	// what makes this a scope question rather than a permission one.
	outsider := e.As(e.Rep1, nil, integration.AdminPerms)

	// THE POSITIVE CONTROL FIRST. Without it every assertion below is satisfied
	// by an outsider who simply cannot reach the route at all — two 403s on some
	// unrelated ground compare equal and prove nothing. This says the seat works
	// here, so the refusals that follow are about the AUDIENCE.
	reachable := seedImportedActivity(t, e, "hubspot")
	control := repairBody(outsider, t, h, crmcontracts.SourceAttributionRequest{
		BatchRef: "control", Rows: []crmcontracts.SourceAttributionRow{row(reachable, "Mutaz Suleiman", 1)},
	})
	if control.Applied != 1 {
		t.Fatalf("the outsider answered %+v on an activity nothing limits — this seat cannot reach "+
			"the route at all, so the refusals below would prove nothing", control)
	}

	// Now the limited one. Both guesses must be refused, and refused THE SAME
	// WAY: an outcome that differed between a right and a wrong guess would tell
	// the caller the author without ever letting them read the row.
	rightRows := repairBody(outsider, t, h, crmcontracts.SourceAttributionRequest{
		BatchRef: "guess-right", Rows: []crmcontracts.SourceAttributionRow{row(id, "Mutaz Suleiman", 2)},
	})
	wrongRows := repairBody(outsider, t, h, crmcontracts.SourceAttributionRequest{
		BatchRef: "guess-wrong", Rows: []crmcontracts.SourceAttributionRow{row(id, "Someone Else", 3)},
	})
	if rightRows.Rows[0].Outcome != wrongRows.Rows[0].Outcome {
		t.Errorf("a correct guess reported %q and a wrong one %q — the difference IS the author, "+
			"handed to a caller outside the audience", rightRows.Rows[0].Outcome, wrongRows.Rows[0].Outcome)
	}
	// And the shared answer is the refusal, not a quiet success. `skipped` is
	// what EnsureActivityWritableIn produces for a row outside the caller's
	// audience — the same answer a deleted row gives, which is the point.
	if rightRows.Skipped != 1 || wrongRows.Skipped != 1 {
		t.Errorf("the outsider's guesses answered right=%+v wrong=%+v, want both skipped — the "+
			"visibility check must refuse before anything compares authors", rightRows, wrongRows)
	}
	// The REASON too, and identical for both. A future refusal on some other
	// ground would otherwise satisfy everything above while the audience check
	// was gone.
	rightReason, wrongReason := "", ""
	if rightRows.Rows[0].Reason != nil {
		rightReason = *rightRows.Rows[0].Reason
	}
	if wrongRows.Rows[0].Reason != nil {
		wrongReason = *wrongRows.Rows[0].Reason
	}
	if rightReason != wrongReason {
		t.Errorf("a correct guess was refused %q and a wrong one %q — the wording is the leak", rightReason, wrongReason)
	}
	if !strings.Contains(rightReason, "no such activity here") {
		t.Errorf("the refusal reads %q, want the not-here answer — a caller outside the audience "+
			"must not be able to tell a limited message from a missing one", rightReason)
	}

	// And nothing moved, whichever guess it was.
	if name, revision := authorOn(t, e, id); name == nil || *name != "Mutaz Suleiman" || revision == nil || *revision != 1 {
		t.Errorf("the outsider's guesses left author=%v revision=%v, want the seeded answer at revision 1", name, revision)
	}
}

// repairBody drives the route as a named seat and answers the decoded result,
// for the assertions that care what each ROW said rather than only the status.
func repairBody(as context.Context, t *testing.T, h attributionHandlers, body crmcontracts.SourceAttributionRequest) crmcontracts.SourceAttributionResult {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("encoding the batch: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/records/attribution", bytes.NewReader(raw)).WithContext(as)
	rec := httptest.NewRecorder()
	h.RepairSourceAttribution(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("the repair answered %d: %s", rec.Code, rec.Body.String())
	}
	var out crmcontracts.SourceAttributionResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decoding the result: %v", err)
	}
	return out
}

// TestTheNameBoundCountsCharactersNotBytes pins the contract's own unit.
//
// `maxLength: 200` counts CHARACTERS. Go's len() counts UTF-8 bytes, and the
// first version of this check used it — which refuses a name the contract
// permits, and refuses it by accent: an author called Renée or مطاز costs more
// bytes than characters, so a name at the limit carrying one of them would 422
// the entire batch. On a German portal's correspondence that is not an edge
// case, it is most of the interesting names.
func TestTheNameBoundCountsCharactersNotBytes(t *testing.T) {
	e := integration.Setup(t)
	h := attributionHandlers{db: InstallationDB(e.Pool), activities: activities.NewStore(InstallationDB(e.Pool))}

	// 200 characters, 210 bytes: ten accented letters and 190 ASCII.
	atTheLimit := strings.Repeat("é", 10) + strings.Repeat("a", 190)
	if utf8.RuneCountInString(atTheLimit) != 200 || len(atTheLimit) <= 200 {
		t.Fatalf("the fixture is %d characters and %d bytes, want 200 characters and more than 200 bytes",
			utf8.RuneCountInString(atTheLimit), len(atTheLimit))
	}

	accepted := repair(t, e, h, crmcontracts.SourceAttributionRequest{
		BatchRef: "at-the-limit",
		Rows:     []crmcontracts.SourceAttributionRow{row(seedImportedActivity(t, e, "hubspot"), atTheLimit, 1)},
	})
	if accepted.Applied != 1 {
		t.Errorf("a 200-character name answered %+v, want it applied — the contract bounds "+
			"characters and this one is exactly at the bound", accepted)
	}

	// And one character past it is still refused, so the bound is enforced
	// rather than merely widened.
	tooLong := atTheLimit + "a"
	code := repairAs(e.Admin(), t, h, crmcontracts.SourceAttributionRequest{
		BatchRef: "past-the-limit",
		Rows:     []crmcontracts.SourceAttributionRow{row(seedImportedActivity(t, e, "hubspot"), tooLong, 1)},
	})
	if code != http.StatusUnprocessableEntity {
		t.Errorf("a 201-character name answered %d, want 422", code)
	}
}
