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
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/blobstore"
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

// Purge destroys the mail one exclusion rule matched.
//
// Two scopes, two authorities. A seat's OWN rule destroys what that seat
// imported, and the authority is the owner being the one asking. A WORKSPACE
// rule destroys what the workspace captured, across every seat, and takes the
// admin role — destroying every colleague's matching mail is the workspace's own
// act, not one seat's.
//
// Human-only either way. Destroying mail is not something a background pass does
// on somebody's behalf, which is why this takes a human principal even though
// the sibling personal-mail sweep runs unattended: that sweep acts on a verdict
// about one sender, and this acts on a rule somebody wrote.
//
// preview answers the same question and changes nothing, so the counts a caller
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
	rule, workspaceScoped, err := p.purgeableRule(ctx, exclusionID, actor.UserID)
	if err != nil {
		return PurgeOutcome{}, err
	}
	var subject capture.PurgeSubject
	if err := database.WithWorkspaceTx(ctx, p.pool, func(tx pgx.Tx) error {
		var err error
		if workspaceScoped {
			subject, err = capture.SelectWorkspacePurgeSubjectTx(ctx, tx, rule.Kind, rule.Value, statutoryFloor())
			return err
		}
		subject, err = capture.SelectPurgeSubjectTx(ctx, tx, actor.UserID, rule.Kind, rule.Value, statutoryFloor())
		return err
	}); err != nil {
		return PurgeOutcome{}, err
	}
	// Seat-scoped, and deliberately not run for a workspace rule.
	// SelectPurgeablePeopleTx answers "which people did THIS seat's capture
	// mint, that nothing else holds" — a question with no workspace-wide
	// analogue, because a contact every seat can see is by definition held by
	// more than the mail one rule matched. Anonymising workspace-wide is the
	// erasure lane's job and takes a subject request, not an exclusion rule.
	var people []ids.UUID
	if !workspaceScoped {
		if err := database.WithWorkspaceTx(ctx, p.pool, func(tx pgx.Tx) error {
			var err error
			people, err = capture.SelectPurgeablePeopleTx(ctx, tx, actor.UserID, rule.Kind, rule.Value)
			return err
		}); err != nil {
			return PurgeOutcome{}, err
		}
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
	reason := privacy.PurgeOwnerRule
	if workspaceScoped {
		reason = privacy.PurgeWorkspaceRule
	}
	if err := p.carryOut(ctx, subject, people, actor.UserID, reason); err != nil {
		return PurgeOutcome{}, err
	}
	return outcome, nil
}

// carryOut performs what a selected purge decided, in the one order that is
// survivable, and is the ONLY place that order is written.
//
// Both purge paths run it: an owner acting on their own exclusion rule, and the
// sweep that destroys personal mail once its window closes. They differ in what
// they select and why; what happens to a selected message is one answer, and a
// second copy of this sequence is how one path would quietly stop auditing a
// release or stop recomputing an audience.
//
// Held by TestBothPurgePathsShareOneExecutor (backend/gates/purgeexecutor_test.go).
func (p *CapturePurger) carryOut(
	ctx context.Context, subject capture.PurgeSubject, people []ids.UUID,
	seat ids.UUID, reason privacy.PurgeReason,
) error {
	// The PEOPLE first, while their mail still exists to identify them by.
	// SelectPurgeablePeopleTx matches a person through the activities this seat
	// imported, so destroying the mail first would leave nothing to select them
	// with and the contacts would survive a purge that reported anonymising
	// them.
	//
	// Skipped outright when there are none, rather than called with an empty
	// slice: AnonymisePeople takes the person/delete grant before it looks at
	// its argument, and the personal sweep runs under a system principal that
	// passes that check by BYPASSING it. A caller with no people to anonymise
	// should not be asking for the grant at all.
	if len(people) > 0 {
		if _, err := p.retention.AnonymisePeople(ctx, people, reason); err != nil {
			return err
		}
	}
	// Destruction FIRST, release second. A crash between them leaves messages
	// destroyed and a colleague's shared ones still claimed, which the next run
	// finishes; the other order would release a claim and then fail to destroy,
	// leaving mail the owner believes is gone with nobody's name on it.
	if _, err := p.retention.PurgeActivities(ctx, subject.SoleImports, reason); err != nil {
		return err
	}
	// Every claim on a DESTROYED message goes with it. PurgeActivities empties
	// the activity and leaves capture_import and activity_participant pointing
	// at it, which for the seat arm is right — the row it destroys is one only
	// that seat imported, and its own claim goes below with the release. For a
	// workspace purge it is not: the message is destroyed for everyone, so a
	// surviving claim is a stub on a colleague's timeline for mail the workspace
	// was told is gone, that nothing would ever collect.
	if reason == privacy.PurgeWorkspaceRule {
		for _, id := range subject.SoleImports {
			if err := database.WithWorkspaceTx(ctx, p.pool, func(tx pgx.Tx) error {
				return capture.ReleaseEveryImportTx(ctx, tx, id)
			}); err != nil {
				return err
			}
		}
	}
	for _, id := range subject.SharedImports {
		if err := database.WithWorkspaceTx(ctx, p.pool, func(tx pgx.Tx) error {
			if err := capture.ReleaseImportTx(ctx, tx, id, seat); err != nil {
				return err
			}
			// The write shape, on the arm that is easiest to forget: this
			// removes a seat's ACCESS to a message — the import row is their
			// hold on it and the participant row is what makes it readable — and
			// without an audit row the question "who could read this, and when
			// did that change" has no answer for exactly the messages somebody
			// asked to be rid of.
			if _, err := storekit.Audit(ctx, tx, "archive", "activity", id, nil, map[string]any{
				"purge_reason": string(reason), "released_by": seat.String(),
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
			return err
		}
	}
	return nil
}

// personalSweepBatch bounds one pass, matching noiseSweepBatch: the backlog is a
// query, so what a pass does not reach this tick it reaches the next.
const personalSweepBatch = 500

// SweepPersonalMail destroys the personal correspondence whose undo window has
// closed, for every seat in the workspace that has any.
//
// This is the automatic half of the `personal` verdict, and it is the only place
// in the product where a classification leads to irreversible deletion with no
// human in the loop at the moment it happens. Three things make that survivable,
// and removing any one of them breaks it:
//
//   - SelectPersonalPurgeTx measures the window PER MESSAGE and refuses any
//     address carrying a live `business` override, which is how a person cancels.
//   - A verdict the classifier reached alone waits four times as long as one a
//     person reached, because nobody has looked at it.
//   - The statutory floor is applied as a shield, so correspondence the law
//     requires kept is reported rather than destroyed.
//
// It reuses carryOut, so a message destroyed here goes through exactly the
// sequence an owner's own purge uses.
func (p *CapturePurger) SweepPersonalMail(ctx context.Context, windows capture.PersonalPurgeWindows) (int, error) {
	var seats []ids.UUID
	if err := database.WithWorkspaceTx(ctx, p.pool, func(tx pgx.Tx) error {
		var err error
		seats, err = capture.SeatsWithPersonalMailDueTx(ctx, tx, windows, personalSweepBatch)
		return err
	}); err != nil {
		return 0, fmt.Errorf("verdict: finding seats with personal mail due: %w", err)
	}
	destroyed := 0
	for _, seat := range seats {
		var subject capture.PurgeSubject
		if err := database.WithWorkspaceTx(ctx, p.pool, func(tx pgx.Tx) error {
			var err error
			subject, err = capture.SelectPersonalPurgeTx(ctx, tx, seat, windows, statutoryFloor(), personalSweepBatch)
			return err
		}); err != nil {
			return destroyed, fmt.Errorf("verdict: selecting personal mail for one seat: %w", err)
		}
		// No people are anonymised here. A `personal` verdict creates NO
		// counterparty record in the first place — captureverdict's KindPersonal
		// arm returns without one — so there is nothing this sweep could match,
		// and passing a selector that looked for one would either find a person
		// somebody made for another reason or find nothing while implying it had
		// looked.
		if err := p.carryOut(ctx, subject, nil, seat, privacy.PurgePersonalVerdict); err != nil {
			return destroyed, fmt.Errorf("verdict: destroying personal mail: %w", err)
		}
		destroyed += len(subject.SoleImports)
	}
	return destroyed, nil
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

// purgeableRule reads the rule this caller may act on, and reports whether it is
// the workspace's rather than their own.
//
// Two authorities, one lookup, because a purge names a rule by id and an id is a
// guess anybody can make. Reading it back scoped to what the caller may reach is
// what stops one seat destroying the mail another seat's rule matched.
//
// A WORKSPACE rule takes the admin role, gated directly rather than inferred
// from being able to create one. Creating a workspace exclusion takes
// `capture_settings.update`, which admin AND ops both hold, so inferring the
// authority from the creation grant would hand workspace-wide destruction to an
// ops seat. Destroying every colleague's matching mail is the workspace's own
// act, and only an admin speaks for the workspace.
//
// A USER rule is scoped on user_id, NOT created_by: created_by is the provenance
// string every capture row carries (`human:<uuid>`, `connector:gmail`), and
// matching a uuid against it would never succeed — the check would refuse every
// purge, which looks like a permissions bug rather than the wrong column.
func (p *CapturePurger) purgeableRule(ctx context.Context, id, user ids.UUID) (capture.Exclusion, bool, error) {
	var rule capture.Exclusion
	var scope string
	err := database.WithWorkspaceTx(ctx, p.pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT id, kind, value, scope FROM capture_exclusion
			 WHERE id = $1 AND (scope = $3 OR (scope = $4 AND user_id = $2))`,
			id, user, capture.ExclusionScopeWorkspace, capture.ExclusionScopeUser).
			Scan(&rule.ID, &rule.Kind, &rule.Value, &scope)
	})
	if err != nil {
		return capture.Exclusion{}, false, noSuchRule()
	}
	if scope != capture.ExclusionScopeWorkspace {
		return rule, false, nil
	}
	// A non-admin naming a real workspace rule gets the SAME answer as one
	// naming nothing. Distinguishing them turns this endpoint into an existence
	// oracle: a rep probing ids would read 403 as "a workspace rule with this id
	// exists" and 404 as "it does not", which is how one seat learns what the
	// workspace has decided to keep out.
	if err := auth.RequireAdmin(ctx); err != nil {
		return capture.Exclusion{}, false, noSuchRule()
	}
	return rule, true, nil
}

// noSuchRule is the one answer a caller gets for a rule that does not exist, one
// that is not theirs, and one they may not act on.
//
// ErrNotFound rather than a bare error, because the bare one wraps no sentinel:
// httperr cannot classify it and answers 500, which the contract does not
// declare and which is itself distinguishable from every other outcome.
func noSuchRule() error {
	return fmt.Errorf("capture purge: no such rule of yours: %w", apperrors.ErrNotFound)
}

// capturePurgerFor builds the purge a background pass uses, or nil when the role
// has no object store.
//
// Nil is the honest answer rather than a degraded one: a purge that destroyed
// the rows naming an attachment and left its bytes in the bucket would report
// mail as gone while it is not, so a role that cannot reach the blobs does not
// purge at all.
func capturePurgerFor(pool *pgxpool.Pool, blob blobstore.Store, log *slog.Logger) *CapturePurger {
	if blob == nil {
		return nil
	}
	return NewCapturePurger(pool, NewRetentionServiceFor(InstallationDB(pool), blob, log))
}
