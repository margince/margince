// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

// The audience gate asked the other way round: not "may this caller read the
// row" but "may anybody". An audience write owes that question, because a write
// that leaves the answer no has not narrowed the message — it has deleted it
// from view, and irreversibly.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ActivityHasAReaderTx answers whether ANY human is admitted to this
// activity's content as the row now stands.
//
// It is activityAudienceArm's existential twin: that one asks whether ONE named
// caller matches an arm, this one asks whether anybody does. An audience write
// that leaves the answer false has not narrowed the message, it has deleted it
// from view — and irreversibly, because widening it back needs the content
// visibility the write destroyed.
//
// WHY IT IS A SECOND SPELLING RATHER THAN A SHARED CLAUSE. The two questions
// bind the user differently, and that changes each arm's SQL shape rather than
// only its operand: `captured_by LIKE '%:<me>'` becomes a join against every
// live user, `ci.user_id = $me` becomes a bare EXISTS, and a `selected`
// membership stops being "is it me" and becomes "does this subject name anybody
// at all" — a team names its members, and an empty team names none.
// A parameterised single spelling would have to carry all three shapes and
// would read as neither. What holds the pair together instead is a test that
// builds a row admitted by each arm alone and asserts the two functions agree
// about it. It has to fail in BOTH directions: a refusal that missed an arm
// refuses a legal write, and one that kept an arm the read clause has dropped
// admits the orphan silently, which is the half no assertion would notice.
//
// Held by: TestTheOrphanRefusalAgreesWithTheAudienceGateArmForArm
// (backend/internal/compose/integration/audienceorphanarms_integration_test.go)
//
// WHERE THE MIRROR STOPS, stated once so the two halves below are one rule and
// not two exceptions.
//
// What is mirrored is the ARM — including any input the read side computes
// OUTSIDE the SQL. Team ids are the case: identity's loadGrants joins
// `team ... archived_at IS NULL` before a team id ever reaches a principal, so
// the read arm's `am.subject_id = ANY($teams)` can never match an archived team
// however many membership rows it keeps. Counting those rows here would answer
// "somebody can read it" about a team nobody is currently in — the orphan
// admitted rather than refused, which is the one direction of drift no
// assertion downstream would catch. So the join carries that filter too.
//
// What is NOT mirrored is anything that gates every read alike rather than this
// arm. Authentication is the one that matters: an archived account's uuid still
// sitting in captured_by counts as a reader here, exactly as it does in the read
// clause, and that seat cannot sign in to use it. Filtering it only here would
// make the refusal stricter than the gate it mirrors, with no fitness function
// behind the difference — and `status = 'active' AND archived_at IS NULL` has an
// owner already (identity.LiveMemberSQL, which this package may not import), so
// a third spelling would be the duplication this file exists to argue against.
// Whether a departed seat's provenance should keep admitting a row at all is one
// question about the READ clause, and it is not this one.
//
// It answers the AUDIENCE question alone. Reading content is
// `discover AND audience`, and row scope is the other half: a named reader whose
// scope reaches none of the activity's links still cannot open it. That is a
// separate axis which a later scope change can flip either way, so the refusal
// does not pretend to it — "nobody can read this" is proven here only in the
// sense the audience column decides.
func ActivityHasAReaderTx(ctx context.Context, tx pgx.Tx, id ids.UUID) (bool, error) {
	var reachable bool
	if err := tx.QueryRow(ctx, `
		SELECT a.audience = 'workspace'
		    OR EXISTS (SELECT 1 FROM app_user u WHERE a.captured_by LIKE '%:' || u.id)
		    OR EXISTS (SELECT 1 FROM capture_import ci WHERE ci.activity_id = a.id)
		    OR EXISTS (SELECT 1 FROM activity_participant ap
		                WHERE ap.activity_id = a.id AND ap.user_id IS NOT NULL)
		    OR (a.audience = 'selected' AND EXISTS (
		          SELECT 1 FROM activity_audience_member am
		           WHERE am.activity_id = a.id
		             AND (am.subject_type = 'user'
		               OR (am.subject_type = 'team' AND EXISTS (
		                     SELECT 1 FROM team_membership tm
		                       JOIN team t ON t.id = tm.team_id AND t.archived_at IS NULL
		                      WHERE tm.team_id = am.subject_id)))))
		  FROM activity a WHERE a.id = $1`, id).Scan(&reachable); err != nil {
		return false, fmt.Errorf("auth: reading whether an activity still has a reader: %w", err)
	}
	return reachable, nil
}
