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
	// The relink target is a client-supplied reference (H1).
	if err := auth.EnsureLinkTarget(ctx, tx, in.EntityType, in.EntityID); err != nil {
		return false, false, err
	}
	return relinkActivityRow(ctx, tx, id, in, column)
}

// deleteVisibleLinksOfType drops the activity's links of one entity type and
// answers the contact ids that delete actually displaced. Those ids come from
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
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	idPos, typePos := arg(id), arg(entityType)
	scope, err := auth.ScopeClauseFor(ctx, entityType, "t", arg)
	if err != nil {
		return nil, err
	}
	visible := "true"
	if scope != "" {
		visible = scope
	}
	rows, err := tx.Query(ctx, storekit.SQLf(`
		DELETE FROM activity_link
		WHERE activity_id = $%d AND entity_type = $%d
		  AND EXISTS (SELECT 1 FROM %s t WHERE t.id = activity_link.%s AND %s)
		RETURNING contact_id`,
		idPos, typePos, entityType, column, visible), args...)
	if err != nil {
		return nil, err
	}
	// Every id this delete actually removed. A link row of another
	// entity type returns NULL here and contributes nothing.
	displaced, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (ids.UUID, error) {
		var pid *ids.UUID
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

// repointDisplacedParticipants moves the displaced contacts' participant rows
// onto the relink target. A relink to a CONTACT is a human saying "this
// conversation was actually with someone else", so the participant row naming
// the old contact is now wrong (ACT-DDL-3). Repointing it keeps the
// participants and the links telling one story.
//
// The DISPLACED contact carries the row scope too. The relink already gated the
// new target; without this the old one is rewritten sight unseen, so a caller
// could repoint a participant naming a contact they cannot read — including an
// owner-private captured one. The link delete scopes for the same reason; this
// is its participant twin.
//
// KNOWN GAP, stated rather than papered over: the graph consumer derives its
// affected (user, contact) pairs from the participant rows, and by the time it
// runs they name the NEW contact — so the OLD edge is not recomputed and keeps
// counting an interaction that no longer points at it. The nightly rebuild
// clears it, which bounds the staleness to the same 24h the window counts
// already carry, but it is a bound and not a fix.
//
// The fix is the additive `relinked_from` reference ADR-0078 specifies on the
// activity.updated relink payload: the consumer needs the displaced id, and
// this module cannot recompute the edge itself because search is a sibling.
// That is a public-event contract change and belongs in its own slice.
func repointDisplacedParticipants(ctx context.Context, tx pgx.Tx, id ids.ActivityID, target ids.UUID, displaced []ids.UUID) error {
	var pargs []any
	parg := func(v any) int { pargs = append(pargs, v); return len(pargs) }
	idPos, targetPos, displacedPos := parg(id), parg(target), parg(displaced)
	visible, err := auth.ScopeClauseFor(ctx, linkEntityContact, "op", parg)
	if err != nil {
		return err
	}
	if visible == "" {
		// An unbounded caller narrows nothing.
		visible = "true"
	}
	// One merge, not a conditional rewrite. The repoint used to UPDATE each
	// displaced row to the target and skip when the target was already a
	// participant, which was wrong in both directions:
	//
	//   - the skip left the displaced row naming the OLD contact, so the
	//     activity's links said one thing and its participants another —
	//     exactly what the repoint exists to prevent — and that row kept
	//     feeding a relationship-strength signal for somebody the human had
	//     just said the conversation was not with;
	//   - with several displaced participants and no target row, every one of
	//     them qualified and each was rewritten to the target, colliding on
	//     uq_activity_participant.
	//
	// The uniqueness is per (activity, role, user, contact, address), not per
	// contact, so both the skip test and the collision are decided by the whole
	// tuple. Within each such group exactly one displaced row is promoted to
	// the target — and only when the target holds no row of that shape
	// already — and the rest are deleted. Either way the target is named once
	// and no displaced row survives.
	const nilUUID = `'00000000-0000-0000-0000-000000000000'::uuid`
	if _, err := tx.Exec(ctx, storekit.SQLf(`
		WITH scoped AS (
			SELECT ap.id, ap.role, ap.user_id, ap.address,
			       row_number() OVER (
			           PARTITION BY ap.role, coalesce(ap.user_id, `+nilUUID+`), coalesce(ap.address, '')
			           ORDER BY ap.id) AS rank
			  FROM activity_participant ap
			 WHERE ap.activity_id = $%d
			   -- Exactly the contacts the link delete removed, and no
			   -- others. A participant can name somebody who was never
			   -- linked at all, and inferring the displaced set from "no
			   -- longer linked" would rewrite them too.
			   AND ap.contact_id = ANY($%d::uuid[])
			   AND ap.contact_id <> $%d
			   AND EXISTS (SELECT 1 FROM contact op WHERE op.id = ap.contact_id AND (`+visible+`))
		), promoted AS (
			UPDATE activity_participant ap SET contact_id = $%d
			  FROM scoped s
			 WHERE ap.id = s.id AND s.rank = 1
			   AND NOT EXISTS (
			       SELECT 1 FROM activity_participant other
			        WHERE other.activity_id = ap.activity_id
			          AND other.role = s.role
			          AND other.contact_id = $%d
			          AND coalesce(other.user_id, `+nilUUID+`) = coalesce(s.user_id, `+nilUUID+`)
			          AND coalesce(other.address, '') = coalesce(s.address, ''))
			RETURNING ap.id
		)
		DELETE FROM activity_participant
		 WHERE id IN (SELECT id FROM scoped)
		   AND id NOT IN (SELECT id FROM promoted)`,
		idPos, displacedPos, targetPos, targetPos, targetPos), pargs...); err != nil {
		return err
	}
	return nil
}
