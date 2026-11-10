// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// unfileDealProject takes back the project link capture copied from a deal
// the message was just declined on. It goes through the project-filing undo.
// A retention rule, a project the caller cannot see or an unnamed account
// keeps the link, and the answer says so.
func unfileDealProject(ctx context.Context, tx pgx.Tx, mail ids.ActivityID, deal ids.UUID, out *crmcontracts.ActivityDealDecline) error {
	derived, err := projectCameFromDeal(ctx, tx, mail, deal)
	if err != nil || !derived {
		return err
	}
	actor, err := admitDeclineUndoDecider(ctx)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		out.ProjectKept = true
		return nil
	}
	if err != nil {
		return err
	}
	var refused *ProjectFilingRefusedError
	var unnamed *DeciderUnnamedError
	err = undoProjectFilingLocked(ctx, tx, mail, actor, dealDeclineUndoReason)
	switch {
	case errors.As(err, &refused), errors.As(err, &unnamed):
		out.ProjectKept = true
		return nil
	case err != nil:
		return err
	}
	out.ProjectUnfiled = true
	return nil
}

// admitDeclineUndoDecider is admitUndoDecider widened for a deal decline: it
// also admits an agent working on a colleague's passport. The product owner
// lets an agent's decline carry the deal's own project link with it. The
// standalone undo stays a member's decision, so admitUndoDecider still refuses
// every agent there.
func admitDeclineUndoDecider(ctx context.Context) (principal.Principal, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || !actor.ActsAsItsUser() || actor.UserID == ids.Nil {
		return admitUndoDecider(ctx)
	}
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return principal.Principal{}, err
	}
	return actor, nil
}

// projectCameFromDeal answers whether the message's project link is the
// deal's own project, written by capture's deal rung, and never filed by a
// colleague. A link with no rung on record is kept, since nothing proves it
// came from the deal.
func projectCameFromDeal(ctx context.Context, tx pgx.Tx, mail ids.ActivityID, deal ids.UUID) (bool, error) {
	var args []any
	arg := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	mailArg, dealArg, rungArg := arg(mail), arg(deal), arg(provenance.ProjectRungDeal)
	var derived bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM activity_link l JOIN deal d ON d.project_id = l.project_id
		 WHERE l.activity_id = `+mailArg+` AND l.entity_type = 'project' AND d.id = `+dealArg+`
		   AND NOT `+provenance.HumanProjectFilingSQL(mailArg, "l.project_id")+`
		   AND EXISTS (SELECT 1 FROM audit_log p
		     WHERE p.entity_type = 'activity' AND p.entity_id = l.activity_id AND p.action = 'activity_relink'
		       AND p.after->>'entity_type' = 'project' AND p.after->>'entity_id' = l.project_id::text
		       AND p.actor_type IN ('connector', 'system')
		       AND p.evidence->>'`+provenance.ProjectRungKey+`' = `+rungArg+`))`, args...).Scan(&derived)
	if err != nil {
		return false, fmt.Errorf("activities: reading whether the project came from the deal: %w", err)
	}
	return derived, nil
}
