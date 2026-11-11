// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// categoryLink is the attachment category of a document link sent by mail: a
// row that points at a document and holds no bytes.
const categoryLink = "link"

// linkNoLongerListed is the archive reason of a link its source stopped listing.
const linkNoLongerListed = "no_longer_listed"

// linkPartPrefix starts the part identity of a link row, so a link's identity
// cannot equal a file's "part:N".
const linkPartPrefix = "link:"

// CapturedLink is one document link a message carried: the canonical address
// and the text the sender showed it under, empty when none was shown.
type CapturedLink struct {
	URL  string
	Text string
}

// RecordCapturedLinks writes an attachment row for each document link of a
// newly captured activity, inside the capture transaction.
//
// The row hangs off the activity alone, so the activity's audience decides who
// sees it. It has bytes_withheld and no storage key, so every byte reader
// refuses it. The unique index on message and address hash stops a second row
// on a replay and keeps an archived link archived. It answers how many rows are
// new.
func (s *Store) RecordCapturedLinks(
	ctx context.Context, tx pgx.Tx, activityID ids.ActivityID,
	from CapturedFileSource, links []CapturedLink,
) (int, error) {
	if err := auth.Require(ctx, "activity", principal.ActionCreate); err != nil {
		return 0, err
	}
	if len(links) == 0 {
		return 0, nil
	}
	added := 0
	from.Category = categoryLink
	for _, link := range links {
		address := link.URL
		sum := sha256.Sum256([]byte(address))
		row := capturedFileRow{
			id: ids.NewV7(),
			file: CapturedFile{
				PartID:   linkPartPrefix + hex.EncodeToString(sum[:]),
				Filename: linkFilename(link),
			},
			withheld: true,
			url:      &address,
		}
		isNew, err := insertCapturedAttachment(ctx, tx, activityID, from, row)
		if err != nil {
			return added, err
		}
		if isNew {
			added++
		}
	}
	return added, nil
}

// RetireCapturedLinks archives the live link rows one source recorded on an
// activity that are not at a kept address, inside the capture transaction. It
// answers how many it archived; each is audited as an archive.
func (s *Store) RetireCapturedLinks(
	ctx context.Context, tx pgx.Tx, activityID ids.ActivityID,
	from CapturedFileSource, kept []string,
) (int, error) {
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return 0, err
	}
	rows, err := tx.Query(ctx, `
		UPDATE attachment SET archived_at = now()
		 WHERE entity_type = 'activity' AND entity_id = $1 AND category = '`+categoryLink+`'
		   AND external_source_id = $2 AND archived_at IS NULL AND NOT (url = ANY($3::text[]))
		RETURNING id`, activityID, providerMessageKey(from), kept)
	if err != nil {
		return 0, fmt.Errorf("activities: archiving the links a source no longer lists: %w", err)
	}
	retired, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return 0, fmt.Errorf("activities: archiving the links a source no longer lists: %w", err)
	}
	for _, id := range retired {
		auditID, err := storekit.AuditWithEvidence(ctx, tx, "archive", "attachment", id, nil,
			map[string]any{fieldEntityType: linkEntityActivity}, map[string]any{evidenceReasonKey: linkNoLongerListed})
		if err != nil {
			return 0, fmt.Errorf("activities: auditing an archived link: %w", err)
		}
		if err := storekit.EmitEvent(ctx, tx, auditID, id, crmcontracts.PublicEventAttachmentArchived{
			ParentType: linkEntityActivity, ParentId: openapi_types.UUID(activityID.UUID),
		}); err != nil {
			return 0, fmt.Errorf("activities: announcing an archived link: %w", err)
		}
	}
	return len(retired), nil
}

// RestoreCapturedLinks brings back the link rows one source recorded on an
// activity, at a listed address, that RetireCapturedLinks archived. A row
// archived by hand stays archived: only an archive whose reason is that the
// source stopped listing it is undone. Each is audited as a restore.
func (s *Store) RestoreCapturedLinks(
	ctx context.Context, tx pgx.Tx, activityID ids.ActivityID,
	from CapturedFileSource, listed []string,
) (int, error) {
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return 0, err
	}
	rows, err := tx.Query(ctx, `
		WITH prior AS (
		  SELECT l.id, l.archived_at FROM attachment l
		   WHERE l.entity_type = 'activity' AND l.entity_id = $1 AND l.category = '`+categoryLink+`'
		     AND l.external_source_id = $2 AND l.archived_at IS NOT NULL AND l.url = ANY($3::text[])
		     AND (SELECT a.evidence ->> '`+evidenceReasonKey+`' FROM audit_log a
		           WHERE a.entity_type = 'attachment' AND a.entity_id = l.id AND a.action = 'archive'
		           ORDER BY a.occurred_at DESC, a.id DESC LIMIT 1) = $4
		     FOR UPDATE)
		UPDATE attachment l SET archived_at = NULL FROM prior WHERE l.id = prior.id
		RETURNING l.id, prior.archived_at`, activityID, providerMessageKey(from), listed, linkNoLongerListed)
	if err != nil {
		return 0, fmt.Errorf("activities: restoring the links a source lists again: %w", err)
	}
	type restoredLink struct {
		id         ids.UUID
		archivedAt time.Time
	}
	restored, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (restoredLink, error) {
		var link restoredLink
		return link, row.Scan(&link.id, &link.archivedAt)
	})
	if err != nil {
		return 0, fmt.Errorf("activities: restoring the links a source lists again: %w", err)
	}
	for _, link := range restored {
		id := link.id
		auditID, err := storekit.Audit(ctx, tx, "restore", "attachment", id,
			map[string]any{"archived_at": link.archivedAt}, map[string]any{"archived_at": nil})
		if err != nil {
			return 0, fmt.Errorf("activities: auditing a restored link: %w", err)
		}
		// The row is back in the activity's listings, which is what created
		// tells a subscriber that keeps its own index of a record's files.
		if err := storekit.EmitEvent(ctx, tx, auditID, id, crmcontracts.PublicEventAttachmentCreated{
			ParentType: linkEntityActivity, ParentId: openapi_types.UUID(activityID.UUID),
		}); err != nil {
			return 0, fmt.Errorf("activities: announcing a restored link: %w", err)
		}
	}
	return len(restored), nil
}

// linkFilename is the name a link row shows: the sender's anchor text cleaned of
// control characters, else the host.
func linkFilename(link CapturedLink) string {
	name := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, link.Text)
	if name = strings.Join(strings.Fields(name), " "); name != "" {
		return name
	}
	return linkHostOf(link.URL)
}

// linkHostOf is the lowercased host of an address, or "link" for one that does
// not parse, so a row always has a name.
func linkHostOf(address string) string {
	parsed, err := url.Parse(address)
	if err != nil || parsed.Hostname() == "" {
		return "link"
	}
	return strings.ToLower(parsed.Hostname())
}
