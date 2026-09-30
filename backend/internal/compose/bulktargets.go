// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The four record types a bulk change acts on, each reached through its owning
// module's own single-record writes. The engine never writes a record itself:
// it locks, compares and hands each row to the module, so the rules a row is
// changed under are the ones a PATCH or an archive of that row would meet.

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// bulkRow is one record as the bulk change found it under its row lock.
type bulkRow struct {
	label   string
	version int64
	ownerID *ids.UUID
}

// bulkTarget is one record type's share of a bulk change. version is the one
// the caller was shown; each write is conditioned on it.
type bulkTarget interface {
	lock(ctx context.Context, tx pgx.Tx, id ids.UUID) (bulkRow, error)
	reassign(ctx context.Context, tx pgx.Tx, id ids.UUID, owner ids.UserID, version int64) error
}

// bulkArchiver is the share of a record type that has a single-record archive.
// A lead has none — it leaves the queue by being disqualified — so the archive
// verb is refused for it before any row is tried.
type bulkArchiver interface {
	archive(ctx context.Context, tx pgx.Tx, id ids.UUID, version int64) error
	// restore brings an archived record back, conditioned on version, and
	// tries again the links earlier restores of the same undo left behind.
	restore(ctx context.Context, tx pgx.Tx, id ids.UUID, version int64, pending []storekit.LeftBehind) (storekit.RestoreReport, error)
}

// bulkTargets builds the four adapters over the stores the REST handlers use.
func bulkTargets(contactsStore *contacts.Store, dealsStore *deals.Store) map[crmcontracts.BulkRecordType]bulkTarget {
	return map[crmcontracts.BulkRecordType]bulkTarget{
		crmcontracts.BulkRecordTypeContact: contactBulkTarget{store: contactsStore},
		crmcontracts.BulkRecordTypeCompany: companyBulkTarget{store: contactsStore},
		crmcontracts.BulkRecordTypeDeal:    dealBulkTarget{store: dealsStore},
		crmcontracts.BulkRecordTypeLead:    leadBulkTarget{store: contactsStore},
	}
}

// archiveIsBehindErasure is the erasure boundary an un-archive asks of the
// archive row it reverses: privacy's own predicate, through the restore seam's
// reader of it.
func archiveIsBehindErasure(ctx context.Context, tx pgx.Tx, archiveAuditID ids.UUID) (bool, error) {
	return rowIsBehindTheErasureBoundary(ctx, tx, AuditRow{ID: archiveAuditID})
}

type contactBulkTarget struct{ store *contacts.Store }

func (t contactBulkTarget) lock(ctx context.Context, tx pgx.Tx, id ids.UUID) (bulkRow, error) {
	row, err := t.store.LockContactForBulkTx(ctx, tx, ids.From[ids.ContactKind](id))
	return bulkRow{label: row.Label, version: row.Version, ownerID: row.OwnerID}, err
}

func (t contactBulkTarget) reassign(ctx context.Context, tx pgx.Tx, id ids.UUID, owner ids.UserID, version int64) error {
	return t.store.ReassignContactTx(ctx, tx, ids.From[ids.ContactKind](id), owner, &version)
}

func (t contactBulkTarget) archive(ctx context.Context, tx pgx.Tx, id ids.UUID, version int64) error {
	return t.store.ArchiveContactTx(ctx, tx, ids.From[ids.ContactKind](id), &version)
}

func (t contactBulkTarget) restore(
	ctx context.Context, tx pgx.Tx, id ids.UUID, version int64, pending []storekit.LeftBehind,
) (storekit.RestoreReport, error) {
	return t.store.RestoreContactTx(ctx, tx, ids.From[ids.ContactKind](id), &version,
		storekit.RestoreWith{Erased: archiveIsBehindErasure, PendingLinks: pending})
}

type companyBulkTarget struct{ store *contacts.Store }

func (t companyBulkTarget) lock(ctx context.Context, tx pgx.Tx, id ids.UUID) (bulkRow, error) {
	row, err := t.store.LockCompanyForBulkTx(ctx, tx, ids.From[ids.CompanyKind](id))
	return bulkRow{label: row.Label, version: row.Version, ownerID: row.OwnerID}, err
}

func (t companyBulkTarget) reassign(ctx context.Context, tx pgx.Tx, id ids.UUID, owner ids.UserID, version int64) error {
	return t.store.ReassignCompanyTx(ctx, tx, ids.From[ids.CompanyKind](id), owner, &version)
}

func (t companyBulkTarget) archive(ctx context.Context, tx pgx.Tx, id ids.UUID, version int64) error {
	return t.store.ArchiveCompanyTx(ctx, tx, ids.From[ids.CompanyKind](id), &version)
}

func (t companyBulkTarget) restore(
	ctx context.Context, tx pgx.Tx, id ids.UUID, version int64, pending []storekit.LeftBehind,
) (storekit.RestoreReport, error) {
	return t.store.RestoreCompanyTx(ctx, tx, ids.From[ids.CompanyKind](id), &version,
		storekit.RestoreWith{Erased: archiveIsBehindErasure, PendingLinks: pending})
}

type dealBulkTarget struct{ store *deals.Store }

func (t dealBulkTarget) lock(ctx context.Context, tx pgx.Tx, id ids.UUID) (bulkRow, error) {
	row, err := t.store.LockDealForBulkTx(ctx, tx, ids.From[ids.DealKind](id))
	return bulkRow{label: row.Label, version: row.Version, ownerID: row.OwnerID}, err
}

func (t dealBulkTarget) reassign(ctx context.Context, tx pgx.Tx, id ids.UUID, owner ids.UserID, version int64) error {
	return t.store.ReassignDealTx(ctx, tx, ids.From[ids.DealKind](id), owner, &version)
}

func (t dealBulkTarget) archive(ctx context.Context, tx pgx.Tx, id ids.UUID, version int64) error {
	return t.store.ArchiveDealTx(ctx, tx, ids.From[ids.DealKind](id), &version)
}

func (t dealBulkTarget) restore(
	ctx context.Context, tx pgx.Tx, id ids.UUID, version int64, pending []storekit.LeftBehind,
) (storekit.RestoreReport, error) {
	return t.store.RestoreDealTx(ctx, tx, ids.From[ids.DealKind](id), &version,
		storekit.RestoreWith{Erased: archiveIsBehindErasure, PendingLinks: pending})
}

type leadBulkTarget struct{ store *contacts.Store }

func (t leadBulkTarget) lock(ctx context.Context, tx pgx.Tx, id ids.UUID) (bulkRow, error) {
	row, err := t.store.LockLeadForBulkTx(ctx, tx, ids.From[ids.LeadKind](id))
	return bulkRow{label: row.Label, version: row.Version, ownerID: row.OwnerID}, err
}

func (t leadBulkTarget) reassign(ctx context.Context, tx pgx.Tx, id ids.UUID, owner ids.UserID, version int64) error {
	return t.store.ReassignLeadTx(ctx, tx, ids.From[ids.LeadKind](id), owner, &version)
}
