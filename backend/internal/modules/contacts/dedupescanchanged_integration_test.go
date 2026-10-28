// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// The nightly duplicate scan reads the rows changed since its watermark,
// again over the last day once it has caught up.

import (
	"context"
	"os"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (e *dedupeEnv) scanChanged(t *testing.T) DedupeScanResult {
	t.Helper()
	result, err := e.store.ScanChangedForDuplicates(e.asScan())
	if err != nil {
		t.Fatalf("ScanChangedForDuplicates: %v", err)
	}
	return result
}

// ageRows moves the updated_at of every row in the given tables three days
// back: records written before the last few passes. The tables' own
// updated_at triggers would stamp the update itself, so they are switched off
// for this one transaction.
func ageRows(t *testing.T, tables ...string) {
	t.Helper()
	ctx := context.Background()
	owner, err := pgx.Connect(ctx, os.Getenv("MARGINCE_TEST_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := owner.Close(ctx); err != nil {
			t.Errorf("closing the owner connection: %v", err)
		}
	}()
	if err := pgx.BeginFunc(ctx, owner, func(tx pgx.Tx) error {
		for _, table := range tables {
			trigger := pgx.Identifier{"trg_" + table + "_updated"}.Sanitize()
			name := pgx.Identifier{table}.Sanitize()
			for _, statement := range []string{
				`ALTER TABLE ` + name + ` DISABLE TRIGGER ` + trigger,
				`UPDATE ` + name + ` SET updated_at = updated_at - interval '3 days'`,
				`ALTER TABLE ` + name + ` ENABLE TRIGGER ` + trigger,
			} {
				if _, err := tx.Exec(ctx, statement); err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("age the rows: %v", err)
	}
}

// ageAllScannedRows ages every table the nightly pass reads, taken from its
// own stream list so a new stream is aged too.
func ageAllScannedRows(t *testing.T) {
	t.Helper()
	tables := make([]string, 0, len(dedupeScanStreams))
	for _, stream := range dedupeScanStreams {
		tables = append(tables, stream.table)
	}
	ageRows(t, tables...)
}

// A similar name files only with a second fact. A fact that arrives after both
// companies last changed is found by the next nightly pass. That pass reads the
// facts and the employment edges as well as the companies.
func TestTheNightlyScanFilesASimilarNameOnceASecondFactArrives(t *testing.T) {
	for name, share := range map[string]func(e *dedupeEnv, ctx context.Context, t *testing.T, brand, line ids.CompanyID){
		"a stated phone number": func(e *dedupeEnv, ctx context.Context, t *testing.T, brand, line ids.CompanyID) {
			e.statePhone(ctx, t, brand, "+49 89 1234567")
			e.statePhone(ctx, t, line, "0049 89 123 45 67")
		},
		"a contact employed at both": func(e *dedupeEnv, _ context.Context, t *testing.T, brand, line ids.CompanyID) {
			colleague := mustContact(t, e, "Greta Hoffmann")
			employ(t, e, colleague, brand)
			employ(t, e, colleague, line)
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := setupDedupe(t)
			ctx := e.as()
			brand := e.createNamedCompany(ctx, t, brandName)
			line := e.createNamedCompany(ctx, t, lineName)
			e.forgetFiledPairs(ctx, t)
			ageAllScannedRows(t)
			if got := e.scanChanged(t); got.Filed != 0 {
				t.Fatalf("the pass before any shared fact = %+v, want nothing filed", got)
			}
			ageAllScannedRows(t)

			share(e, ctx, t, brand, line)
			if got := e.scanChanged(t); got.Filed != 1 {
				t.Errorf("the pass after the shared fact = %+v, want the pair filed", got)
			}
		})
	}
}

func TestTheFirstNightlyScanReadsEverythingAndTheNextOnlyWhatChanged(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	seeded := e.seedPreRuleDuplicates(ctx, t)
	ageAllScannedRows(t)

	if got := e.scanChanged(t); got.Filed != 3 {
		t.Fatalf("the first nightly pass = %+v, want the 3 pairs filed", got)
	}
	e.forgetFiledPairs(ctx, t)
	if got := e.scanChanged(t); got.Examined != 0 || got.Filed != 0 {
		t.Errorf("a pass with nothing changed for days = %+v, want nothing examined", got)
	}

	description := "Consulting for the example industry"
	if _, err := e.store.UpdateCompany(ctx, seeded.consulting, UpdateCompanyInput{Description: &description}); err != nil {
		t.Fatalf("edit the company: %v", err)
	}
	if got := e.scanChanged(t); got.Filed != 1 {
		t.Errorf("a pass after one company changed = %+v, want its pair filed", got)
	}
	queued, _ := e.queuedPairs(ctx, t)
	if _, ok := queued[storedPair(seeded.consultingGmbH.UUID, seeded.consulting.UUID)]; !ok || len(queued) != 1 {
		t.Errorf("the queue after the edit = %v, want only the edited company's pair", queued)
	}
}

// A pass short of time stops before its next batch. It leaves the rest to the
// next run, so its job's deadline never cancels a batch.
func TestANightlyScanShortOfTimeLeavesTheRestToTheNextRun(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.seedPreRuleDuplicates(ctx, t)
	ageAllScannedRows(t)

	short, cancel := context.WithTimeout(e.asScan(), dedupeRunReserve/2)
	defer cancel()
	got, err := e.store.ScanChangedForDuplicates(short)
	if err != nil {
		t.Fatalf("a pass short of time: %v", err)
	}
	if !got.Stopped || got.Examined != 0 || got.Filed != 0 {
		t.Fatalf("a pass short of time = %+v, want it stopped before reading anything", got)
	}
	if got := e.scanChanged(t); got.Stopped || got.Filed != 3 {
		t.Errorf("the next pass = %+v, want the 3 pairs the stopped pass left", got)
	}
}

// A write's updated_at is its transaction's start, so it can commit behind
// a pass that has already read past that time.
func TestTheNightlyScanFindsAWriteThatCommittedBehindItsWatermark(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	seeded := e.seedPreRuleDuplicates(ctx, t)
	ageAllScannedRows(t)
	e.scanChanged(t)
	e.forgetFiledPairs(ctx, t)

	late, err := e.store.db.Pool().Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := late.Rollback(ctx); err != nil && err != pgx.ErrTxClosed {
			t.Errorf("rollback: %v", err)
		}
	}()
	if _, err := late.Exec(ctx, `UPDATE company SET description = 'late' WHERE id = $1`, seeded.consulting); err != nil {
		t.Fatal(err)
	}
	if got := e.scanChanged(t); got.Filed != 0 {
		t.Errorf("a pass beside the open write filed %d pairs, want none yet", got.Filed)
	}
	if err := late.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if got := e.scanChanged(t); got.Filed != 1 {
		t.Errorf("the pass after the late commit = %+v, want the edited company's pair filed", got)
	}
}

func TestTheNightlyScanStartsWhereTheFullPassBegan(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.seedPreRuleDuplicates(ctx, t)
	ageAllScannedRows(t)
	if filed, _ := e.scanEverything(t, 100); filed != 3 {
		t.Fatalf("the full pass filed %d pairs, want 3", filed)
	}
	e.forgetFiledPairs(ctx, t)
	if got := e.scanChanged(t); got.Examined != 0 {
		t.Errorf("the first nightly pass after the full pass = %+v, want the records it covered left alone", got)
	}
}

// A merge moves the retired contact's number onto the survivor and leaves the
// survivor's own row as it was. The moved row still names the survivor.
func TestTheNightlyScanFilesAPairAMergeMadeByMovingANumber(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	shared := "+49 30 1234567"
	holder := e.seedContact(ctx, t, "Ola Berg", []string{"ola@first.example"}, []string{shared})
	survivor := e.seedContact(ctx, t, "Kim Lund", []string{"kim@second.example"}, nil)
	retired := e.seedContact(ctx, t, "Kim Lund", []string{"kim@third.example"}, []string{shared})
	ageAllScannedRows(t)
	e.scanChanged(t)
	if _, err := e.store.MergeContact(ctx, retired, survivor, nil); err != nil {
		t.Fatalf("merge: %v", err)
	}
	e.forgetFiledPairs(ctx, t)
	ageRows(t, "company", "company_domain", "contact")

	if got := e.scanChanged(t); got.Filed != 1 {
		t.Errorf("the pass after the merge = %+v, want the shared-number pair filed", got)
	}
	queued, _ := e.queuedPairs(ctx, t)
	if _, ok := queued[storedPair(holder.UUID, survivor.UUID)]; !ok {
		t.Errorf("the queue = %v, want the survivor paired with the other holder of the number", queued)
	}
}

func TestTheDuplicateScanFilesASharedNumber(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	shared := "+49 30 7654321"
	first := e.seedContact(ctx, t, "Ola Berg", []string{"ola@one.example"}, []string{shared})
	second := e.seedContact(ctx, t, "Kim Lund", []string{"kim@two.example"}, []string{shared})
	e.forgetFiledPairs(ctx, t)

	if filed, _ := e.scanEverything(t, 100); filed != 1 {
		t.Errorf("the full pass filed %d pairs, want the shared-number pair", filed)
	}
	queued, _ := e.queuedPairs(ctx, t)
	if source, ok := queued[storedPair(first.UUID, second.UUID)]; !ok || source != dedupeScanSource {
		t.Errorf("the queue = %v, want the two holders of the number filed by the scan", queued)
	}
}

func TestANameManyCompaniesShareFilesAtMostTheRivalLimit(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	for range 20 {
		before, _ := e.queuedPairs(ctx, t)
		e.createNamedCompany(ctx, t, "Acme Widgets")
		if after, _ := e.queuedPairs(ctx, t); len(after)-len(before) > dedupeRivalLimit {
			t.Fatalf("one create filed %d pairs, want at most %d", len(after)-len(before), dedupeRivalLimit)
		}
	}
	e.forgetFiledPairs(ctx, t)

	system := e.asScan()
	after := ids.Nil
	for {
		var page DedupeScanPage
		if err := e.store.tx(system, func(tx pgx.Tx) error {
			var err error
			page, err = ScanForDuplicatesTx(system, tx, nil, DedupeScanCompanies, after, 1)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if page.Filed > dedupeRivalLimit {
			t.Errorf("scanning one company filed %d pairs, want at most %d", page.Filed, dedupeRivalLimit)
		}
		if len(page.Scanned) == 0 {
			break
		}
		after = page.Scanned[0]
	}
}

// The rival cap bounds new filings. Five dismissed look-alikes must not hide
// a sixth rival behind them, for either record type.
func TestADismissedLookAlikeDoesNotTakeARivalsPlace(t *testing.T) {
	for entity, create := range map[DedupeScanEntity]func(*dedupeEnv, context.Context, *testing.T, int) ids.UUID{
		DedupeScanCompanies: func(e *dedupeEnv, ctx context.Context, t *testing.T, _ int) ids.UUID {
			return e.createNamedCompany(ctx, t, "Acme Widgets").UUID
		},
		DedupeScanContacts: func(e *dedupeEnv, ctx context.Context, t *testing.T, i int) ids.UUID {
			return e.plainContact(ctx, t, "Mia Schulz", "mia"+strconv.Itoa(i)+"@lookalike.example")
		},
	} {
		t.Run(string(entity), func(t *testing.T) {
			e := setupDedupe(t)
			ctx := e.as()
			subject := create(e, ctx, t, 0)
			for i := 1; i < 7; i++ {
				create(e, ctx, t, i)
			}
			e.forgetFiledPairs(ctx, t)
			if filed := e.scanOne(t, entity, subject); filed != dedupeRivalLimit {
				t.Fatalf("the first scan filed %d pairs, want %d", filed, dedupeRivalLimit)
			}
			for _, row := range openCandidates(ctx, t, e, string(entity)) {
				if _, err := e.store.DisposeDedupeCandidate(ctx, row.ID, "not_a_duplicate", nil); err != nil {
					t.Fatalf("dismiss: %v", err)
				}
			}
			if filed := e.scanOne(t, entity, subject); filed != 1 {
				t.Errorf("the scan after five dismissals filed %d pairs, want the sixth rival", filed)
			}
		})
	}
}

// scanOne runs one record through the scan's re-check and answers the pairs
// it filed.
func (e *dedupeEnv) scanOne(t *testing.T, entity DedupeScanEntity, id ids.UUID) int {
	t.Helper()
	system := e.asScan()
	var filed int
	if err := e.store.tx(system, func(tx pgx.Tx) error {
		scanner, err := newDedupeScanner(system, tx, entity, nil)
		if err != nil {
			return err
		}
		filed, err = scanner.file(system, tx, id)
		return err
	}); err != nil {
		t.Fatalf("scan one %s: %v", entity, err)
	}
	return filed
}
