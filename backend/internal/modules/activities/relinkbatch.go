// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// One relink, three doors. A captured conversation that landed on the wrong
// record is usually wrong as a whole, so the remedy for mass mis-attribution
// is the SAME guarded write the single relink performs, applied per row inside
// one transaction: per-activity write check, per-activity audit and outbox
// row, per-activity project stamp. The three entry points differ only in how
// they choose the rows and what they do with a row the caller may not write.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// maxBulkRelink bounds how many activities one explicit-id relink may name.
// The contract declares the same 500; this is the bound that holds for every
// transport.
const maxBulkRelink = 500

// RelinkBatchResult is what the thread and bulk doors answer: how many rows
// gained the link. A row the caller could not write (thread door) or that
// already carried the link is not counted. The ids are not answered — the
// response is replayed under its Idempotency-Key and shown in an approval
// inbox, and neither reader is re-checked against the rows.
type RelinkBatchResult struct {
	Relinked int
}

// admitRelink is the pre-transaction admission every relink door shares: the
// destination must be named, the caller must hold activity.UPDATE, and the
// destination type must be one the timeline files under. It answers the link
// column so the write below never re-derives it.
func admitRelink(ctx context.Context, in RelinkActivityInput) (string, error) {
	// Relinking moves an activity ONTO a record; without an entity_id there is
	// nowhere to move it. Required by the contract, and true only here: the zero
	// UUID reaches the link-target gate and answers not-found for a record the
	// caller never named.
	if err := httperr.RequireBodyID("entity_id", in.EntityID); err != nil {
		return "", err
	}
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return "", err
	}
	column := linkColumn(in.EntityType)
	if column == "" {
		return "", &InvalidLinkTypeError{EntityType: in.EntityType}
	}
	return column, nil
}

// relinkActivityRow is the guarded write for ONE activity, inside the caller's
// transaction, after the caller has probed the destination record. It answers
// whether a link was actually written: replaying the same association is a
// no-op, and a no-op writes no audit noise.
//
// The write check is per row on purpose. Customer identity is workspace-
// readable, so the write arm (auth.EnsureActivityWritable) is what keeps a
// colleague's correspondence theirs — and in a batch every row is somebody's.
func relinkActivityRow(ctx context.Context, tx pgx.Tx, id ids.ActivityID, in RelinkActivityInput, column string) (bool, error) {
	if err := auth.EnsureActivityWritable(ctx, tx, id.UUID); err != nil {
		return false, err
	}
	if err := relinkMeetsItsPin(ctx, tx, id, in.IfVersion); err != nil {
		return false, err
	}
	var displaced []ids.UUID
	if in.ReplaceExistingOfType {
		var err error
		displaced, err = deleteVisibleLinksOfType(ctx, tx, id, in.EntityType, column)
		if err != nil {
			return false, err
		}
	}
	if in.EntityType == linkEntityPerson && in.ReplaceExistingOfType && len(displaced) > 0 {
		if err := repointDisplacedParticipants(ctx, tx, id, in.EntityID, displaced); err != nil {
			return false, err
		}
	}
	tag, err := tx.Exec(ctx, storekit.SQLf(`
		INSERT INTO activity_link (activity_id, entity_type, %s)
		VALUES ($1, $2, $3)
		ON CONFLICT (activity_id, entity_type, `+linkIDCoalesce+`) DO NOTHING`, column),
		id, in.EntityType, in.EntityID)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	// Touch the activity ROW itself, not just its link table: a staged
	// approval pins activity.version (versionTables includes objectActivity),
	// and that pin is the only defense between an approved "send this body on
	// this conversation" and the conversation being silently repointed to
	// someone else before the approval is redeemed. A relink that changes who
	// the activity reaches must therefore move the version the pin re-checks,
	// or a stale approval keeps redeeming as if nothing had changed. The
	// trigger (set_updated_at_bump_version, 0008_activity.up.sql) does the
	// actual bump; this only has to be a genuine UPDATE of the row.
	if _, err := tx.Exec(ctx, `UPDATE activity SET updated_at = now() WHERE id = $1`, id); err != nil {
		return false, err
	}
	// Filing under a project is what qualifies the correspondence (D5), so
	// the stamp commits with the link that earned it.
	if in.EntityType == linkEntityProject {
		if err := StampCorrespondenceForProject(ctx, tx, id, in.EntityID); err != nil {
			return false, err
		}
	}
	auditID, err := storekit.Audit(ctx, tx, "activity_relink", "activity", id.UUID, nil, map[string]any{
		"entity_type": in.EntityType, "entity_id": in.EntityID, "replaced": in.ReplaceExistingOfType,
	})
	if err != nil {
		return false, err
	}
	return true, storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventActivityUpdated{
		ChangedFields: relinkedChangedFields(in.EntityType, in.EntityID),
	})
}

// relinkMeetsItsPin re-checks the version the caller read the activity at,
// inside the transaction that moves it.
//
// UNDER THE ROW LOCK, taken before the read, for the reason every other guarded
// write in this module takes it first: two relinks reading the same version
// concurrently would both pass the compare and the loser would silently
// overwrite the winner. The lock also holds until commit, so the version cannot
// move between this check and the write below it.
//
// No pin is the ordinary case and is not an error: a human's relink through the
// app conditions on nothing, and a static-tier agent call has nothing to
// condition on either.
func relinkMeetsItsPin(ctx context.Context, tx pgx.Tx, id ids.ActivityID, ifVersion *int64) error {
	if ifVersion == nil {
		return nil
	}
	if _, err := storekit.LockRow(ctx, tx, "activity", id.UUID, storekit.LiveOnly); err != nil {
		return err
	}
	current, err := readActivity(ctx, tx, id, storekit.LiveOnly)
	if err != nil {
		return err
	}
	if current.Version == nil || int64(*current.Version) != *ifVersion {
		return apperrors.ErrVersionSkew
	}
	return nil
}

// refuseBatchPin keeps a version pin off the doors it cannot mean.
//
// One version cannot speak for a thread or a named set: applied per row it
// would refuse every activity except the one that happened to match, which
// reads as a partial move nobody asked for. The doors that take a pin are the
// single relink's, and a caller that supplies one here is told so rather than
// having it quietly dropped — a pin silently ignored is the failure this whole
// change is about.
func refuseBatchPin(in RelinkActivityInput) error {
	if in.IfVersion == nil {
		return nil
	}
	return &BatchPinError{}
}

// BatchPinError refuses a version pin on a door that moves many activities.
//
// A MessageFault rather than a field fault: the pin arrives as an If-Match
// HEADER, so there is no request field to name, and inventing one would point
// the caller at an input that is not theirs to change.
type BatchPinError struct{}

func (e *BatchPinError) Error() string {
	return "a thread or a named set moves many activities, and one version cannot condition them"
}

// MessageFault maps this to a 422 carrying the code, so a client can branch on
// the reason rather than parsing prose.
func (e *BatchPinError) MessageFault() (code, message string) {
	return "pin_not_supported", e.Error() +
		" — drop the If-Match and relink them one at a time to pin a version"
}

// RelinkThread applies one relink to every non-archived activity carrying
// threadKey that the caller may write. A row the caller cannot see or cannot
// write is LEFT, not refused: the caller never named it, so there is nothing
// to hide and nothing to answer 403 about — the count says how many moved.
func (s *Store) RelinkThread(ctx context.Context, threadKey string, in RelinkActivityInput) (RelinkBatchResult, error) {
	if threadKey == "" {
		return RelinkBatchResult{}, httperr.Validation("thread_key", "required",
			"thread_key names the conversation to move; it cannot be blank")
	}
	if err := refuseBatchPin(in); err != nil {
		return RelinkBatchResult{}, err
	}
	column, err := admitRelink(ctx, in)
	if err != nil {
		return RelinkBatchResult{}, err
	}
	var out RelinkBatchResult
	err = s.tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureLinkTarget(ctx, tx, in.EntityType, in.EntityID); err != nil {
			return err
		}
		members, err := liveThreadMembers(ctx, tx, threadKey)
		if err != nil {
			return err
		}
		for _, id := range members {
			written, err := relinkActivityRow(ctx, tx, id, in, column)
			switch {
			case errors.Is(err, apperrors.ErrNotFound), errors.Is(err, apperrors.ErrPermissionDenied):
				continue
			case err != nil:
				return err
			case written:
				out.Relinked++
			}
		}
		return nil
	})
	return out, err
}

// liveThreadMembers enumerates the thread's live rows in id order, so two
// concurrent moves of one conversation lock rows in the same sequence. The
// per-row write check is what decides which of them move; the enumeration
// itself answers nothing to the caller.
func liveThreadMembers(ctx context.Context, tx pgx.Tx, threadKey string) ([]ids.ActivityID, error) {
	rows, err := tx.Query(ctx,
		`SELECT id FROM activity WHERE thread_key = $1 AND archived_at IS NULL AND restricted_at IS NULL ORDER BY id`,
		threadKey)
	if err != nil {
		return nil, fmt.Errorf("enumerate thread: %w", err)
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (ids.ActivityID, error) {
		var id ids.ActivityID
		err := r.Scan(&id)
		return id, err
	})
}

// RelinkActivities applies one relink to a set of activities the caller
// NAMED. Here a row the caller cannot see is a 404 and one they cannot write
// is a 403, exactly as the single relink answers for that id — and either
// rolls the whole set back, because a caller who named twelve rows and had
// seven of them move has no way to tell which seven.
func (s *Store) RelinkActivities(ctx context.Context, activityIDs []ids.UUID, in RelinkActivityInput) (RelinkBatchResult, error) {
	if len(activityIDs) == 0 || len(activityIDs) > maxBulkRelink {
		return RelinkBatchResult{}, httperr.Validation("activity_ids", "out_of_range",
			fmt.Sprintf("activity_ids names between 1 and %d activities; this request names %d", maxBulkRelink, len(activityIDs)))
	}
	if err := refuseBatchPin(in); err != nil {
		return RelinkBatchResult{}, err
	}
	column, err := admitRelink(ctx, in)
	if err != nil {
		return RelinkBatchResult{}, err
	}
	var out RelinkBatchResult
	err = s.tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureLinkTarget(ctx, tx, in.EntityType, in.EntityID); err != nil {
			return err
		}
		seen := make(map[ids.UUID]struct{}, len(activityIDs))
		for _, raw := range activityIDs {
			// A repeated id is one row, not two audit rows for one move.
			if _, duplicate := seen[raw]; duplicate {
				continue
			}
			seen[raw] = struct{}{}
			written, err := relinkActivityRow(ctx, tx, ids.From[ids.ActivityKind](raw), in, column)
			if err != nil {
				return err
			}
			if written {
				out.Relinked++
			}
		}
		return nil
	})
	return out, err
}
