// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

// The match-time AUDIENCE gate: may the automation's owner read the record
// this firing is about?
//
// gate.go answers a different question and cannot answer this one. It asks
// whether the owner's RBAC permits the ACTION — an object question, resolved
// from grants alone, deliberately DB-free. Audience is a property of the ROW,
// and no grant carries it: a colleague with full activity.update permission
// still may not read a message limited to its participants.
//
// The gap that leaves is not theoretical. Capture emits activity.captured and
// engagement.reply for a newly captured activity whatever its audience
// (capture/sinkactivity.go, capture/sinkreply.go), engagement.reply is a
// configurable trigger (catalog_triggers.go), and every action in the closed
// catalog writes something a colleague can see — a task, a draft, an approval,
// a list membership, a notification. So a rep's automation could raise a task
// about mail they may not read, naming the contact and the moment.
//
// Two async peers in this tree already re-check live state at fire time rather
// than trusting the moment they were queued: scheduled sends rerun their
// preparation gates (activities/scheduledsendfire.go) and the approvals inbox
// re-probes its targets (approvals/targetvisibility.go). This is the third.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// activityEntity is the one entity type whose visibility this gate can answer.
//
// Deliberately narrow. Every other subject a firing names is already covered by
// the object gate next door, because their visibility follows row scope, which
// IS the owner's grants. The activity is the exception: its audience is written
// on the row and moves without anybody's grants moving, which is exactly why
// the object gate cannot see it.
const activityEntity = datasource.EntityType("activity")

// checkOwnerCanReadSubject refuses a firing whose subject the automation's
// owner may not read.
//
// It runs under the OWNER's principal, built here from their live RBAC, never
// under the engine's. The engine binds PrincipalSystem for attribution, and
// auth.ActivityAudienceArm answers TRUE for a system principal — so a check
// that inherited the caller's context would pass for every firing and look
// exactly like a working gate.
//
// A firing that names no activity is not this gate's business and proceeds. A
// system-seeded automation (no owner) likewise: there is no human authority
// behind it to check, the same reasoning the object gate applies to a zero
// OwnerID.
func checkOwnerCanReadSubject(ctx context.Context, db *database.DB, resolver authz.Resolver, ev workflow.Event) (gateDecision, error) {
	if ev.Entity.Type != activityEntity || ev.Entity.ID.IsZero() {
		return gateDecision{}, nil
	}
	if ev.OwnerID == ids.Nil {
		return gateDecision{}, nil
	}
	if resolver == nil {
		// A composed engine always carries a resolver (compose/workflows.go);
		// reaching here is a wiring bug, not a permission question — and it
		// must not be swallowed into a false "allow".
		return gateDecision{}, errors.New("automation: audience gate composed with no authz.Resolver")
	}
	rbac, err := resolver.EffectiveRBAC(ctx, ev.WorkspaceID, ev.OwnerID)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			// A gone, archived or suspended owner is a real denial, not an
			// outage — the same honest hard case the object gate handles.
			return gateDecision{blocked: true, reason: reasonOwnerGone}, nil
		}
		return gateDecision{}, fmt.Errorf("automation: resolving the owner's live authority: %w", err)
	}
	ownerCtx := principal.WithActor(ctx, principal.Principal{
		Type:        principal.PrincipalHuman,
		ID:          "human:" + ev.OwnerID.String(),
		UserID:      ev.OwnerID,
		TeamIDs:     rbac.TeamIDs,
		Permissions: rbac.Permissions,
	})
	err = db.Tx(ctx, func(tx pgx.Tx) error {
		return auth.EnsureActivityContentVisibleLive(ownerCtx, tx, ev.Entity.ID)
	})
	switch {
	case err == nil:
		return gateDecision{}, nil
	case errors.Is(err, apperrors.ErrNotFound), errors.Is(err, apperrors.ErrPermissionDenied):
		// Both mean the same thing here and are deliberately one answer: a row
		// outside the owner's audience reads as absent, which is how existence
		// stays hidden. Neither is an error to retry.
		return gateDecision{
			blocked: true,
			reason:  "the automation's owner can no longer read the message this fired on",
		}, nil
	default:
		// Anything else is infrastructure. Surfaced so the firing retries
		// rather than claiming a terminal blocked row over a blip.
		return gateDecision{}, fmt.Errorf("automation: re-checking the subject's audience: %w", err)
	}
}
