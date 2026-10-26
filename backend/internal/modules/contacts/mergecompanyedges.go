// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// relinkCompanyEdges moves A's relationship edges to B across both company
// columns. Order matters: edges that would degenerate (A↔B partner
// edges, duplicates of what B already has) archive first, then the
// survivors relink.
func relinkCompanyEdges(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.CompanyID) error {
	if err := relinkEmploymentSupport(ctx, tx, sourceID.UUID, targetID.UUID, false); err != nil {
		return err
	}
	args := []any{sourceID}
	sourcePos := len(args)
	args = append(args, targetID)
	if _, err := tx.Exec(ctx, storekit.SQLf(`UPDATE provider_employment_resolution SET company_id=$%d WHERE company_id=$%d`, len(args), sourcePos), args...); err != nil {
		return err
	}
	mergeArgs := pgx.NamedArgs{mergeSourceKey: sourceID, mergeTargetKey: targetID, archiveStateKey: time.Now().UTC()}
	if _, err := mergeEdgeWrite(ctx, tx, `archived_at=@archived`, mergeCompanyEdgeSource+` AND `+mergeCompanyEdgeSelf, mergeArgs); err != nil {
		return err
	}
	if err := foldFirstObservations(ctx, tx, companyFK, sourceID.UUID, targetID.UUID); err != nil {
		return err
	}
	if _, err := mergeEdgeWrite(ctx, tx, `archived_at=@archived`, mergeCompanyEdgeSource+` AND `+mergeCompanyEdgeDuplicate, mergeArgs); err != nil {
		return err
	}
	if _, err := mergeEdgeWrite(ctx, tx, `company_id=@target,is_current_primary=`+mergeCompanyEdgePrimary, mergeCompanyForwardSource, mergeArgs); err != nil {
		return err
	}
	_, err := mergeEdgeWrite(ctx, tx, `counterparty_company_id=@target`, `a.counterparty_company_id=@source AND a.archived_at IS NULL`, mergeArgs)
	return err
}

// primaryEmployersOf maps each live contact whose current primary employer is
// the company to that employer. Duplicate scoring reads only the current primary
// employer, so an ended or non-primary edge cannot create a same-employer twin.
// A company merge takes this before the edges move, because afterwards the
// source no longer names them.
func primaryEmployersOf(ctx context.Context, tx pgx.Tx, company ids.CompanyID) (map[ids.ContactID]ids.CompanyID, error) {
	rows, err := tx.Query(ctx, storekit.SQLf(`SELECT DISTINCT r.contact_id FROM relationship r
 JOIN contact c ON c.id = r.contact_id AND c.archived_at IS NULL
 WHERE r.kind=$1 AND r.company_id=$2 AND r.archived_at IS NULL AND %s`, employment.CurrentPrimarySQL("r")),
		employmentKind, company)
	if err != nil {
		return nil, fmt.Errorf("list the primary employees of the merged-away company: %w", err)
	}
	contacts, err := pgx.CollectRows(rows, pgx.RowTo[ids.ContactID])
	if err != nil {
		return nil, fmt.Errorf("list the primary employees of the merged-away company: %w", err)
	}
	before := make(map[ids.ContactID]ids.CompanyID, len(contacts))
	for _, contact := range contacts {
		employer, _, err := readCurrentEmployer(ctx, tx, contact.UUID)
		if err != nil {
			return nil, err
		}
		if employer != nil {
			before[contact] = *employer
		}
	}
	return before, nil
}

// fileMergedEmployerTwins files the same-employer twins the merge created for
// the contacts whose current primary employer it changed.
//
// Every lane lock of every such contact is taken first, in one global order:
// subject keys, address bases, names, each sorted across all contacts. The
// re-check then re-takes locks it already holds. Locking contact by contact
// could hold one name lane while waiting on the next contact's address lock.
func fileMergedEmployerTwins(ctx context.Context, tx pgx.Tx, before map[ids.ContactID]ids.CompanyID, by string) error {
	moved := make([]contactSubject, 0, len(before))
	for contact, was := range before {
		now, _, err := readCurrentEmployer(ctx, tx, contact.UUID)
		if err != nil {
			return err
		}
		if now == nil || *now == was {
			continue
		}
		subject, found, err := readContactSubject(ctx, tx, contact)
		if err != nil {
			return err
		}
		if found {
			moved = append(moved, subject)
		}
	}
	slices.SortFunc(moved, func(a, b contactSubject) int { return strings.Compare(a.id.String(), b.id.String()) })
	if err := lockLanesForContacts(ctx, tx, moved); err != nil {
		return err
	}
	for _, subject := range moved {
		if err := fileEmployerTwins(ctx, tx, subject.id, by); err != nil {
			return err
		}
	}
	return nil
}
