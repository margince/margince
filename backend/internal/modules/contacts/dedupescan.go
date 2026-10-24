// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The duplicate scan runs the re-checks a rename runs (renamerecheck.go,
// contactrenamerecheck.go) over records that already exist, so a pair the
// filing rule names reaches the queue whenever its two records were written.
// The data task walks every record once; the nightly pass walks the records
// changed since its watermark. A pair the queue already holds, decided or not,
// meets its own row on uq_dedupe_candidate_pair, and nothing is filed again.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/freemail"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// dedupeScanSource names the scan on the queue rows it files.
const dedupeScanSource = "dedupe_scan"

// DedupeScanEntity is a record type the duplicate scan walks. Leads are not
// scanned: their filer (fuzzyLead) runs on every lead write and has no rename.
type DedupeScanEntity string

// The record types the duplicate scan walks.
const (
	DedupeScanCompanies DedupeScanEntity = entityCompany
	DedupeScanContacts  DedupeScanEntity = entityContact
)

// subjectsSQL is the FROM and WHERE that select the records of this type the
// scan may file a pair for. The anchor company is no subject, for the reason
// recheckCompanyForDuplicates gives.
func (e DedupeScanEntity) subjectsSQL() (string, error) {
	switch e {
	case DedupeScanCompanies:
		return `company WHERE archived_at IS NULL AND NOT is_anchor`, nil
	case DedupeScanContacts:
		return `contact WHERE archived_at IS NULL`, nil
	}
	return "", fmt.Errorf("contacts: the duplicate scan has no record type %q", string(e))
}

// DedupeScanPage is one batch of the scan: the records it inspected, in the
// order it read them, and how many pairs are new on the queue.
type DedupeScanPage struct {
	Scanned []ids.UUID
	Filed   int
}

// dedupeScanner files the pairs one record makes, with the evidence and the
// actor of one pass.
type dedupeScanner struct {
	entity       DedupeScanEntity
	consumerMail *freemail.Matcher
	by           string
}

func newDedupeScanner(ctx context.Context, tx pgx.Tx, entity DedupeScanEntity, consumerMail ConsumerMailReader) (dedupeScanner, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return dedupeScanner{}, err
	}
	if err := auth.Require(ctx, string(entity), principal.ActionRead); err != nil {
		return dedupeScanner{}, err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return dedupeScanner{}, err
	}
	scanner := dedupeScanner{entity: entity, by: by}
	if consumerMail != nil && entity == DedupeScanContacts {
		if scanner.consumerMail, err = consumerMail(ctx, tx); err != nil {
			return dedupeScanner{}, err
		}
	}
	return scanner, nil
}

// file runs one record through its re-check.
//
// The contact half takes no lane lock. Those locks stop a create and an edit
// from both reading before the other committed; the scan writes no name and no
// address, so a create running beside it reads the scanned record as it was
// committed and files the pair itself. Locking lanes for many records in one
// transaction would only add lock orders a create can deadlock against.
func (s dedupeScanner) file(ctx context.Context, tx pgx.Tx, id ids.UUID) (int, error) {
	if s.entity == DedupeScanCompanies {
		return recheckCompanyForDuplicates(ctx, tx, ids.From[ids.CompanyKind](id), dedupeScanSource, s.by)
	}
	subject, found, err := readContactSubject(ctx, tx, ids.From[ids.ContactKind](id))
	if err != nil || !found {
		return 0, err
	}
	return subject.fileDuplicates(ctx, tx, s.consumerMail, dedupeScanSource, s.by)
}

// fileEach files the pairs of every record in order and adds them to page.
func (s dedupeScanner) fileEach(ctx context.Context, tx pgx.Tx, page *DedupeScanPage) error {
	for _, id := range page.Scanned {
		filed, err := s.file(ctx, tx, id)
		if err != nil {
			return err
		}
		page.Filed += filed
	}
	return nil
}

// ScanForDuplicatesTx files the pairs of the live records of one type after
// the cursor, at most limit of them in id order. It is the data task's batch:
// a replay from a nil cursor files nothing the first pass filed.
func ScanForDuplicatesTx(ctx context.Context, tx pgx.Tx, consumerMail ConsumerMailReader, entity DedupeScanEntity,
	after ids.UUID, limit int,
) (DedupeScanPage, error) {
	scanner, err := newDedupeScanner(ctx, tx, entity, consumerMail)
	if err != nil {
		return DedupeScanPage{}, err
	}
	subjects, err := entity.subjectsSQL()
	if err != nil {
		return DedupeScanPage{}, err
	}
	rows, err := tx.Query(ctx, `SELECT id FROM `+subjects+` AND id > $1 ORDER BY id LIMIT $2`, after, limit)
	if err != nil {
		return DedupeScanPage{}, fmt.Errorf("contacts: paging records for the duplicate scan: %w", err)
	}
	scanned, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return DedupeScanPage{}, fmt.Errorf("contacts: paging records for the duplicate scan: %w", err)
	}
	page := DedupeScanPage{Scanned: scanned}
	return page, scanner.fileEach(ctx, tx, &page)
}

// CountDedupeScanSubjects counts, up to limit+1, the records
// ScanForDuplicatesTx would inspect, so an estimate never reads past its cap.
func CountDedupeScanSubjects(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, entity DedupeScanEntity, limit int64,
) (int64, error) {
	if err := auth.Require(ctx, string(entity), principal.ActionRead); err != nil {
		return 0, err
	}
	subjects, err := entity.subjectsSQL()
	if err != nil {
		return 0, err
	}
	var count int64
	if err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 FROM `+subjects+` LIMIT $1) subjects`,
		limit+1).Scan(&count); err != nil {
		return 0, fmt.Errorf("contacts: counting records for the duplicate scan: %w", err)
	}
	return count, nil
}

// The nightly pass's bounds. A batch is one transaction, so the company-name
// lock the company re-check takes is held for one batch at a time. A run stops
// after dedupeScanPerRun records of each type and goes on from its watermark
// the next night.
const (
	dedupeScanBatch  = 200
	dedupeScanPerRun = 5000
)

// dedupeScanSettle keeps the newest changes for the next pass. updated_at is
// stamped when a write's transaction starts, so a write still open when the
// pass reads could commit behind the watermark; one that started before the
// settle window has had that long to commit.
const dedupeScanSettle = 10 * time.Minute

// dedupeScanFirstLookback is how far back a pass with no watermark starts. The
// data task scans everything older once; this covers the days between the
// upgrade and the first nightly run.
const dedupeScanFirstLookback = 48 * time.Hour

// DedupeScanResult counts one nightly pass: the records it inspected and the
// pairs new on the queue.
type DedupeScanResult struct {
	Examined int
	Filed    int
}

// dedupeWatermark is the last record a nightly pass inspected, in the order
// it reads them: by updated_at, then id.
type dedupeWatermark struct {
	at time.Time
	id ids.UUID
}

// ScanChangedForDuplicates files the pairs of the companies and contacts
// changed since the last pass, up to dedupeScanPerRun of each.
func (s *Store) ScanChangedForDuplicates(ctx context.Context) (DedupeScanResult, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return DedupeScanResult{}, err
	}
	cutoff := s.clock().Add(-dedupeScanSettle)
	var result DedupeScanResult
	for _, entity := range []DedupeScanEntity{DedupeScanCompanies, DedupeScanContacts} {
		for examined := 0; examined < dedupeScanPerRun; {
			page, err := s.scanChangedBatch(ctx, entity, cutoff)
			if err != nil {
				return result, fmt.Errorf("contacts: scan changed %s records for duplicates: %w", entity, err)
			}
			examined += len(page.Scanned)
			result.Examined += len(page.Scanned)
			result.Filed += page.Filed
			if len(page.Scanned) < dedupeScanBatch {
				break
			}
		}
	}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		_, err := storekit.LogSystem(ctx, tx, dedupeScanSource, map[string]any{
			"examined": result.Examined, "filed": result.Filed,
		})
		return err
	})
	return result, err
}

// scanChangedBatch files the pairs of the next batch of changed records after
// the watermark and moves the watermark past them, in one transaction. The
// watermark row is locked, so two passes running at once take turns.
func (s *Store) scanChangedBatch(ctx context.Context, entity DedupeScanEntity, cutoff time.Time) (DedupeScanPage, error) {
	var page DedupeScanPage
	err := s.txRetryingLockCycles(ctx, func(tx pgx.Tx) error {
		page = DedupeScanPage{}
		scanner, err := newDedupeScanner(ctx, tx, entity, s.consumerMail)
		if err != nil {
			return err
		}
		mark, err := lockDedupeWatermark(ctx, tx, entity, cutoff.Add(-dedupeScanFirstLookback))
		if err != nil {
			return err
		}
		changed, err := changedSince(ctx, tx, entity, mark, cutoff)
		if err != nil || len(changed) == 0 {
			return err
		}
		for _, row := range changed {
			page.Scanned = append(page.Scanned, row.id)
		}
		if err := scanner.fileEach(ctx, tx, &page); err != nil {
			return err
		}
		return moveDedupeWatermark(ctx, tx, entity, changed[len(changed)-1])
	})
	return page, err
}

// lockDedupeWatermark reads and locks the watermark of one record type,
// starting it at first when no pass has run.
func lockDedupeWatermark(ctx context.Context, tx pgx.Tx, entity DedupeScanEntity, first time.Time) (dedupeWatermark, error) {
	if _, err := tx.Exec(ctx, `
		INSERT INTO dedupe_scan_state (entity_type, scanned_through_at, scanned_through_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (entity_type) DO NOTHING`, string(entity), first, ids.Nil); err != nil {
		return dedupeWatermark{}, fmt.Errorf("start the duplicate scan watermark: %w", err)
	}
	var mark dedupeWatermark
	if err := tx.QueryRow(ctx, `
		SELECT scanned_through_at, scanned_through_id FROM dedupe_scan_state
		 WHERE entity_type = $1 FOR UPDATE`, string(entity)).Scan(&mark.at, &mark.id); err != nil {
		return dedupeWatermark{}, fmt.Errorf("read the duplicate scan watermark: %w", err)
	}
	return mark, nil
}

// changedSince reads the next batch of live records changed after the
// watermark and before the cutoff. The bare `updated_at >=` arm lets the
// updated_at index serve the row comparison behind it.
func changedSince(ctx context.Context, tx pgx.Tx, entity DedupeScanEntity, mark dedupeWatermark, cutoff time.Time) ([]dedupeWatermark, error) {
	subjects, err := entity.subjectsSQL()
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT updated_at, id FROM `+subjects+`
		   AND updated_at >= $1 AND (updated_at, id) > ($1, $2) AND updated_at < $3
		 ORDER BY updated_at, id
		 LIMIT $4`, mark.at, mark.id, cutoff, dedupeScanBatch)
	if err != nil {
		return nil, fmt.Errorf("read records changed since the duplicate scan: %w", err)
	}
	changed, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (dedupeWatermark, error) {
		var mark dedupeWatermark
		err := row.Scan(&mark.at, &mark.id)
		return mark, err
	})
	if err != nil {
		return nil, fmt.Errorf("read records changed since the duplicate scan: %w", err)
	}
	return changed, nil
}

func moveDedupeWatermark(ctx context.Context, tx pgx.Tx, entity DedupeScanEntity, last dedupeWatermark) error {
	if _, err := tx.Exec(ctx, `
		UPDATE dedupe_scan_state SET scanned_through_at = $2, scanned_through_id = $3
		 WHERE entity_type = $1`, string(entity), last.at, last.id); err != nil {
		return fmt.Errorf("move the duplicate scan watermark: %w", err)
	}
	return nil
}
