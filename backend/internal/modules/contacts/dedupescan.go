// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The duplicate scan runs the re-checks a rename runs (renamerecheck.go,
// contactrenamerecheck.go) over records that already exist. So a pair the
// filing rule names reaches the queue, whenever its two records were written.
// The data task walks every record once; the nightly pass (dedupescanchanged.go)
// walks the records changed since its watermark. A pair the queue already holds, decided or not,
// meets its own row on uq_dedupe_candidate_pair, and nothing is filed again.

import (
	"context"
	"fmt"

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

// subjectsSQL is the table and filter that select the records of this type
// the scan may file a pair for. The anchor company is no subject, for the reason
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

// requireRead asks the read grant of the record type, each spelled as its
// own object so the grant census can name it.
func (e DedupeScanEntity) requireRead(ctx context.Context) error {
	switch e {
	case DedupeScanCompanies:
		return auth.Require(ctx, companyEntity, principal.ActionRead)
	case DedupeScanContacts:
		return auth.Require(ctx, contactEntity, principal.ActionRead)
	}
	return fmt.Errorf("contacts: the duplicate scan has no record type %q", string(e))
}

// DedupeScanPage is one batch of the scan. Scanned holds the records it read,
// in order, and Filed counts the pairs new on the queue.
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
	if err := entity.requireRead(ctx); err != nil {
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
// from both reading before the other committed. The scan writes no name and no
// address, so a create beside it reads the scanned record and files the pair
// itself. Lane locks for many records in one transaction would only add lock
// orders a create can deadlock against.
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
	if after == ids.Nil {
		if err := startNightlyAfterFullPass(ctx, tx, entity); err != nil {
			return DedupeScanPage{}, err
		}
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
	if err := entity.requireRead(ctx); err != nil {
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
