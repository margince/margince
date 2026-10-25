// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The nightly duplicate scan runs the records changed since the last pass
// through the re-checks the data task runs (dedupescan.go). It reads five
// tables from their own watermarks. The identifier tables catch an address, a
// number or a domain a merge moved without touching the record row. Every
// insert and update stamps updated_at, so no writer has to tell the scan.

// updated_at is the writing transaction's start time, so a write can commit
// behind a watermark that has passed it. A caught-up pass therefore sets its
// watermark dedupeScanOverlap back, and the next pass reads that window again.
// Filing a pair the queue holds is a no-op, so the overlap costs reads only.

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The nightly pass's bounds. A batch is one transaction, so the company re-check
// holds the company-name lock for one batch at a time. A run stops after
// dedupeScanPerRun rows of each table, and the next night goes on from there.
const (
	dedupeScanBatch  = 200
	dedupeScanPerRun = 5000
)

// dedupeScanOverlap is how far back a caught-up pass sets its watermark. A
// write that stays open longer than this and commits behind the watermark is
// only found by the next change to its record.
const dedupeScanOverlap = 24 * time.Hour

// dedupeScanStream is one table the nightly pass reads, and the column that
// names the record a changed row belongs to.
type dedupeScanStream struct {
	table  string
	entity DedupeScanEntity
	// liveOnly reads only rows that are not archived. An archived record has
	// nothing left to pair. An identifier row is read either way, because its
	// record may still be live.
	liveOnly bool
}

// subject is the column naming the record a row belongs to: the record table's
// own id, or the identifier table's company_id or contact_id.
func (stream dedupeScanStream) subject() string {
	if stream.table == string(stream.entity) {
		return "id"
	}
	return string(stream.entity) + "_id"
}

// dedupeScanStreams are the tables whose changes can make a new pair.
var dedupeScanStreams = []dedupeScanStream{
	{table: entityCompany, entity: DedupeScanCompanies, liveOnly: true},
	{table: tableCompanyDomain, entity: DedupeScanCompanies},
	{table: entityContact, entity: DedupeScanContacts, liveOnly: true},
	{table: tableContactEmail, entity: DedupeScanContacts},
	{table: tableContactPhone, entity: DedupeScanContacts},
}

// DedupeScanResult counts one nightly pass: the records it ran through the
// re-check and the pairs new on the queue.
type DedupeScanResult struct {
	Examined int
	Filed    int
}

// dedupeWatermark is a position in one table, in the order the pass reads
// it: by updated_at, then id.
type dedupeWatermark struct {
	at time.Time
	id ids.UUID
}

// changedRow is one changed row and the record it belongs to.
type changedRow struct {
	mark    dedupeWatermark
	subject ids.UUID
}

// ScanChangedForDuplicates files the pairs of the companies and contacts
// changed since the last pass, up to dedupeScanPerRun rows of each table. The
// first pass with no watermark reads every table from the start.
func (s *Store) ScanChangedForDuplicates(ctx context.Context) (DedupeScanResult, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return DedupeScanResult{}, err
	}
	// The database's clock, because the stamps this compares with are its own.
	var cutoff time.Time
	if err := s.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&cutoff)
	}); err != nil {
		return DedupeScanResult{}, fmt.Errorf("contacts: read the database clock: %w", err)
	}
	var result DedupeScanResult
	for _, stream := range dedupeScanStreams {
		for read := 0; read < dedupeScanPerRun; {
			rows, page, err := s.scanChangedBatch(ctx, stream, cutoff)
			if err != nil {
				return result, fmt.Errorf("contacts: scan changed %s rows for duplicates: %w", stream.table, err)
			}
			read += rows
			result.Examined += len(page.Scanned)
			result.Filed += page.Filed
			if rows < dedupeScanBatch {
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

// scanChangedBatch files the pairs of the records behind the next batch of
// changed rows and moves the watermark, in one transaction. It answers the rows
// it read. The watermark row is locked, so two passes running at once take
// turns.
func (s *Store) scanChangedBatch(ctx context.Context, stream dedupeScanStream, cutoff time.Time) (int, DedupeScanPage, error) {
	var page DedupeScanPage
	var rows int
	err := s.txRetryingLockCycles(ctx, func(tx pgx.Tx) error {
		page, rows = DedupeScanPage{}, 0
		scanner, err := newDedupeScanner(ctx, tx, stream.entity, s.consumerMail)
		if err != nil {
			return err
		}
		mark, err := lockDedupeWatermark(ctx, tx, stream.table)
		if err != nil {
			return err
		}
		changed, err := stream.changedSince(ctx, tx, mark, cutoff)
		if err != nil {
			return err
		}
		rows = len(changed)
		for _, row := range changed {
			if !slices.Contains(page.Scanned, row.subject) {
				page.Scanned = append(page.Scanned, row.subject)
			}
		}
		if err := scanner.fileEach(ctx, tx, &page); err != nil {
			return err
		}
		next := dedupeWatermark{at: cutoff.Add(-dedupeScanOverlap), id: ids.Nil}
		if rows == dedupeScanBatch {
			next = changed[rows-1].mark
		}
		return moveDedupeWatermark(ctx, tx, stream.table, next)
	})
	return rows, page, err
}

// lockDedupeWatermark reads and locks the watermark of one table, starting
// it before every row when no pass has run.
func lockDedupeWatermark(ctx context.Context, tx pgx.Tx, stream string) (dedupeWatermark, error) {
	if _, err := tx.Exec(ctx, `
		INSERT INTO dedupe_scan_state (stream, scanned_through_at, scanned_through_id)
		VALUES ($1, '0001-01-01 00:00:00+00', $2)
		ON CONFLICT (stream) DO NOTHING`, stream, ids.Nil); err != nil {
		return dedupeWatermark{}, fmt.Errorf("start the duplicate scan watermark: %w", err)
	}
	var mark dedupeWatermark
	if err := tx.QueryRow(ctx, `
		SELECT scanned_through_at, scanned_through_id FROM dedupe_scan_state
		 WHERE stream = $1 FOR UPDATE`, stream).Scan(&mark.at, &mark.id); err != nil {
		return dedupeWatermark{}, fmt.Errorf("read the duplicate scan watermark: %w", err)
	}
	return mark, nil
}

// changedSince reads the next batch of rows changed after the watermark and
// before the cutoff, over the stream's (updated_at, id) index. The bare
// `updated_at >=` arm gives the index a range to start from.
func (stream dedupeScanStream) changedSince(ctx context.Context, tx pgx.Tx, mark dedupeWatermark, cutoff time.Time) ([]changedRow, error) {
	live := ""
	if stream.liveOnly {
		live = "archived_at IS NULL AND "
	}
	rows, err := tx.Query(ctx, `
		SELECT updated_at, id, `+stream.subject()+` FROM `+stream.table+`
		 WHERE `+live+`updated_at >= $1 AND (updated_at, id) > ($1, $2) AND updated_at < $3
		 ORDER BY updated_at, id
		 LIMIT `+strconv.Itoa(dedupeScanBatch), mark.at, mark.id, cutoff)
	if err != nil {
		return nil, fmt.Errorf("read rows changed since the duplicate scan: %w", err)
	}
	changed, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (changedRow, error) {
		var r changedRow
		err := row.Scan(&r.mark.at, &r.mark.id, &r.subject)
		return r, err
	})
	if err != nil {
		return nil, fmt.Errorf("read rows changed since the duplicate scan: %w", err)
	}
	return changed, nil
}

func moveDedupeWatermark(ctx context.Context, tx pgx.Tx, stream string, next dedupeWatermark) error {
	if _, err := tx.Exec(ctx, `
		UPDATE dedupe_scan_state SET scanned_through_at = $2, scanned_through_id = $3
		 WHERE stream = $1`, stream, next.at, next.id); err != nil {
		return fmt.Errorf("move the duplicate scan watermark: %w", err)
	}
	return nil
}

// startNightlyAfterFullPass moves the nightly watermarks of one record type to
// dedupeScanOverlap before this transaction started, when the data task begins
// its full pass. The full pass reads every record as of then, so the nightly
// pass need not read the same rows again. A watermark already later stays.
func startNightlyAfterFullPass(ctx context.Context, tx pgx.Tx, entity DedupeScanEntity) error {
	for _, stream := range dedupeScanStreams {
		if stream.entity != entity {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO dedupe_scan_state AS s (stream, scanned_through_at, scanned_through_id)
			VALUES ($1, now() - make_interval(secs => $2), $3)
			ON CONFLICT (stream) DO UPDATE
			   SET scanned_through_at = excluded.scanned_through_at, scanned_through_id = excluded.scanned_through_id
			 WHERE s.scanned_through_at < excluded.scanned_through_at`,
			stream.table, dedupeScanOverlap.Seconds(), ids.Nil); err != nil {
			return fmt.Errorf("contacts: start the nightly duplicate scan after the full pass: %w", err)
		}
	}
	return nil
}
