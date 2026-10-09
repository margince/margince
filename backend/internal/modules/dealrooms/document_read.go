// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealrooms

// The seller's reads of a room's documents. Every row joins its attachment,
// because the seller needs the stored filename beside the buyer-facing title
// to know which file a row is.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// documentObject is the table storekit patches, locks and audits. The RBAC
// gate is the ROOM's, as for tasks and the roster: curating what a buyer reads
// is part of running the room.
const documentObject = "deal_room_document"

// The four groups, as machine keys. Labels are the client's i18n.
var documentGroups = map[string]bool{
	"commercial":          true,
	"legal":               true,
	"security_privacy":    true,
	"delivery_operations": true,
}

// documentColumns is the projection every document read returns, in the order
// scanDocument consumes it. The attachment's file facts ride along so the
// seller can tell rows apart and the manifest can be frozen from one read.
const documentColumns = `d.id, d.room_id, d.attachment_id, d.group_key, d.title, d.position,
	a.filename, a.content_type, a.byte_size,
	d.source, d.captured_by, d.version, d.created_at, d.updated_at, d.archived_at`

// documentFrom is the seller's join: every entry, whatever became of its file,
// so an entry whose file was archived can still be seen and removed.
const documentFrom = `deal_room_document d
	JOIN deal_room r ON r.id = d.room_id
	JOIN attachment a ON a.id = d.attachment_id`

// inTheDealsFilesArea is the membership rule for a room's documents, spelled
// once and re-applied at ADD, at PUBLISH and at DOWNLOAD: the attachment is
// live, it is in the room's deal's Files area — uploaded on the deal, or
// carried by a message linked to the deal — and it is not hidden from that
// deal. A file archived, unlinked or hidden after it was added drops out of
// the next release and out of the buyer's download, rather than riding on the
// strength of a check that was true when it was added.
//
// A LIMITED message's file is not in a room at all. The company narrowed who
// may read that correspondence; a room exists to hand its documents to an
// outsider, so the two cannot both be honoured and the narrowing wins. Stated
// without a principal because the buyer has none: the question is not whether
// some reader is in the audience, it is whether the message is still open to
// the workspace. A message narrowed, restricted or archived AFTER its file was
// added drops out on the next read. The rest of this predicate re-checks the
// same way. The seller's own list does not carry the predicate, so a seat that
// can remove a stale entry still sees it. It carries sellerReadsTheCarrier
// instead.
//
// It needs the room aliased `r` and the attachment aliased `a`, and carries no
// principal: the public download has none. The caller-bound half — may THIS
// seller see a captured file — is asked once, at add (addDocumentTx).
//
// The activities module spells the same membership for the deal's own Files
// read, inline-image ceiling included (a logo a rep never sees in the area
// must not be shareable by id); it cannot be shared because a module never
// imports a sibling, and
// TestRoomDocumentMembershipIsSpelledOnce holds this module's three readers to
// this one constant.
const inTheDealsFilesArea = `a.archived_at IS NULL AND NOT a.bytes_withheld
	AND ((a.entity_type = 'deal' AND a.entity_id = r.deal_id)
	  OR (a.entity_type = 'activity' AND EXISTS (
	        SELECT 1 FROM activity_link l
	         WHERE l.activity_id = a.entity_id AND l.entity_type = 'deal' AND l.deal_id = r.deal_id)
	      AND NOT (a.content_type LIKE 'image/%' AND COALESCE(a.byte_size, 0) < 65536)
	      AND EXISTS (
	        SELECT 1 FROM activity m
	         WHERE m.id = a.entity_id AND m.archived_at IS NULL
	           AND m.restricted_at IS NULL AND m.audience = 'workspace')))
	AND NOT EXISTS (SELECT 1 FROM deal_document_hide h WHERE h.deal_id = r.deal_id AND h.attachment_id = a.id)`

// sellerReadsTheCarrier is the caller-bound half of every seller read of a
// room document. A file that arrived with a message is shown only to a seat
// that may read that message. To any other seat its title and filename answer
// as absent, as the message does. It needs the attachment aliased `a` and
// renders an AND-prefixed clause. With no seat bound it refuses rather than
// render a clause that answers for nobody.
func sellerReadsTheCarrier(ctx context.Context, arg func(any) int) (string, error) {
	if _, ok := principal.Actor(ctx); !ok {
		return "", apperrors.ErrPermissionDenied
	}
	if !auth.ReadGranted(ctx, "activity") {
		return " AND a.entity_type <> 'activity'", nil
	}
	content, err := auth.ActivityContentClause(ctx, "carrier", arg)
	if err != nil {
		return "", err
	}
	return ` AND (a.entity_type <> 'activity' OR EXISTS (
	        SELECT 1 FROM activity carrier WHERE carrier.id = a.entity_id AND ` + content + `))`, nil
}

// ListDocuments returns a room's documents in group-then-position order.
func (s *Store) ListDocuments(ctx context.Context, roomID ids.DealRoomID) ([]crmcontracts.DealRoomDocument, storekit.Page, error) {
	if err := auth.Require(ctx, roomObject, principal.ActionRead); err != nil {
		return nil, storekit.Page{}, err
	}
	var out []crmcontracts.DealRoomDocument
	err := s.tx(ctx, func(tx pgx.Tx) error {
		// Reading the room first IS the scope gate, exactly as for tasks.
		if _, err := readRoom(ctx, tx, roomID); err != nil {
			return err
		}
		var err error
		out, err = documentRows(ctx, tx, roomID)
		return err
	})
	// Bounded by what one deal hands a buyer; answered whole, page object kept
	// because every list response in this contract carries one.
	return out, storekit.Page{}, err
}

func documentRows(ctx context.Context, tx pgx.Tx, roomID ids.DealRoomID) ([]crmcontracts.DealRoomDocument, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	readable, err := sellerReadsTheCarrier(ctx, arg)
	if err != nil {
		return nil, err
	}
	// The clause is a %s operand, never part of the format. An audience arm
	// may carry a LIKE pattern, which a format string would read as a verb.
	rows, err := tx.Query(ctx, storekit.SQLf(
		`SELECT %s FROM %s
		  WHERE d.room_id = $%d AND d.archived_at IS NULL%s
		  ORDER BY d.group_key, d.position, d.created_at, d.id`,
		documentColumns, documentFrom, arg(roomID), readable), args...)
	if err != nil {
		return nil, fmt.Errorf("list deal room documents: %w", err)
	}
	defer rows.Close()
	var out []crmcontracts.DealRoomDocument
	for rows.Next() {
		doc, err := scanDocument(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, doc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read deal room documents: %w", err)
	}
	return out, nil
}

// readDocument returns one live document in a room; a document of another
// room is absent rather than refused.
func readDocument(ctx context.Context, tx pgx.Tx, roomID ids.DealRoomID, id ids.DealRoomDocumentID) (crmcontracts.DealRoomDocument, error) {
	return readDocumentIn(ctx, tx, roomID, id, " AND d.archived_at IS NULL")
}

// readArchivedDocument is the response to a removal, read from the row because
// the trigger wrote the version.
func readArchivedDocument(ctx context.Context, tx pgx.Tx, roomID ids.DealRoomID, id ids.DealRoomDocumentID) (crmcontracts.DealRoomDocument, error) {
	return readDocumentIn(ctx, tx, roomID, id, "")
}

func readDocumentIn(ctx context.Context, tx pgx.Tx, roomID ids.DealRoomID, id ids.DealRoomDocumentID, liveOnly string) (crmcontracts.DealRoomDocument, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	roomPos, docPos := arg(roomID), arg(id)
	readable, err := sellerReadsTheCarrier(ctx, arg)
	if err != nil {
		return crmcontracts.DealRoomDocument{}, err
	}
	row := tx.QueryRow(ctx, storekit.SQLf(
		`SELECT %s FROM %s WHERE d.room_id = $%d AND d.id = $%d%s%s`,
		documentColumns, documentFrom, roomPos, docPos, liveOnly, readable), args...)
	doc, err := scanDocument(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return crmcontracts.DealRoomDocument{}, apperrors.ErrNotFound
	}
	return doc, err
}

func scanDocument(row rowScanner) (crmcontracts.DealRoomDocument, error) {
	var (
		out        crmcontracts.DealRoomDocument
		group      string
		filename   string
		archivedAt *time.Time
		capturedBy string
	)
	if err := row.Scan(&out.Id, &out.RoomId, &out.AttachmentId, &group, &out.Title, &out.Position,
		&filename, &out.ContentType, &out.ByteSize,
		&out.Source, &capturedBy, &out.Version, &out.CreatedAt, &out.UpdatedAt, &archivedAt); err != nil {
		return crmcontracts.DealRoomDocument{}, fmt.Errorf("scan deal room document: %w", err)
	}
	out.GroupKey = crmcontracts.DealRoomDocumentGroup(group)
	out.Filename = &filename
	out.CapturedBy = &capturedBy
	out.ArchivedAt = archivedAt
	return out, nil
}

// documentIDOf reads the UUID out of a contract id, named once for the three writers.
func documentIDOf(id openapi_types.UUID) ids.DealRoomDocumentID {
	return ids.From[ids.DealRoomDocumentKind](ids.UUID(id))
}
