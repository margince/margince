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
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/capturepurgebind"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
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
	// request. They are reported so that "gone" can be trusted.
	Skipped int `json:"skipped"`
	// Anonymised is how many contacts the purge stripped: those this seat's
	// mail is the only reason the CRM knows, whom nothing else holds.
	Anonymised int `json:"anonymised"`
	// Preview reports that nothing was actually done.
	Preview bool `json:"preview"`
	// Kept says why the skipped messages were skipped: a hold lifts when
	// somebody lifts it, a statutory window on a date, a request when closed.
	Kept KeptBreakdown `json:"kept"`
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
// A seat's own rule destroys what that seat imported, and the owner asking is
// the authority. A workspace rule destroys what the workspace captured across
// every seat and takes the admin role.
//
// Human-only either way: destroying mail is not something a background pass does
// on somebody's behalf, although the personal-mail sweep runs unattended. That
// sweep acts on a verdict about one sender; this acts on a rule somebody wrote.
//
// preview changes nothing, so the counts a caller is shown before they confirm
// are the counts they get. Counts confirmed on the context (capturepurgebind)
// bind the destroy to that preview.
func (p *CapturePurger) Purge(ctx context.Context, exclusionID ids.UUID, preview bool) (PurgeOutcome, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman || actor.UserID == ids.Nil {
		return PurgeOutcome{}, errors.New("capture purge: destroying mail is a contact's own act")
	}
	// A read seat is licensed to look, not to destroy. The object grants inside
	// privacy answer what this caller may do to an activity or a contact; the
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
	var policy privacy.FloorPolicy
	if err := database.WithWorkspaceTx(ctx, p.pool, func(tx pgx.Tx) error {
		floor, resolved, err := purgeFloorTx(ctx, tx)
		if err != nil {
			return err
		}
		if workspaceScoped {
			subject, err = capture.SelectWorkspacePurgeSubjectTx(ctx, tx, rule.Kind, rule.Value, floor)
		} else {
			subject, err = capture.SelectPurgeSubjectTx(ctx, tx, actor.UserID, rule.Kind, rule.Value, floor)
		}
		if err != nil {
			return err
		}
		// The receipt names the floor of the entities whose mail it kept.
		policy, err = resolved.ForMessagesTx(ctx, tx, subject.UnderStatute)
		return err
	}); err != nil {
		return PurgeOutcome{}, err
	}
	// Seat-scoped only: a contact every seat can see is held by more than one
	// rule's mail, and anonymising workspace-wide is the erasure lane's job.
	var minted []ids.UUID
	if !workspaceScoped {
		if minted, err = p.purgeableContacts(ctx, actor.UserID, rule); err != nil {
			return PurgeOutcome{}, err
		}
	}
	outcome := PurgeOutcome{
		Destroyed:  len(subject.SoleImports),
		Released:   len(subject.SharedImports),
		Skipped:    len(subject.Restricted),
		Anonymised: len(minted),
		Preview:    preview,
		Kept:       keptBreakdown(subject, policy),
	}
	// After the selection, because the contact grant depends on whether it found
	// contacts to anonymise.
	if err := p.retention.CheckPurgeAuthority(ctx, len(minted) > 0); err != nil {
		return PurgeOutcome{}, err
	}
	if preview {
		return outcome, nil
	}
	if err := capturepurgebind.Check(ctx, outcome.Destroyed, outcome.Released, outcome.Anonymised); err != nil {
		return PurgeOutcome{}, err
	}
	return p.destroy(ctx, exclusionID, actor.UserID, rule, workspaceScoped, subject, minted, outcome)
}

// destroy runs the cascade for a selection the caller confirmed and then
// writes the receipt.
func (p *CapturePurger) destroy(ctx context.Context, exclusionID, seat ids.UUID, rule capture.Exclusion, workspaceScoped bool, subject capture.PurgeSubject, minted []ids.UUID, outcome PurgeOutcome) (PurgeOutcome, error) {
	reason := privacy.PurgeOwnerRule
	if workspaceScoped {
		reason = privacy.PurgeWorkspaceRule
	}
	anonymised, err := p.carryOut(ctx, subject, minted, p.stillPurgeable(seat, rule), seat, reason)
	if err != nil {
		return PurgeOutcome{}, err
	}
	outcome.Anonymised = anonymised // the recheck inside the cascade can skip some
	// After the cascade: carryOut commits per item, so a receipt written first
	// would certify a plan and not an act. If this write fails, the
	// per-activity rows still hold the trail and the caller is told it failed.
	if err := p.auditPurgeReceipt(ctx, exclusionID, outcome); err != nil {
		return PurgeOutcome{}, err
	}
	return outcome, nil
}

// purgeableContacts is what a seat's rule purge anonymises. Only this seat's
// mail explains them, capture made them for this seat, and nobody worked on
// them since. A contact somebody typed in is not this mailbox's to erase.
func (p *CapturePurger) purgeableContacts(ctx context.Context, seat ids.UUID, rule capture.Exclusion) (minted []ids.UUID, err error) {
	err = database.WithWorkspaceTx(ctx, p.pool, func(tx pgx.Tx) error {
		minted, err = purgeableContactsTx(ctx, tx, seat, rule)
		return err
	})
	return minted, err
}

func purgeableContactsTx(ctx context.Context, tx pgx.Tx, seat ids.UUID, rule capture.Exclusion) ([]ids.UUID, error) {
	floor, _, err := purgeFloorTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	mailboxOnly, err := capture.SelectPurgeableContactsTx(ctx, tx, seat, rule.Kind, rule.Value, floor)
	if err != nil {
		return nil, err
	}
	return contacts.CaptureMintedForSeatTx(ctx, tx, seat, mailboxOnly)
}

// stillPurgeable asks the same question again for one contact, inside the
// transaction that anonymises it; only that contact pays the capture-origin check.
func (p *CapturePurger) stillPurgeable(seat ids.UUID, rule capture.Exclusion) privacy.ContactStillPurgeable {
	return func(ctx context.Context, tx pgx.Tx, id ids.UUID) (bool, error) {
		floor, _, err := purgeFloorTx(ctx, tx)
		if err != nil {
			return false, err
		}
		mailboxOnly, err := capture.SelectPurgeableContactsTx(ctx, tx, seat, rule.Kind, rule.Value, floor)
		if err != nil || !slices.Contains(mailboxOnly, id) {
			return false, err
		}
		minted, err := contacts.CaptureMintedForSeatTx(ctx, tx, seat, []ids.UUID{id})
		return slices.Contains(minted, id), err
	}
}

// Anonymise contacts before destroying the mail that identifies them, and keep
// shared-import release on the same audited path for every purge.
func (p *CapturePurger) carryOut(
	ctx context.Context, subject capture.PurgeSubject,
	minted []ids.UUID, stillPurgeable privacy.ContactStillPurgeable,
	seat ids.UUID, reason privacy.PurgeReason,
) (int, error) {
	// The CONTACTS first, while their mail still exists to identify them by.
	// SelectPurgeableContactsTx matches a contact through the activities this seat
	// imported, so destroying the mail first would leave nothing to select them
	// with and the contacts would survive a purge that reported anonymising
	// them.
	//
	// Skipped outright when there are none, rather than called with an empty
	// slice: AnonymiseContacts takes the contact/delete grant before it looks at
	// its argument, and the personal sweep runs under a system principal that
	// passes that check by BYPASSING it. A caller with no contacts to anonymise
	// should not be asking for the grant at all.
	anonymised := 0
	if len(minted) > 0 {
		var err error
		if anonymised, err = p.retention.AnonymiseContacts(ctx, minted, reason, stillPurgeable); err != nil {
			return anonymised, err
		}
	}
	// Destruction FIRST, release second. A crash between them leaves messages
	// destroyed and a colleague's shared ones still claimed, which the next run
	// finishes; the other order would release a claim and then fail to destroy,
	// leaving mail the owner believes is gone with nobody's name on it.
	if _, err := p.retention.PurgeActivities(ctx, subject.SoleImports, reason); err != nil {
		return anonymised, err
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
				return anonymised, err
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
			return anonymised, err
		}
	}
	return anonymised, nil
}

// personalSweepBatch bounds one pass, matching noiseSweepBatch: the backlog is a
// query, so what a pass does not reach this tick it reaches the next.
const personalSweepBatch = 500

// personalSweepActor binds the actor and correlation id the purge refuses to
// run without. The fleet job's context carries only a workspace.
const personalSweepActor = "system:capture-personal-purge"

// SweepPersonalMail destroys the personal correspondence whose undo window has
// closed, for every seat in the workspace that has any. It is the only place
// where a classification leads to irreversible deletion with nobody in the loop.
// Three guards make that survivable, and removing any one of them breaks it:
//
//   - SelectPersonalPurgeTx measures the window per message and refuses any
//     address carrying a live `business` override, which is how a contact cancels.
//   - A verdict the classifier reached alone waits four times as long as one a
//     contact reached, because nobody has looked at it.
//   - The statutory floor shields what the law requires kept, and reports it.
//
// It reuses carryOut, so a message destroyed here goes through the same
// sequence an owner's own purge uses.
func (p *CapturePurger) SweepPersonalMail(ctx context.Context, windows capture.PersonalPurgeWindows) (int, error) {
	ctx = principal.SystemActing(ctx, personalSweepActor)
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
		outcome, err := p.sweepSeat(ctx, seat, windows)
		if err != nil {
			return destroyed, err
		}
		destroyed += outcome.Destroyed
	}
	return destroyed, nil
}

// sweepSeat destroys one seat's due personal mail and writes the receipt for the
// pass. A pass that destroyed nothing writes none, so a seat whose due mail the
// shields keep does not add an audit row per tick.
//
// The kept numbers are counted over every due message of the seat, apart from
// the batch the destroy reached.
func (p *CapturePurger) sweepSeat(ctx context.Context, seat ids.UUID, windows capture.PersonalPurgeWindows) (PurgeOutcome, error) {
	var subject capture.PurgeSubject
	var kept capture.KeptCounts
	if err := database.WithWorkspaceTx(ctx, p.pool, func(tx pgx.Tx) error {
		floor, _, err := purgeFloorTx(ctx, tx)
		if err != nil {
			return err
		}
		if subject, err = capture.SelectPersonalPurgeTx(ctx, tx, seat, windows, floor, personalSweepBatch); err != nil {
			return err
		}
		kept, err = capture.CountPersonalKeptTx(ctx, tx, seat, windows, floor)
		return err
	}); err != nil {
		return PurgeOutcome{}, fmt.Errorf("verdict: selecting personal mail for one seat: %w", err)
	}
	// No contacts are anonymised here. A `personal` verdict creates no
	// counterparty record, so there is nothing this sweep could match. A
	// selector that looked for one could find a contact made for another reason.
	n, err := p.purgePersonalSelection(ctx, subject, seat, windows)
	if err != nil {
		return PurgeOutcome{}, fmt.Errorf("verdict: destroying personal mail: %w", err)
	}
	outcome := PurgeOutcome{
		Destroyed: n,
		Skipped:   kept.UnderStatute + kept.UnderRequest + kept.UnderUndeterminedFloor,
		Kept:      keptFromCounts(kept),
	}
	if outcome.Destroyed == 0 {
		return outcome, nil
	}
	// Each destruction commits in its own transaction, so this row cannot join
	// them. A failed write loses no evidence. Every destroyed message has its own
	// erase audit row, and the kept counts are recomputed from current state.
	if err := p.auditPersonalSweepReceipt(ctx, seat, outcome); err != nil {
		return PurgeOutcome{}, fmt.Errorf("verdict: recording the personal sweep receipt: %w", err)
	}
	return outcome, nil
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

// PurgeRemoved applies a provider deletion under the caller's maintenance authority.
func (p *CapturePurger) PurgeRemoved(ctx context.Context, seat ids.UUID, provider, providerMessageID string) error {
	return p.PurgeRemovedAdmitted(ctx, nil, seat, provider, providerMessageID)
}
