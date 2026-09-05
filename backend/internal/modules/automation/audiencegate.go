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
// Deliberately narrow, and narrow in a way worth stating because two nearby
// shapes are NOT covered:
//
//   - The gate reads ev.Entity, the trigger's subject, never an action's own
//     Target. Those can differ by design (gate.go's target-scoped arm says so),
//     and no shipped handler produces an activity target that differs from its
//     trigger today — people/leadrouting.go passes ev.Entity straight through.
//     TestNoHandlerTargetsAnActivityAwayFromItsTrigger fails when one appears.
//   - A signal carries activity-derived evidence with its own visibility
//     (platform/auth's SignalScopeClause), and a signal-subject firing skips
//     this gate. No human-owned signal workflow is registered
//     (compose/workflows.go), so nothing reaches it, and
//     TestNoCatalogTriggerCarriesASignalSubject fails when one is added.
//
// Every other subject's visibility follows row scope, which IS the owner's
// grants, so the object gate next door already resolves it. The activity is the
// exception: its audience is written on the row and moves without anybody's
// grants moving.
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
// A firing that names no activity is not this gate's business and proceeds.
//
// A system-seeded automation (no owner) is NOT exempt, and this is where that
// used to be got wrong. The object gate next door is right to skip an ownerless
// firing — "may this owner do this action" has no subject to ask about. This
// gate's question is about the ROW, and an ownerless automation still writes
// content derived from it: post_meeting_recap planned 468 drafts holding
// model-written summaries of mail held to its participants, and no gate here
// ran at all. So an ownerless firing takes the floor instead: it may derive
// from a row the whole workspace can read, and nothing else.
func checkOwnerCanReadSubject(ctx context.Context, db *database.DB, resolver authz.Resolver, ev workflow.Event) (gateDecision, error) {
	if ev.Entity.Type != activityEntity || ev.Entity.ID.IsZero() {
		return gateDecision{}, nil
	}
	if ev.OwnerID == ids.Nil {
		return checkOwnerlessSubject(ctx, db, ev)
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
	// The OBJECT grant first, then the row. This is the order the real read
	// path runs (activities/audience.go's GetActivityContent) and both halves
	// are load-bearing: EnsureActivityContentVisibleLive answers row and
	// audience scope, never whether this principal may read activities at all.
	//
	// The two come apart in practice. Every action in the catalog that touches
	// an activity requires activity.CREATE (catalog_actions.go), and a role's
	// CRUD verbs are independently editable — so an owner granted create but
	// not read cannot open the message through the API while their automation
	// would have fired on it.
	if err := auth.Require(ownerCtx, "activity", principal.ActionRead); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return gateDecision{
				blocked: true,
				reason:  "the automation's owner may not read messages at all",
			}, nil
		}
		return gateDecision{}, fmt.Errorf("automation: checking the owner's read grant: %w", err)
	}
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

// checkOwnerlessSubject decides what a firing with no human behind it may
// derive from, answered off the row rather than through a principal.
//
// There is no authority to resolve, so there is none to check — and the safe
// reading of that is the one this tree takes everywhere a principal is missing
// rather than denied: approvals' own decidable says it plainly, that a proposal
// nobody is recorded for is one nobody may read, not one everybody may.
//
// The floor is therefore the audience column itself. `workspace` is the only
// value that says every seat here may read this, which is the most an
// automation nobody owns can claim. `participants` and `selected` both name a
// set this firing is not in and cannot be checked against.
//
// Giving these automations a human owner at seed time was the other way to make
// the existing gate apply. It answers a different question: what the automation
// may read would become whatever one arbitrarily chosen seat may read, moving
// as that person's grants move, and the audit would record a human authority
// that never existed.
func checkOwnerlessSubject(ctx context.Context, db *database.DB, ev workflow.Event) (gateDecision, error) {
	var audience string
	err := db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT audience FROM activity WHERE id = $1`, ev.Entity.ID).Scan(&audience)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// A row that is not there is not one to derive from, and it reads as
		// absent rather than as a fault: an activity erased or archived between
		// the firing and this check is an ordinary race, not an outage.
		return gateDecision{blocked: true, reason: reasonOwnerlessUnreadable}, nil
	}
	if err != nil {
		// Infrastructure. Surfaced so the firing retries rather than recording
		// a terminal blocked row over a blip.
		return gateDecision{}, fmt.Errorf("automation: reading the subject's audience for an ownerless firing: %w", err)
	}
	if audience == audienceWorkspace {
		return gateDecision{}, nil
	}
	return gateDecision{blocked: true, reason: reasonOwnerlessHeld}, nil
}
