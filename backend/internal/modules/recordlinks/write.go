// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package recordlinks

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
	"github.com/margince/margince/backend/internal/shared/ports/recordkinds"
)

const (
	linkAdded   = "added"
	linkRemoved = "removed"
)

// validate refuses a link the registry or the table could not carry. It speaks
// before any row is read, so a bad request learns nothing about the records.
func validate(in NewLink) error {
	if !recordkinds.IsLinkEnd(in.FromType) || !recordkinds.IsLinkEnd(in.ToType) {
		return &values.ParseError{Field: "from_type", Code: "unknown_link_end",
			Message: "a link joins a deal, a company or a contact"}
	}
	declared, ok := recordkinds.LookupLinkType(in.LinkType)
	if !ok {
		return &values.ParseError{Field: "link_type", Code: "unknown_link_type",
			Message: "no core or extension declared this link type; GET /v1/link-types lists them"}
	}
	if !declared.Allows(in.FromType, in.ToType) {
		return &values.ParseError{Field: "link_type", Code: "link_ends_not_allowed",
			Message: fmt.Sprintf("the type %s does not join a %s to a %s", in.LinkType, in.FromType, in.ToType)}
	}
	if in.FromType == in.ToType && in.FromID == in.ToID {
		return &values.ParseError{Field: "to_id", Code: "link_to_itself",
			Message: "a record cannot be linked to itself"}
	}
	return nil
}

// Create links the two records. The caller needs write authority over the
// record the link starts on. They may attach to the one it ends on, the bar for
// filing an activity there. A pair that already holds the type answers the row
// it has and writes nothing.
func (s *Store) Create(ctx context.Context, in NewLink) (crmcontracts.RecordLink, error) {
	if err := validate(in); err != nil {
		return crmcontracts.RecordLink{}, err
	}
	if err := auth.Require(ctx, in.FromType, principal.ActionUpdate); err != nil {
		return crmcontracts.RecordLink{}, err
	}
	if err := auth.Require(ctx, in.ToType, principal.ActionRead); err != nil {
		return crmcontracts.RecordLink{}, err
	}
	capturedBy, err := storekit.CapturedBy(ctx)
	if err != nil {
		return crmcontracts.RecordLink{}, err
	}
	var out crmcontracts.RecordLink
	err = s.tx(ctx, func(tx pgx.Tx) error {
		if err := lockEnds(ctx, tx, in); err != nil {
			return err
		}
		if err := auth.EnsureWritableLive(ctx, tx, in.FromType, in.FromID); err != nil {
			return err
		}
		if err := auth.EnsureAttachTarget(ctx, tx, in.ToType, in.ToID); err != nil {
			return err
		}
		var created bool
		out, created, err = insertLink(ctx, tx, in, capturedBy)
		if err != nil || !created {
			return err
		}
		return recordLinkChange(ctx, tx, out, linkAdded)
	})
	return out, err
}

type linkEnd struct {
	kind string
	id   ids.UUID
}

// lockEnds holds both records, so an erasure of a contact waits for the link
// being written. The ends are locked in one order whichever way the link
// points. Two links between one pair, one each way, would lock crosswise.
func lockEnds(ctx context.Context, tx pgx.Tx, in NewLink) error {
	ends := []linkEnd{{in.FromType, in.FromID}, {in.ToType, in.ToID}}
	slices.SortFunc(ends, func(a, b linkEnd) int {
		return cmp.Or(strings.Compare(a.kind, b.kind), strings.Compare(a.id.String(), b.id.String()))
	})
	for _, end := range ends {
		if err := auth.LockSubjectLive(ctx, tx, end.kind, end.id); err != nil {
			return err
		}
	}
	return nil
}

// insertLink writes the row, or reads the one a repeat would have written.
func insertLink(ctx context.Context, tx pgx.Tx, in NewLink, capturedBy string) (crmcontracts.RecordLink, bool, error) {
	var id ids.UUID
	err := tx.QueryRow(ctx, `
		INSERT INTO record_link (from_type, from_id, to_type, to_id, link_type, captured_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (from_type, from_id, to_type, to_id, link_type) DO NOTHING
		RETURNING id`,
		in.FromType, in.FromID, in.ToType, in.ToID, in.LinkType, capturedBy).Scan(&id)
	created := err == nil
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `
			SELECT id FROM record_link
			 WHERE from_type = $1 AND from_id = $2 AND to_type = $3 AND to_id = $4 AND link_type = $5`,
			in.FromType, in.FromID, in.ToType, in.ToID, in.LinkType).Scan(&id)
	}
	if err != nil {
		return crmcontracts.RecordLink{}, false, fmt.Errorf("recordlinks: writing a link: %w", err)
	}
	link, err := readLink(ctx, tx, id)
	return link, created, err
}

const linkColumns = `id, from_type, from_id, to_type, to_id, link_type, created_at`

func scanLink(row pgx.Row) (crmcontracts.RecordLink, error) {
	var out crmcontracts.RecordLink
	var id, from, to ids.UUID
	var fromType, toType string
	if err := row.Scan(&id, &fromType, &from, &toType, &to, &out.LinkType, &out.CreatedAt); err != nil {
		return out, err
	}
	out.Id, out.FromId, out.ToId = openapi_types.UUID(id), openapi_types.UUID(from), openapi_types.UUID(to)
	out.FromType, out.ToType = crmcontracts.LinkedRecordType(fromType), crmcontracts.LinkedRecordType(toType)
	return out, nil
}

func readLink(ctx context.Context, tx pgx.Tx, id ids.UUID) (crmcontracts.RecordLink, error) {
	return scanLink(tx.QueryRow(ctx, `SELECT `+linkColumns+` FROM record_link WHERE id = $1`, id))
}

// Delete removes a link. The bar is Create's: the caller updates the record the
// link starts on and can see the one it ends on. A link the caller may not see
// answers not found, as it would have on a read.
func (s *Store) Delete(ctx context.Context, id ids.UUID) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		link, err := scanLink(tx.QueryRow(ctx,
			`SELECT `+linkColumns+` FROM record_link WHERE id = $1 FOR UPDATE`, id))
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return err
		}
		from, to := string(link.FromType), string(link.ToType)
		if err := auth.Require(ctx, from, principal.ActionUpdate); err != nil {
			return err
		}
		if err := auth.EnsureWritable(ctx, tx, from, ids.UUID(link.FromId)); err != nil {
			return err
		}
		seen, err := auth.VisibleTo(ctx, tx, to, ids.UUID(link.ToId))
		if err != nil {
			return err
		}
		if !seen {
			return apperrors.ErrNotFound
		}
		if _, err := tx.Exec(ctx, `DELETE FROM record_link WHERE id = $1`, id); err != nil {
			return fmt.Errorf("recordlinks: removing a link: %w", err)
		}
		return recordLinkChange(ctx, tx, link, linkRemoved)
	})
}

// recordLinkChange is the write shape's second half. The audit row and the
// event name the record the link starts on and carry the link's id and type.
// The other end stays off both, because a reader of one record may not be
// allowed to see the other.
func recordLinkChange(ctx context.Context, tx pgx.Tx, link crmcontracts.RecordLink, status string) error {
	from := ids.UUID(link.FromId)
	auditID, err := storekit.AuditEvent(ctx, tx, "update", string(link.FromType), from, map[string]any{
		"record_link": map[string]any{"id": link.Id, "link_type": link.LinkType, "status": status},
	})
	if err != nil {
		return err
	}
	return storekit.EmitEventForEntity(ctx, tx, auditID, string(link.FromType), from,
		crmcontracts.PublicEventRecordLinkChanged{
			LinkId: link.Id, LinkType: link.LinkType, Status: status,
		})
}
