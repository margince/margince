// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const mailCopyReason = "mail_duplicate"

// MailCopyCandidatesSQL uses message identities, never a conversation key shared by replies.
const MailCopyCandidatesSQL = `FROM activity copy
 LEFT JOIN raw_capture original ON original.id=copy.raw_capture_id
 LEFT JOIN LATERAL (SELECT evidence->>'` + fieldRFCMessageID + `' AS message_id FROM audit_log WHERE entity_type='activity' AND entity_id=copy.id AND action='create' AND evidence ? '` + fieldRFCMessageID + `' ORDER BY occurred_at,id LIMIT 1) claimed ON true
 JOIN activity_identity identity ON identity.identity_kind='mail' AND identity.identity_key=
 coalesce(nullif(claimed.message_id,''),nullif(copy.raw->>'rfc_message_id',''),nullif(coalesce(original.payload_object->>'rfc_message_id',original.payload->>'rfc_message_id'),''),
  CASE WHEN copy.source_system='email' THEN split_part(copy.source_id,'` + connector.MailSeatMarker + `',1) END)
 JOIN activity kept ON kept.id=identity.activity_id
 WHERE copy.kind='email' AND copy.archived_at IS NULL AND kept.archived_at IS NULL
 AND copy.restricted_at IS NULL AND kept.restricted_at IS NULL AND copy.id<>kept.id`

// MailCopy names two proven copies without loading their message content.
type MailCopy struct{ ID, Kept ids.UUID }

// MailCopiesTx pages repair candidates inside the data task transaction.
func MailCopiesTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) ([]MailCopy, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	args := schedulingArgs{}
	rows, err := tx.Query(ctx, `SELECT copy.id,kept.id `+MailCopyCandidatesSQL+` AND copy.id>`+args.add(after)+` ORDER BY copy.id LIMIT `+args.add(limit), args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (MailCopy, error) {
		var one MailCopy
		err := row.Scan(&one.ID, &one.Kept)
		return one, err
	})
}

// CountMailCopies caps the estimate so counting cannot dominate repair work.
func CountMailCopies(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	var count int64
	args := schedulingArgs{}
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT copy.id `+MailCopyCandidatesSQL+` LIMIT `+args.add(maximum+1)+`) candidates`, args...).Scan(&count)
	return min(count, maximum), count > maximum, err
}

// FoldMailCopyTx keeps mailbox evidence and references together before retiring a duplicate.
func (s *Store) FoldMailCopyTx(ctx context.Context, tx pgx.Tx, pair MailCopy, proved AddressProver, carryImports func(context.Context, pgx.Tx, ids.ActivityID, ids.ActivityID) (bool, error)) (bool, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return false, err
	}
	kept, duplicate := ids.From[ids.ActivityKind](pair.Kept), ids.From[ids.ActivityKind](pair.ID)
	stamps := map[ids.UUID]string{}
	args := schedulingArgs{}
	rows, err := tx.Query(ctx, `SELECT id,captured_by FROM activity WHERE id=ANY(`+args.add([]ids.UUID{pair.ID, pair.Kept})+`) AND archived_at IS NULL AND restricted_at IS NULL ORDER BY id FOR UPDATE`, args...)
	if err != nil {
		return false, err
	}
	var rowID ids.UUID
	var stamp string
	_, err = pgx.ForEachRow(rows, []any{&rowID, &stamp}, func() error { stamps[rowID] = stamp; return nil })
	if err != nil {
		return false, err
	}
	if len(stamps) != 2 {
		return false, nil
	}
	var same bool
	args = schedulingArgs{}
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 `+MailCopyCandidatesSQL+` AND copy.id=`+args.add(pair.ID)+` AND kept.id=`+args.add(pair.Kept)+`)`, args...).Scan(&same); err != nil {
		return false, err
	}
	if !same {
		return false, nil
	}
	if strings.HasPrefix(stamps[pair.ID], "connector:") && strings.HasPrefix(stamps[pair.Kept], "human:") {
		kept, duplicate = duplicate, kept
	}
	referenced, err := mailCopyReferencedTx(ctx, tx, duplicate)
	if err != nil || referenced {
		return false, err
	}
	attachable, err := mailCopyLinksAttachableTx(ctx, tx, duplicate, kept)
	if err != nil || !attachable {
		return false, err
	}
	allowed, err := mailCopiesShareSeat(ctx, tx, kept, duplicate, stamps[kept.UUID], stamps[duplicate.UUID], proved)
	if err != nil || !allowed {
		return false, err
	}
	return s.foldAdmittedMailCopyTx(ctx, tx, duplicate, kept, carryImports)
}

func (s *Store) foldAdmittedMailCopyTx(ctx context.Context, tx pgx.Tx, duplicate, kept ids.ActivityID, carryImports func(context.Context, pgx.Tx, ids.ActivityID, ids.ActivityID) (bool, error)) (bool, error) {
	allowed, err := carryImports(ctx, tx, duplicate, kept)
	if err != nil || !allowed {
		return false, err
	}
	if err := carryMailCopyTx(ctx, tx, duplicate, kept); err != nil {
		return false, err
	}
	if err := RecomputeAudiencesTx(ctx, tx, []ids.ActivityID{kept}); err != nil {
		return false, err
	}
	if err := TransferIdentities(ctx, tx, duplicate, kept); err != nil {
		return false, err
	}
	patch := storekit.NewPatch()
	patch.Set("archived_at", nil, s.now())
	lock, err := storekit.LockRow(ctx, tx, "activity", duplicate.UUID, storekit.LiveOnly)
	if err != nil {
		return false, err
	}
	if err := patch.ApplyLocked(ctx, tx, lock); err != nil {
		return false, err
	}
	audit, err := storekit.AuditWithEvidence(ctx, tx, "archive", "activity", duplicate.UUID, patch.Before(), patch.After(), map[string]any{meetingCopySurvivorKey: kept.UUID, evidenceReasonKey: mailCopyReason})
	if err != nil {
		return false, err
	}
	return true, storekit.EmitEvent(ctx, tx, audit, duplicate.UUID, crmcontracts.PublicEventActivityArchived{})
}

func mailCopiesShareSeat(ctx context.Context, tx pgx.Tx, kept, duplicate ids.ActivityID, keepStamp, copyStamp string, proved AddressProver) (bool, error) {
	seat := actingHumanOf(keepStamp)
	if seat != "" && seat == actingHumanOf(copyStamp) {
		return true, nil
	}
	if !strings.HasPrefix(keepStamp, "connector:") || !strings.HasPrefix(copyStamp, "human:") {
		return false, nil
	}
	args := schedulingArgs{}
	rows, err := tx.Query(ctx, `SELECT address FROM activity_participant WHERE activity_id=`+args.add(duplicate)+` AND coalesce(address,'')<>''`, args...)
	if err != nil {
		return false, err
	}
	addresses, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return false, err
	}
	return importNamesIncumbentSeat(ctx, tx, kept, proved, addresses)
}

func carryMailCopyTx(ctx context.Context, tx pgx.Tx, duplicate, kept ids.ActivityID) error {
	args := schedulingArgs{}
	rows, err := tx.Query(ctx, `SELECT entity_type,`+linkIDCoalesce+` FROM activity_link WHERE activity_id=`+args.add(duplicate)+``, args...)
	if err != nil {
		return err
	}
	links, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (RelinkActivityInput, error) {
		var one RelinkActivityInput
		err := row.Scan(&one.EntityType, &one.EntityID)
		return one, err
	})
	if err != nil {
		return err
	}
	for _, link := range links {
		if _, err := RelinkActivityInTx(ctx, tx, kept, link); err != nil {
			return err
		}
	}
	if err := carryMailAttachmentsTx(ctx, tx, duplicate, kept); err != nil {
		return err
	}
	args = schedulingArgs{}
	_, err = tx.Exec(ctx, `INSERT INTO activity_reader_state (activity_id,reader_id,state,snoozed_until,reopen_on,reopen_ref,set_by,set_at,snoozed_wall,snoozed_zone)
 SELECT `+args.add(kept)+`,reader_id,state,snoozed_until,reopen_on,reopen_ref,set_by,set_at,snoozed_wall,snoozed_zone FROM activity_reader_state WHERE activity_id=`+args.add(duplicate)+`
 ON CONFLICT (activity_id,reader_id) DO UPDATE SET state=excluded.state,snoozed_until=excluded.snoozed_until,reopen_on=excluded.reopen_on,reopen_ref=excluded.reopen_ref,set_by=excluded.set_by,set_at=excluded.set_at,snoozed_wall=excluded.snoozed_wall,snoozed_zone=excluded.snoozed_zone
 WHERE activity_reader_state.set_at<excluded.set_at`, args...)
	return err
}

func carryMailAttachmentsTx(ctx context.Context, tx pgx.Tx, duplicate, kept ids.ActivityID) error {
	args := schedulingArgs{}
	rows, err := tx.Query(ctx, `SELECT id FROM attachment WHERE activity_id=`+args.add(duplicate)+` AND archived_at IS NULL ORDER BY id FOR UPDATE`, args...)
	if err != nil {
		return err
	}
	files, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return err
	}
	for _, id := range files {
		patch := storekit.NewPatch()
		patch.Set("activity_id", duplicate.UUID, kept.UUID)
		patch.Set("entity_id", duplicate.UUID, kept.UUID)
		lock, err := storekit.LockRow(ctx, tx, "attachment", id, storekit.LiveOnly)
		if err != nil {
			return err
		}
		if err := patch.ApplyLocked(ctx, tx, lock); err != nil {
			return err
		}
		audit, err := storekit.Audit(ctx, tx, "update", "attachment", id, patch.Before(), patch.After())
		if err != nil {
			return err
		}
		if err := storekit.EmitEvent(ctx, tx, audit, id, crmcontracts.PublicEventAttachmentUpdated{ParentType: linkEntityActivity, ParentId: openapi_types.UUID(kept.UUID)}); err != nil {
			return fmt.Errorf("activities: announcing carried file: %w", err)
		}
	}
	return nil
}

func mailCopyLinksAttachableTx(ctx context.Context, tx pgx.Tx, duplicate, kept ids.ActivityID) (bool, error) {
	args := schedulingArgs{}
	rows, err := tx.Query(ctx, `SELECT entity_type,`+linkIDCoalesce+` FROM activity_link WHERE activity_id=`+args.add(duplicate), args...)
	if err != nil {
		return false, err
	}
	links, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (ActivityLinkInput, error) {
		var link ActivityLinkInput
		err := row.Scan(&link.EntityType, &link.EntityID)
		return link, err
	})
	if err != nil {
		return false, err
	}
	for _, link := range links {
		if err := mayAttach(ctx, tx, link); errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
			return false, nil
		} else if err != nil {
			return false, err
		}
		if link.EntityType == linkEntityProject {
			var conflict bool
			args = schedulingArgs{}
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM activity_link WHERE activity_id=`+args.add(kept)+` AND project_id IS NOT NULL AND project_id<>`+args.add(link.EntityID)+`)`, args...).Scan(&conflict); err != nil {
				return false, err
			}
			if conflict {
				return false, nil
			}
		}
	}
	return true, nil
}
