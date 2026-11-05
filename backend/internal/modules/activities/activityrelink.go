// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Moving an activity from one record to another: the admitted write itself and
// the two sweeps it owes — the links it displaces, and the participants those
// links were speaking for.
//
// Beside RelinkActivity (relinkbatch.go) rather than in lifecycle.go, which is
// where they were: a relink is not a patch of the activity's own fields, and
// the file that holds the patch had grown to hold both.

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// relinkAdmittedRow is the transactional half both doors share: the target
// probe, then the guarded row write. The admission stays outside it so the
// single door can refuse a malformed request before it opens a transaction.
func relinkAdmittedRow(ctx context.Context, tx pgx.Tx, id ids.ActivityID, in RelinkActivityInput, column string) (wrote, held bool, err error) {
	// The relink target is a client-supplied reference (H1), and moving an
	// activity onto it is an ATTACH: the record it lands on gains the activity.
	if err := auth.EnsureAttachTarget(ctx, tx, in.EntityType, in.EntityID); err != nil {
		return false, false, err
	}
	return relinkActivityRow(ctx, tx, id, in, column)
}

// deleteVisibleLinksOfType drops the activity's links of one entity type and
// answers the entity ids that delete actually removed. Those ids come from
// the delete ITSELF. Inferring them instead — "whoever is a participant but no
// longer linked" — sweeps up participants that were never linked in the first
// place, and repoints conversations the correction never mentioned.
//
// Only the links this caller can SEE are replaced. An activity's own
// visibility derives from its links, so an unscoped delete lets someone who
// reached this activity through one link cut another — dropping a team's sight
// of a record by rewriting an association they were never shown.
//
// A link outside the caller's scope survives instead, and for `project` that
// used to leave a residual: at most one project link may exist, so the insert
// then hit the partial index and refused, and the difference between that
// refusal and a success told the caller a project link they could not see was
// there. One bit escaped, and hiding a link's existence while enforcing
// one-per-activity looked like the same question asked twice.
//
// It is closed, and closed on both halves rather than narrowed. A project
// carries no own/team arm (platform/auth tableclass.go) and no capture privacy
// either — its visibility CHECK admits 'workspace' and nothing else, because
// nothing auto-creates a project and an owner-private one was a state no writer
// could reach. So no project link can be invisible to a caller
// holding the object grant: the delete reaches every one, and the move
// succeeds. The 23505 path below still stands for the caller who asks to
// associate rather than move.
func deleteVisibleLinksOfType(ctx context.Context, tx pgx.Tx, id ids.ActivityID, entityType, column string) ([]ids.UUID, error) {
	predicate, args, err := visibleRelinkPredicate(ctx, id, entityType, column)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, storekit.SQLf("DELETE FROM activity_link WHERE %s RETURNING %s", predicate, column), args...)
	if err != nil {
		return nil, err
	}
	// Every id this delete actually removed: the audit row's restore image,
	// and for contacts the set whose participants are repointed.
	displaced, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (ids.UUID, error) {
		var pid *ids.UUID // NULL for a link row of another entity type
		if err := r.Scan(&pid); err != nil {
			return ids.Nil, err
		}
		if pid == nil {
			return ids.Nil, nil
		}
		return *pid, nil
	})
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(displaced, func(contactID ids.UUID) bool { return contactID == ids.Nil }), nil
}

// Displaced participants merge by role, user, address and channel account so replacing several
// linked contacts cannot create duplicate participant rows.
func repointDisplacedParticipants(ctx context.Context, tx pgx.Tx, id ids.ActivityID, target ids.UUID, displaced []ids.UUID) error {
	query, args, targetPos, err := participantRelinkQuery(ctx, id, target, displaced)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, storekit.SQLf(`WITH planned AS (%s), promoted AS (
  UPDATE activity_participant ap SET contact_id = $%d
  FROM planned p WHERE ap.id = p.id AND p.move
  RETURNING ap.id
 ) DELETE FROM activity_participant
 WHERE id IN (SELECT id FROM planned WHERE NOT move)`, query, targetPos), args...)
	return err
}
