// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package recordlinks

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
	"github.com/margince/margince/backend/internal/shared/ports/recordkinds"
)

// List answers the links that start or end on one record, newest first. A link
// is left out when the caller may not open its other end, or that end is
// archived.
func (s *Store) List(ctx context.Context, recordType string, recordID ids.UUID) ([]crmcontracts.RecordLink, error) {
	if !recordkinds.IsLinkEnd(recordType) {
		return nil, &values.ParseError{Field: "record_type", Code: "unknown_link_end",
			Message: "links are read for a deal, a company or a contact"}
	}
	if err := auth.Require(ctx, recordType, principal.ActionRead); err != nil {
		return nil, err
	}
	out := []crmcontracts.RecordLink{}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureVisibleExisting(ctx, tx, recordType, recordID); err != nil {
			return err
		}
		own, live, err := liveName(ctx, tx, recordType, recordID)
		if err != nil || !live {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT `+linkColumns+` FROM record_link
			 WHERE (from_type = $1 AND from_id = $2) OR (to_type = $1 AND to_id = $2)
			 ORDER BY created_at DESC, id LIMIT $3`, recordType, recordID, listCap)
		if err != nil {
			return fmt.Errorf("recordlinks: reading links: %w", err)
		}
		links, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (crmcontracts.RecordLink, error) { return scanLink(r) })
		if err != nil {
			return fmt.Errorf("recordlinks: reading links: %w", err)
		}
		for _, link := range links {
			named, seen, err := nameOtherEnd(ctx, tx, link, recordType, recordID, own)
			if err != nil {
				return err
			}
			if seen {
				out = append(out, named)
			}
		}
		return nil
	})
	return out, err
}

// nameOtherEnd fills both names of a link read from one record, and reports
// false when the other end is not the caller's to see.
func nameOtherEnd(ctx context.Context, tx pgx.Tx, link crmcontracts.RecordLink,
	recordType string, recordID ids.UUID, own string,
) (crmcontracts.RecordLink, bool, error) {
	startsHere := string(link.FromType) == recordType && ids.UUID(link.FromId) == recordID
	kind, id := string(link.ToType), ids.UUID(link.ToId)
	if !startsHere {
		kind, id = string(link.FromType), ids.UUID(link.FromId)
	}
	seen, err := auth.VisibleTo(ctx, tx, kind, id)
	if err != nil || !seen {
		return link, false, err
	}
	other, live, err := liveName(ctx, tx, kind, id)
	if err != nil || !live {
		return link, false, err
	}
	if startsHere {
		link.FromName, link.ToName = &own, &other
	} else {
		link.FromName, link.ToName = &other, &own
	}
	return link, true, nil
}

// liveName answers a record's name, and false when it is archived or gone.
func liveName(ctx context.Context, tx pgx.Tx, kind string, id ids.UUID) (string, bool, error) {
	column, ok := nameColumns[kind]
	if !ok {
		return "", false, apperrors.ErrNotFound
	}
	var name string
	err := tx.QueryRow(ctx, storekit.SQLf(`SELECT %s FROM %s WHERE id = $1 AND archived_at IS NULL`, column, kind), id).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	return name, err == nil, err
}
