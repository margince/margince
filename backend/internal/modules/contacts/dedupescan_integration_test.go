// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// The duplicate scan over records that are already stored. Every fixture is
// written through the real create path, and then the pairs those creates
// filed are removed. That is the state of records written before a lane
// existed, or written together by an import.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// preRuleDuplicates are three pairs the filing rule names and two records
// that pair with nothing.
type preRuleDuplicates struct {
	consultingGmbH, consulting ids.CompanyID
	parent, subdomain          ids.CompanyID
	bjoern, bjorn              ids.UUID
}

func (e *dedupeEnv) asScan() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.SystemActing(ctx, "dedupe_scan")
}

func (e *dedupeEnv) seedPreRuleDuplicates(ctx context.Context, t *testing.T) preRuleDuplicates {
	t.Helper()
	seeded := preRuleDuplicates{
		consultingGmbH: e.createNamedCompany(ctx, t, "Example Consulting GmbH", "consulting-one.example"),
		consulting:     e.createNamedCompany(ctx, t, "Example Consulting", "consulting-two.example"),
		parent:         e.createNamedCompany(ctx, t, "Contoso Holdings", "contoso.example"),
		// The subdomain pair shares no name, so only the domain lane finds it.
		subdomain: e.createNamedCompany(ctx, t, "Blue Yonder Labs", "labs.contoso.example"),
		bjoern:    e.plainContact(ctx, t, "Bjoern Example", "bjoern@first.example"),
		bjorn:     e.plainContact(ctx, t, "Björn Example", "bjorn@second.example"),
	}
	e.createNamedCompany(ctx, t, "Northwind Traders", "northwind.example")
	e.plainContact(ctx, t, "Alex Meyer", "alex@third.example")
	e.forgetFiledPairs(ctx, t)
	return seeded
}

// forgetFiledPairs removes every queue row. Their audit rows stay, as
// audit_log is append-only.
func (e *dedupeEnv) forgetFiledPairs(ctx context.Context, t *testing.T) {
	t.Helper()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM dedupe_candidate`)
		return err
	}); err != nil {
		t.Fatalf("forget the filed pairs: %v", err)
	}
}

// scanEverything runs the data task's batches for both record types from a
// nil cursor, limit records at a time. It answers the pairs filed and the
// batches it took.
func (e *dedupeEnv) scanEverything(t *testing.T, limit int) (filed, batches int) {
	t.Helper()
	ctx := e.asScan()
	for _, entity := range []DedupeScanEntity{DedupeScanCompanies, DedupeScanContacts} {
		after := ids.Nil
		for {
			var page DedupeScanPage
			if err := e.store.tx(ctx, func(tx pgx.Tx) error {
				var err error
				page, err = ScanForDuplicatesTx(ctx, tx, nil, entity, after, limit)
				return err
			}); err != nil {
				t.Fatalf("scan %s after %s: %v", entity, after, err)
			}
			batches++
			filed += page.Filed
			if len(page.Scanned) > limit {
				t.Fatalf("a batch of %d inspected %d records", limit, len(page.Scanned))
			}
			if len(page.Scanned) < limit {
				break
			}
			after = page.Scanned[len(page.Scanned)-1]
		}
	}
	return filed, batches
}

// queuedPairs answers the pairs on the queue in any disposition, keyed by
// their two ids in stored order. It also answers the queue's audit row count.
func (e *dedupeEnv) queuedPairs(ctx context.Context, t *testing.T) (map[[2]ids.UUID]string, int) {
	t.Helper()
	pairs := map[[2]ids.UUID]string{}
	var audits int
	err := e.store.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT coalesce(left_contact_id, left_company_id), coalesce(right_contact_id, right_company_id), source
			  FROM dedupe_candidate`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var left, right ids.UUID
			var source string
			if err := rows.Scan(&left, &right, &source); err != nil {
				return err
			}
			pairs[[2]ids.UUID{left, right}] = source
		}
		if err := rows.Err(); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE entity_type = 'dedupe_candidate'`).Scan(&audits)
	})
	if err != nil {
		t.Fatalf("read the queue: %v", err)
	}
	return pairs, audits
}

func storedPair(a, b ids.UUID) [2]ids.UUID {
	if b.String() < a.String() {
		return [2]ids.UUID{b, a}
	}
	return [2]ids.UUID{a, b}
}

func (d preRuleDuplicates) pairs() [][2]ids.UUID {
	return [][2]ids.UUID{
		storedPair(d.consultingGmbH.UUID, d.consulting.UUID),
		storedPair(d.parent.UUID, d.subdomain.UUID),
		storedPair(d.bjoern, d.bjorn),
	}
}

func TestTheDuplicateScanFilesEveryPairWrittenBeforeTheRuleOnce(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	seeded := e.seedPreRuleDuplicates(ctx, t)

	// One record per batch over five companies and three contacts, so each
	// type ends on an empty batch.
	filed, batches := e.scanEverything(t, 1)
	if filed != 3 {
		t.Errorf("the scan filed %d pairs, want 3", filed)
	}
	if batches != 10 {
		t.Errorf("the scan took %d batches of one, want 10 (five companies and three contacts, each type ending on an empty batch)", batches)
	}
	queued, audits := e.queuedPairs(ctx, t)
	for _, pair := range seeded.pairs() {
		if source := queued[pair]; source != dedupeScanSource {
			t.Errorf("pair %v is on the queue with source %q, want %q", pair, source, dedupeScanSource)
		}
	}
	if len(queued) != 3 {
		t.Errorf("the queue holds %d pairs, want 3: %v", len(queued), queued)
	}

	if again, _ := e.scanEverything(t, 100); again != 0 {
		t.Errorf("a second scan filed %d pairs, want none", again)
	}
	if _, after := e.queuedPairs(ctx, t); after != audits {
		t.Errorf("a second scan wrote audit rows: %d -> %d", audits, after)
	}
}

func TestTheDuplicateScanNeverFilesADecidedPairAgain(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	seeded := e.seedPreRuleDuplicates(ctx, t)
	e.scanEverything(t, 100)

	for _, row := range openCandidates(ctx, t, e, "") {
		switch storedPair(row.LeftID, row.RightID) {
		case storedPair(seeded.consultingGmbH.UUID, seeded.consulting.UUID):
			if _, err := e.store.DisposeDedupeCandidate(ctx, row.ID, "not_a_duplicate", nil); err != nil {
				t.Fatalf("dismiss: %v", err)
			}
		case storedPair(seeded.bjoern, seeded.bjorn):
			winner := seeded.bjoern
			if _, err := e.store.DisposeDedupeCandidate(ctx, row.ID, "merge", &winner); err != nil {
				t.Fatalf("merge: %v", err)
			}
		}
	}
	_, audits := e.queuedPairs(ctx, t)
	if filed, _ := e.scanEverything(t, 100); filed != 0 {
		t.Errorf("the scan filed %d pairs after one was dismissed and one merged, want none", filed)
	}
	if _, after := e.queuedPairs(ctx, t); after != audits {
		t.Errorf("the scan wrote audit rows on decided pairs: %d -> %d", audits, after)
	}

	// With the queue gone the merged contact is archived and pairs with
	// nobody, while the dismissed companies are filed afresh.
	e.forgetFiledPairs(ctx, t)
	if filed, _ := e.scanEverything(t, 100); filed != 2 {
		t.Errorf("with no queue the scan filed %d pairs, want the two whose records are live", filed)
	}
}

func TestTheDuplicateScanSkipsArchivedRecordsAndTheAnchor(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	seeded := e.seedPreRuleDuplicates(ctx, t)
	system := e.asScan()
	if err := e.store.tx(system, func(tx pgx.Tx) error {
		if err := e.store.ArchiveContactTx(system, tx, ids.From[ids.ContactKind](seeded.bjorn), nil); err != nil {
			return err
		}
		_, err := tx.Exec(system, `UPDATE company SET is_anchor = true WHERE id = $1`, seeded.consultingGmbH)
		return err
	}); err != nil {
		t.Fatalf("archive and anchor: %v", err)
	}
	e.forgetFiledPairs(ctx, t)

	if filed, _ := e.scanEverything(t, 100); filed != 1 {
		t.Errorf("the scan filed %d pairs, want only the subdomain pair", filed)
	}
	queued, _ := e.queuedPairs(ctx, t)
	if _, ok := queued[storedPair(seeded.parent.UUID, seeded.subdomain.UUID)]; !ok {
		t.Errorf("the subdomain pair is missing from the queue: %v", queued)
	}
	for entity, want := range map[DedupeScanEntity]int64{DedupeScanCompanies: 4, DedupeScanContacts: 2} {
		if got, err := CountDedupeScanSubjects(system, e.store.db.Pool(), entity, 100); err != nil || got != want {
			t.Errorf("the scan counts %d %s subjects (err %v), want %d without the anchor and the archived", got, entity, err, want)
		}
	}

	// A record archived or made the anchor after its batch was paged is still
	// no subject.
	e.forgetFiledPairs(ctx, t)
	for entity, id := range map[DedupeScanEntity]ids.UUID{DedupeScanCompanies: seeded.consultingGmbH.UUID, DedupeScanContacts: seeded.bjorn} {
		if err := e.store.tx(system, func(tx pgx.Tx) error {
			scanner, err := newDedupeScanner(system, tx, entity, nil)
			if err != nil {
				return err
			}
			filed, err := scanner.file(system, tx, id)
			if err == nil && filed != 0 {
				t.Errorf("the %s scan filed %d pairs for a record that is no subject", entity, filed)
			}
			return err
		}); err != nil {
			t.Fatalf("scan one %s: %v", entity, err)
		}
	}
}

// dbClockPlus answers the database's clock moved by d. The nightly pass
// compares stored updated_at stamps, which the database writes, so the test
// clock starts from the same source.
func (e *dedupeEnv) dbClockPlus(ctx context.Context, t *testing.T, d time.Duration) time.Time {
	t.Helper()
	var now time.Time
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now)
	}); err != nil {
		t.Fatalf("read the database clock: %v", err)
	}
	return now.Add(d)
}

func (e *dedupeEnv) scanChanged(t *testing.T) DedupeScanResult {
	t.Helper()
	result, err := e.store.ScanChangedForDuplicates(e.asScan())
	if err != nil {
		t.Fatalf("ScanChangedForDuplicates: %v", err)
	}
	return result
}

func TestTheNightlyScanRescansOnlyRecordsChangedSinceItsWatermark(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	seeded := e.seedPreRuleDuplicates(ctx, t)
	later := e.dbClockPlus(ctx, t, time.Hour)
	e.store.WithClock(func() time.Time { return later })

	if got := e.scanChanged(t); got.Filed != 3 || got.Examined != 8 {
		t.Fatalf("the first nightly pass = %+v, want 8 records examined and 3 pairs filed", got)
	}
	e.forgetFiledPairs(ctx, t)
	if got := e.scanChanged(t); got.Examined != 0 || got.Filed != 0 {
		t.Errorf("a pass with nothing changed = %+v, want nothing examined", got)
	}

	description := "Consulting for the example industry"
	if _, err := e.store.UpdateCompany(ctx, seeded.consulting, UpdateCompanyInput{Description: &description}); err != nil {
		t.Fatalf("edit the company: %v", err)
	}
	later = e.dbClockPlus(ctx, t, time.Hour)
	if got := e.scanChanged(t); got.Examined != 1 || got.Filed != 1 {
		t.Errorf("a pass after one company changed = %+v, want that company examined and its pair filed", got)
	}
	queued, _ := e.queuedPairs(ctx, t)
	if _, ok := queued[storedPair(seeded.consultingGmbH.UUID, seeded.consulting.UUID)]; !ok || len(queued) != 1 {
		t.Errorf("the queue after the edit = %v, want only the edited company's pair", queued)
	}
}

func TestTheNightlyScanLeavesChangesInsideTheSettleWindowForTheNextPass(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.seedPreRuleDuplicates(ctx, t)
	now := e.dbClockPlus(ctx, t, 0)
	e.store.WithClock(func() time.Time { return now })

	if got := e.scanChanged(t); got.Examined != 0 {
		t.Errorf("a pass run as the records were written examined %d, want them left for the next pass", got.Examined)
	}
	now = now.Add(time.Hour)
	if got := e.scanChanged(t); got.Filed != 3 {
		t.Errorf("the next pass = %+v, want the 3 pairs filed", got)
	}
}
