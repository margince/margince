// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// stampArchivable sets each row's archivable flag from auth.ArchivableSubset,
// which asks the row write gate and the archive rule together, so the flag
// answers what the archive itself would.
func stampArchivable[T any](
	ctx context.Context, tx pgx.Tx, rule auth.ArchiveRule, rows []T, id func(T) ids.UUID, set func(*T, bool),
) error {
	named := make([]ids.UUID, len(rows))
	for i := range rows {
		named[i] = id(rows[i])
	}
	archivable, err := auth.ArchivableSubset(ctx, tx, rule, named)
	if err != nil {
		return err
	}
	for i := range rows {
		set(&rows[i], archivable[named[i]])
	}
	return nil
}

// lockThenEnsureArchivable locks the record before it reads who may archive
// it, so an owner change that commits meanwhile is read rather than raced: the
// archive then runs on the authority the row holds when it is written.
func lockThenEnsureArchivable(ctx context.Context, tx pgx.Tx, table string, rule auth.ArchiveRule, id ids.UUID) error {
	if err := lockArchivedRow(ctx, tx, table, id); err != nil {
		return err
	}
	if err := auth.EnsureWritable(ctx, tx, table, id); err != nil {
		return err
	}
	return auth.EnsureArchivable(ctx, tx, rule, id)
}

// stampCompaniesArchivable is stampArchivable for companies, narrowed by the
// refusals the archive itself answers: the anchor, a company that is the only
// one on a live project, and one others' records still name. So the header
// never offers an Archive the store then refuses.
func stampCompaniesArchivable(ctx context.Context, tx pgx.Tx, companies []crmcontracts.Company) error {
	if err := stampArchivable(ctx, tx, auth.CompanyArchive, companies,
		func(o crmcontracts.Company) ids.UUID { return ids.UUID(o.Id) },
		func(o *crmcontracts.Company, may bool) { o.Archivable = &may }); err != nil {
		return err
	}
	for i := range companies {
		if companies[i].Archivable == nil || !*companies[i].Archivable {
			continue
		}
		err := companyArchiveAllowed(ctx, tx, ids.From[ids.CompanyKind](ids.UUID(companies[i].Id)))
		if companyArchiveRefusal(err) {
			companies[i].Archivable = new(false)
			continue
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// lockArchivedRow takes the row lock the archive's patch takes, named per
// table so the write's owner is plain to read.
func lockArchivedRow(ctx context.Context, tx pgx.Tx, table string, id ids.UUID) error {
	var err error
	switch table {
	case contactEntity:
		_, err = storekit.LockRow(ctx, tx, contactEntity, id, storekit.LiveOnly)
	case companyEntity:
		_, err = storekit.LockRow(ctx, tx, companyEntity, id, storekit.LiveOnly)
	default:
		err = fmt.Errorf("contacts: no archive lock for table %q", table)
	}
	return err
}
