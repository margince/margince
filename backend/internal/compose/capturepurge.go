// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The seam that turns an owner's exclusion rule into destroyed mail.
//
// Capture knows WHICH messages its own connections brought in; privacy knows
// HOW to destroy a message and everything it left behind. Neither may import
// the other, and neither should learn the other's half — a capture that grew
// its own destruction would be a second erasure path, and a privacy that grew
// its own import-row selection would be a second answer to "whose mail is
// this".
//
// So the purge is assembled here: capture selects, privacy destroys, and this
// file owns the one decision that belongs to neither — that a message a
// colleague also imported is released rather than destroyed.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// PurgeOutcome is what a purge did, or what a preview says it would do.
type PurgeOutcome struct {
	// Destroyed is how many messages are gone: text, provider original,
	// attachments, vectors, delivery copies, everything derived.
	Destroyed int `json:"destroyed"`
	// Released is how many messages a colleague also imported. This seat's
	// claim on them is gone; the message stayed.
	Released int `json:"released"`
	// Skipped is how many are under a statutory hold or an open erasure
	// request. Reported rather than silently passed over: an owner told their
	// mail is gone must not find it still there.
	Skipped int `json:"skipped"`
	// Anonymised is how many contacts the purge stripped: those this seat's
	// mail is the only reason the CRM knows, whom nothing else holds.
	Anonymised int `json:"anonymised"`
	// Preview reports that nothing was actually done.
	Preview bool `json:"preview"`
}

// CapturePurger destroys what one seat's exclusion rule matched.
type CapturePurger struct {
	pool      *pgxpool.Pool
	retention *privacy.RetentionService
}

// NewCapturePurger assembles the purge over one database.
func NewCapturePurger(pool *pgxpool.Pool, retention *privacy.RetentionService) *CapturePurger {
	return &CapturePurger{pool: pool, retention: retention}
}

// Purge destroys the mail one exclusion rule matched, for the CALLING seat.
//
// Own-rule only, and human-only. Destroying mail is not something a background
// pass or a colleague does on somebody's behalf: the rule is the owner's, the
// mailbox is the owner's, and the authority is the owner being the one asking.
//
// preview answers the same question and changes nothing, so the counts an owner
// is shown before they confirm are the counts they get.
func (p *CapturePurger) Purge(ctx context.Context, exclusionID ids.UUID, preview bool) (PurgeOutcome, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman || actor.UserID == ids.Nil {
		return PurgeOutcome{}, fmt.Errorf("capture purge: destroying mail is a person's own act")
	}
	// A read seat is licensed to look, not to destroy. The object grants inside
	// privacy answer what this caller may do to an activity or a person; the
	// seat tier answers whether they may change anything at all, and the two
	// are different questions — a read seat can hold grants and still not be
	// somebody who mutates.
	if !actor.SeatType.CanMutate() {
		return PurgeOutcome{}, apperrors.ErrPermissionDenied
	}
	rule, err := p.ownRule(ctx, exclusionID, actor.UserID)
	if err != nil {
		return PurgeOutcome{}, err
	}
	var subject capture.PurgeSubject
	if err := database.WithWorkspaceTx(ctx, p.pool, func(tx pgx.Tx) error {
		var err error
		subject, err = capture.SelectPurgeSubjectTx(ctx, tx, actor.UserID, rule.Kind, rule.Value, statutoryFloor())
		return err
	}); err != nil {
		return PurgeOutcome{}, err
	}
	var people []ids.UUID
	if err := database.WithWorkspaceTx(ctx, p.pool, func(tx pgx.Tx) error {
		var err error
		people, err = capture.SelectPurgeablePeopleTx(ctx, tx, actor.UserID, rule.Kind, rule.Value)
		return err
	}); err != nil {
		return PurgeOutcome{}, err
	}
	outcome := PurgeOutcome{
		Destroyed:  len(subject.SoleImports),
		Released:   len(subject.SharedImports),
		Skipped:    len(subject.Restricted),
		Anonymised: len(people),
		Preview:    preview,
	}
	if preview {
		return outcome, nil
	}
	// The PEOPLE first, while their mail still exists to identify them by.
	// SelectPurgeablePeopleTx matches a person through the activities this seat
	// imported, so destroying the mail first would leave nothing to select them
	// with and the contacts would survive a purge that reported anonymising
	// them.
	if _, err := p.retention.AnonymisePeople(ctx, people, privacy.PurgeOwnerRule); err != nil {
		return PurgeOutcome{}, err
	}
	// Destruction FIRST, release second. A crash between them leaves messages
	// destroyed and a colleague's shared ones still claimed, which the next run
	// finishes; the other order would release a claim and then fail to destroy,
	// leaving mail the owner believes is gone with nobody's name on it.
	if _, err := p.retention.PurgeActivities(ctx, subject.SoleImports, privacy.PurgeOwnerRule); err != nil {
		return PurgeOutcome{}, err
	}
	for _, id := range subject.SharedImports {
		if err := database.WithWorkspaceTx(ctx, p.pool, func(tx pgx.Tx) error {
			if err := capture.ReleaseImportTx(ctx, tx, id, actor.UserID); err != nil {
				return err
			}
			// The write shape, on the arm that is easiest to forget: this
			// removes a seat's ACCESS to a message — the import row is their
			// hold on it and the participant row is what makes it readable — and
			// without an audit row the question "who could read this, and when
			// did that change" has no answer for exactly the messages somebody
			// asked to be rid of.
			if _, err := storekit.Audit(ctx, tx, "archive", "activity", id, nil, map[string]any{
				"purge_reason": string(privacy.PurgeOwnerRule), "released_by": actor.UserID.String(),
			}); err != nil {
				return err
			}
			// No event of its own. The recompute below re-derives this
			// message's audience across its remaining importers and emits
			// activity.updated when that changes, which is the observable
			// consequence of the release — a second event announcing the same
			// change would be two answers to one question on the bus.
			// The message's audience is derived across its importers, so
			// dropping one changes what the rest ask for. Recomputing here is
			// what stops a released message keeping a hold its only remaining
			// reason has just left.
			return activities.RecomputeAudienceTx(ctx, tx, ids.From[ids.ActivityKind](id))
		}); err != nil {
			return PurgeOutcome{}, err
		}
	}
	return outcome, nil
}

// statutoryFloor hands capture the shield privacy spells once.
//
// Commercial correspondence inside its legal retention window is not the
// owner's to destroy: the nightly evaluator refuses to touch it for six years,
// and a purge that ignored the floor would be the one destructive path in the
// tree that bypasses it. Capture cannot import privacy, so the predicate
// travels rather than being written a second time — and a second copy is
// exactly how one path stops shielding what the others do.
func statutoryFloor() capture.StatutoryFloor {
	interval, anchor := privacy.StatutoryFloorArgs()
	return capture.StatutoryFloor{
		// The predicate privacy hands out is the NEGATED form — it filters a
		// destructive statement down to rows the law permits destroying. The
		// purge needs the positive question ("must this be kept?"), so it is
		// negated back here, where both halves are visible in one place.
		Clause:   func(intervalArg, anchorArg int) string { return privacy.StatutoryFloorShield(intervalArg, anchorArg) },
		Interval: interval,
		Anchor:   anchor,
	}
}

// ownRule reads the exclusion rule and refuses one that is not this seat's.
//
// A purge names a rule by id, and an id is a guess anybody can make. Reading it
// back scoped to the caller is what stops one seat destroying the mail another
// seat”'s rule matched.
//
// Scoped on user_id, NOT created_by: created_by is the provenance string every
// capture row carries (`human:<uuid>`, `connector:gmail`), and matching a uuid
// against it would never succeed — the check would refuse every purge, which
// looks like a permissions bug rather than the wrong column. A workspace-scoped
// rule is refused here too: it belongs to the workspace, and destroying every
// colleague”'s matching mail is not one seat”'s act.
func (p *CapturePurger) ownRule(ctx context.Context, id, user ids.UUID) (capture.Exclusion, error) {
	var rule capture.Exclusion
	err := database.WithWorkspaceTx(ctx, p.pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT id, kind, value FROM capture_exclusion
			 WHERE id = $1 AND scope = 'user' AND user_id = $2`,
			id, user).Scan(&rule.ID, &rule.Kind, &rule.Value)
	})
	if err != nil {
		// Not found and not yours are one answer, so an id probe learns nothing
		// about whether a rule exists.
		return capture.Exclusion{}, fmt.Errorf("capture purge: no such rule of yours")
	}
	return rule, nil
}
