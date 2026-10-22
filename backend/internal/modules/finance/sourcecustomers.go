// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package finance

// The source's customer directory, and the links an administrator makes from
// it onto companies. A link is always a human's decision: a sync never infers
// one, because a wrong guess puts money on the wrong account.
//
// A company may hold several accounting customers (a business billed under two
// records), and a customer belongs to one company at a time.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const entityCustomerLink = "finance_customer_link"

// directoryPosition is the keyset of the directory's order: name, then id.
type directoryPosition struct {
	Name string `json:"n"`
	ID   string `json:"i"`
}

// customerLinkImage is a link's audited state.
type customerLinkImage struct {
	CompanyID          ids.CompanyID `json:"company_id"`
	ExternalCustomerID string        `json:"external_customer_id"`
}

// ListSourceCustomers answers one page of the live connection's directory,
// each entry with the company it is linked to, if the caller can see it.
func (s *Store) ListSourceCustomers(
	ctx context.Context, cursor *string, limit *int,
) ([]crmcontracts.FinanceSourceCustomer, crmcontracts.PageInfo, error) {
	if err := auth.Require(ctx, objectIntegrations, principal.ActionRead); err != nil {
		return nil, crmcontracts.PageInfo{}, err
	}
	var after *directoryPosition
	if cursor != nil && *cursor != "" {
		position, err := storekit.DecodeOpaque[directoryPosition](*cursor)
		if err != nil || position.ID == "" {
			return nil, crmcontracts.PageInfo{}, fmt.Errorf("%w: the cursor is not one this list issued", apperrors.ErrInvalidArgument)
		}
		after = &position
	}
	size := storekit.ClampLimit(limit)
	out := []crmcontracts.FinanceSourceCustomer{}
	var page crmcontracts.PageInfo
	err := s.tx(ctx, func(tx pgx.Tx) error {
		conn, connected, err := readConnection(ctx, tx)
		if err != nil || !connected {
			return err
		}
		rows, err := readDirectoryPage(ctx, tx, conn.id, after, size+1)
		if err != nil {
			return err
		}
		if len(rows) > size {
			rows = rows[:size]
			last := rows[size-1]
			next, err := storekit.EncodeOpaque(directoryPosition{Name: last.DisplayName, ID: last.ExternalId})
			if err != nil {
				return err
			}
			page = crmcontracts.PageInfo{HasMore: true, NextCursor: &next}
		}
		out = rows
		return nil
	})
	return out, page, err
}

func readDirectoryPage(
	ctx context.Context, tx pgx.Tx, connectionID ids.UUID, after *directoryPosition, limit int,
) ([]crmcontracts.FinanceSourceCustomer, error) {
	// A caller without the company grant sees the directory without company
	// names. One with it sees only the companies its row scope reaches.
	showCompanies := auth.ReadGranted(ctx, "company")
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	connPos, limitPos := arg(connectionID), arg(limit)
	visible, err := auth.ScopeClauseFor(ctx, "company", "o", arg)
	if err != nil {
		return nil, err
	}
	if visible != "" {
		visible = "AND " + visible
	}
	keyset := ""
	if after != nil {
		keyset = fmt.Sprintf("AND (c.display_name, c.external_customer_id) > ($%d, $%d)", arg(after.Name), arg(after.ID))
	}
	rows, err := tx.Query(ctx, storekit.SQLf(`
		SELECT c.external_customer_id, c.display_name, o.id, o.display_name
		  FROM finance_external_customer c
		  LEFT JOIN finance_customer_link l
		         ON l.connection_id = c.connection_id
		        AND l.external_customer_id = c.external_customer_id
		        AND l.archived_at IS NULL
		  LEFT JOIN company o ON o.id = l.company_id AND o.archived_at IS NULL %s
		 WHERE c.connection_id = $%d AND c.archived_at IS NULL %s
		 ORDER BY c.display_name, c.external_customer_id
		 LIMIT $%d`, visible, connPos, keyset, limitPos), args...)
	if err != nil {
		return nil, fmt.Errorf("read the source's customers: %w", err)
	}
	defer rows.Close()
	var out []crmcontracts.FinanceSourceCustomer
	for rows.Next() {
		var (
			each        crmcontracts.FinanceSourceCustomer
			companyID   *ids.UUID
			companyName *string
		)
		if err := rows.Scan(&each.ExternalId, &each.DisplayName, &companyID, &companyName); err != nil {
			return nil, fmt.Errorf("scan a source customer: %w", err)
		}
		if showCompanies && companyID != nil && companyName != nil {
			each.Company = &crmcontracts.FinanceLinkedCompany{Id: openapi_types.UUID(*companyID), DisplayName: *companyName}
		}
		out = append(out, each)
	}
	return out, rows.Err()
}

// LinkCustomer links one source customer onto a company the caller can see.
// Relinking moves the customer; its mirrored invoices follow on the next sync.
func (s *Store) LinkCustomer(
	ctx context.Context, externalID string, companyID ids.CompanyID,
) (crmcontracts.FinanceSourceCustomer, error) {
	if err := auth.RequireHuman(ctx); err != nil {
		return crmcontracts.FinanceSourceCustomer{}, err
	}
	if err := auth.Require(ctx, objectIntegrations, principal.ActionUpdate); err != nil {
		return crmcontracts.FinanceSourceCustomer{}, err
	}
	if err := auth.Require(ctx, "company", principal.ActionRead); err != nil {
		return crmcontracts.FinanceSourceCustomer{}, err
	}
	var out crmcontracts.FinanceSourceCustomer
	err := s.tx(ctx, func(tx pgx.Tx) error {
		// Write authority on the company, not visibility: a link puts money on
		// its account, which a read-only share must not be able to do.
		if err := auth.EnsureWritableLive(ctx, tx, "company", companyID.UUID); err != nil {
			return err
		}
		conn, name, err := s.lockDirectoryEntry(ctx, tx, externalID)
		if err != nil {
			return err
		}
		if err := writeLink(ctx, tx, conn, externalID, companyID); err != nil {
			return err
		}
		var companyName string
		if err := tx.QueryRow(ctx, `SELECT display_name FROM company WHERE id = $1`, companyID).Scan(&companyName); err != nil {
			return fmt.Errorf("read the linked company: %w", err)
		}
		out = crmcontracts.FinanceSourceCustomer{
			ExternalId: externalID, DisplayName: name,
			Company: &crmcontracts.FinanceLinkedCompany{Id: openapi_types.UUID(companyID.UUID), DisplayName: companyName},
		}
		return nil
	})
	return out, err
}

// UnlinkCustomer archives a customer's live link. The sync stops reading its
// ledger; what it already mirrored stays where it is.
func (s *Store) UnlinkCustomer(ctx context.Context, externalID string) error {
	if err := auth.RequireHuman(ctx); err != nil {
		return err
	}
	if err := auth.Require(ctx, objectIntegrations, principal.ActionUpdate); err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		conn, _, err := s.lockDirectoryEntry(ctx, tx, externalID)
		if err != nil {
			return err
		}
		link, linked, err := readLiveLink(ctx, tx, conn, externalID)
		if err != nil || !linked {
			return err
		}
		if err := updateLiveLink(ctx, tx, link, "archived_at = now()"); err != nil {
			return err
		}
		_, err = storekit.Audit(ctx, tx, "archive", entityCustomerLink, link.id, link.image, nil)
		return err
	})
}

// lockDirectoryEntry resolves the live connection and one entry of its
// directory, serializing writers of that entry's link. ErrNotFound when either
// is missing.
func (s *Store) lockDirectoryEntry(ctx context.Context, tx pgx.Tx, externalID string) (ids.UUID, string, error) {
	conn, connected, err := readConnection(ctx, tx)
	if err != nil {
		return ids.UUID{}, "", err
	}
	if !connected {
		return ids.UUID{}, "", fmt.Errorf("%w: no accounting source is connected", apperrors.ErrNotFound)
	}
	if err := storekit.LockWriteIdentity(ctx, tx, entityCustomerLink, conn.id.String()+"/"+externalID); err != nil {
		return ids.UUID{}, "", err
	}
	var name string
	err = tx.QueryRow(ctx, `
		SELECT display_name FROM finance_external_customer
		 WHERE connection_id = $1 AND external_customer_id = $2 AND archived_at IS NULL`,
		conn.id, externalID).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.UUID{}, "", fmt.Errorf("%w: the source lists no customer %q", apperrors.ErrNotFound, externalID)
	}
	if err != nil {
		return ids.UUID{}, "", fmt.Errorf("read the source customer: %w", err)
	}
	return conn.id, name, nil
}

// liveLink is a customer's live link as read, with the version its write
// guards on.
type liveLink struct {
	id      ids.UUID
	version int64
	image   customerLinkImage
}

func readLiveLink(
	ctx context.Context, tx pgx.Tx, connectionID ids.UUID, externalID string,
) (liveLink, bool, error) {
	link := liveLink{image: customerLinkImage{ExternalCustomerID: externalID}}
	err := tx.QueryRow(ctx, `
		SELECT id, version, company_id FROM finance_customer_link
		 WHERE connection_id = $1 AND external_customer_id = $2 AND archived_at IS NULL`,
		connectionID, externalID).Scan(&link.id, &link.version, &link.image.CompanyID)
	if errors.Is(err, pgx.ErrNoRows) {
		return liveLink{}, false, nil
	}
	if err != nil {
		return liveLink{}, false, fmt.Errorf("read the customer link: %w", err)
	}
	// Moving or removing a link takes money off its company, so it needs the
	// same write authority on that company as making the link did.
	if err := auth.EnsureWritable(ctx, tx, "company", link.image.CompanyID.UUID); err != nil {
		return liveLink{}, false, err
	}
	return link, true, nil
}

// updateLiveLink runs one guarded write of a live link: the version it was
// read at, and never an archived row. A miss is a concurrent change.
func updateLiveLink(ctx context.Context, tx pgx.Tx, link liveLink, set string, args ...any) error {
	args = append([]any{link.id, link.version}, args...)
	tag, err := tx.Exec(ctx, storekit.SQLf(`
		UPDATE finance_customer_link SET %s
		 WHERE id = $1 AND version = $2 AND archived_at IS NULL`, set), args...)
	if err != nil {
		return fmt.Errorf("write the customer link: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: the customer link changed meanwhile; reload and try again", apperrors.ErrConflict)
	}
	return nil
}

// writeLink points the customer's live link at a company, creating it when
// there is none. Linking to the company it already names writes nothing.
func writeLink(ctx context.Context, tx pgx.Tx, connectionID ids.UUID, externalID string, companyID ids.CompanyID) error {
	link, linked, err := readLiveLink(ctx, tx, connectionID, externalID)
	if err != nil {
		return err
	}
	after := customerLinkImage{CompanyID: companyID, ExternalCustomerID: externalID}
	if linked {
		if link.image.CompanyID == companyID {
			return nil
		}
		if err := updateLiveLink(ctx, tx, link, "company_id = $3", companyID); err != nil {
			return err
		}
		_, err = storekit.Audit(ctx, tx, "update", entityCustomerLink, link.id, link.image, after)
		return err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	linkID := ids.NewV7()
	// sync_hash is required by the table and unused for a link: a link is a
	// human's decision, never compared against a source.
	if _, err := tx.Exec(ctx, `
		INSERT INTO finance_customer_link
		       (id, connection_id, company_id, external_customer_id, sync_hash, source, captured_by)
		VALUES ($1, $2, $3, $4, '', 'manual', $5)`,
		linkID, connectionID, companyID, externalID, by); err != nil {
		return fmt.Errorf("record the customer link: %w", err)
	}
	_, err = storekit.Audit(ctx, tx, "create", entityCustomerLink, linkID, nil, after)
	return err
}
