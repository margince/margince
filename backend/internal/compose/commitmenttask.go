// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The one writer of a commitment's task, and the effect that runs it when a
// human accepts a proposed one. The direct path and the accept path both end
// here, so a task written either way is the same row.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// CommitmentTaskProposal is the staged payload of a commitment_task.
//
// The links are frozen at staging time: a rep confirms the proposal they were
// shown, and a relink between the two moments must not move the task.
type CommitmentTaskProposal struct {
	ExistingTaskID   *ids.UUID `json:"existing_task_id,omitempty"`
	SourceActivityID ids.UUID  `json:"source_activity_id"`
	Summary          string    `json:"summary"`
	// Party is who made the promise, as the source names them.
	Party string `json:"party"`
	// SeatID is the colleague the task is for: the promiser for a new task,
	// the task's holder for added recipients (approvalsubject.CommitmentAskedOf).
	SeatID *ids.UUID `json:"seat_id,omitempty"`
	// PromiserSeatID is who made the promise.
	PromiserSeatID *ids.UUID `json:"promiser_seat_id,omitempty"`
	// PromiserUnresolved marks a proposal staged before every request named its
	// promiser; nothing stages one now.
	PromiserUnresolved bool `json:"promiser_unresolved,omitempty"`
	// DueDate is a day, YYYY-MM-DD, or empty. Text rather than an instant, so a
	// reviewer edits a day and acceptance decides when that day ends.
	DueDate string `json:"due_date,omitempty"`
	// DueZone is the zone the day ends in; empty is the installation's.
	DueZone  string                         `json:"due_zone,omitempty"`
	Links    []activities.ActivityLinkInput `json:"links"`
	Quote    string                         `json:"quote"`
	Locator  string                         `json:"locator"`
	ClaimID  *ids.UUID                      `json:"claim_id,omitempty"`
	ClaimIDs []ids.UUID                     `json:"claim_ids,omitempty"`
	Body     string                         `json:"body"`
	// PrivateTo is the one member the source mail answers to. The task is read
	// by them and its holder alone. Nil for a shared conversation.
	PrivateTo *ids.UUID `json:"private_to,omitempty"`
}

// commitmentTask is what the writer needs to know about one promise.
type commitmentTask struct {
	Extractor        string
	Locator          string
	Summary          string
	Body             string
	SourceActivityID ids.UUID
	Links            []activities.ActivityLinkInput
	DueDate          string
	DueZone          string
	// DueAt is a deadline already stated as an instant; DueDate is used when nil.
	DueAt     *time.Time
	Assignee  ids.UUID
	ClaimID   *ids.UUID
	ClaimIDs  []ids.UUID
	PrivateTo *ids.UUID
	// Origin is activities.OriginHuman when a colleague took the promise over.
	Origin string
}

// writeCommitmentTask writes the task for a promise, keyed on its locator, and
// points its claim at it.
//
// A task already written under the locator — archived by a rep included — is
// handed back rather than written again (activities.replayedActivity).
func writeCommitmentTask(
	ctx context.Context, tx pgx.Tx, tasks *activities.Store, claims *contacts.Store, t commitmentTask,
) (ids.UUID, error) {
	// The reader's own pass files for the assignee; a human accepting a
	// proposal writes as themselves, so the audit row names nobody else.
	execCtx := extractorContext(ctx, t.Extractor)
	if actor, ok := principal.Actor(ctx); ok && actor.Type == principal.PrincipalSystem {
		execCtx = onBehalfOf(execCtx, t.Assignee)
	}
	sourceSystem, sourceID := activities.CommitmentTaskSource, t.Locator
	assignee := ids.From[ids.UserKind](t.Assignee)
	sourceActivity := t.SourceActivityID
	in := activities.LogActivityInput{
		Kind:             string(crmcontracts.ActivityKindTask),
		Subject:          &t.Summary,
		Body:             &t.Body,
		SourceSystem:     &sourceSystem,
		SourceID:         &sourceID,
		Source:           activities.CommitmentTaskSource,
		Links:            t.Links,
		SourceActivityID: &sourceActivity,
		AssigneeID:       &assignee,
		Origin:           t.Origin,
		DueAt:            t.DueAt,
	}
	// The promiser's own task waits for them as a task made by hand would,
	// also before they first sign in.
	in.AdmitInvitedAssignee()
	if t.PrivateTo != nil {
		in.VisibleOnlyTo(slices.Compact([]ids.UUID{t.Assignee, *t.PrivateTo})...)
	}
	if t.DueAt == nil && t.DueDate != "" {
		due, err := commitmentDueInstant(ctx, tx, t.DueDate, t.DueZone)
		if err != nil {
			return ids.UUID{}, err
		}
		in.DueAt = &due
	}
	task, _, err := tasks.LogActivityTx(execCtx, tx, in)
	if err != nil {
		return ids.UUID{}, err
	}
	taskID := ids.UUID(task.Id)
	claimIDs := commitmentClaimIDs(t.ClaimID, t.ClaimIDs)
	for _, claimID := range claimIDs {
		if err := claims.SetClaimTaskTx(execCtx, tx, claimID, taskID); err != nil {
			return ids.UUID{}, err
		}
	}
	return taskID, nil
}

// commitmentDueInstant turns the day a source stated into the moment that day
// ENDS in the installation's own zone. Callers ask it only for a stated day.
//
// End of day, because a deadline is the last moment the thing is still on
// time. The installation's zone rather than a reader's, because a due day is a
// fact about the record and colleagues elsewhere read the same one. Built from
// the day's own parts: on a daylight-saving day, adding 24 hours lands on the
// wrong day.
//
// A day that will not parse is an error rather than a silent skip: a reviewer
// edited the payload into something acceptance cannot read, and an undated
// task would hide that from them.
func commitmentDueInstant(ctx context.Context, tx pgx.Tx, day, zone string) (time.Time, error) {
	if zone == "" {
		var err error
		if zone, err = identity.TimezoneOf(ctx, tx); err != nil {
			return time.Time{}, err
		}
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return time.Time{}, fmt.Errorf("compose: commitment due-date timezone %q: %w", zone, err)
	}
	parsed, err := time.ParseInLocation(time.DateOnly, day, loc)
	if err != nil {
		return time.Time{}, fmt.Errorf(
			"compose: commitment due date %q is not a date — write it as YYYY-MM-DD: %w", day, err)
	}
	return time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 23, 59, 59, 0, loc).UTC(), nil
}

// commitmentTaskEffect executes an accepted commitment: redeem and write in ONE
// transaction, so the approval is spent if and only if the task exists.
//
// The task goes to the colleague the proposal is for, who is the only member
// who may accept it (commitmentTaskPrecheck). Accepting never makes the
// decider the owner of somebody else's promise.
func commitmentTaskEffect(
	svc *approvals.Service, tasks *activities.Store, claims *contacts.Store,
) approvals.ApprovedEffect {
	return func(ctx context.Context, approvalID ids.ApprovalID, proposedChange json.RawMessage, diffHash string) error {
		var proposal CommitmentTaskProposal
		if err := json.Unmarshal(proposedChange, &proposal); err != nil {
			return fmt.Errorf("compose: unmarshal commitment proposal: %w", err)
		}
		assignee, err := commitmentDecider(ctx, proposedChange)
		if err != nil {
			return err
		}
		// The write runs as the deciding human, so each staged link meets the
		// visibility check a link they typed would. One they cannot see refuses
		// the whole acceptance, and the approval stays unspent.
		return svc.RedeemAndApply(ctx, approvalID, CommitmentTaskKind, diffHash, func(tx pgx.Tx) error {
			privateTo, err := privateWorkHeldNow(ctx, tx, proposal, assignee)
			if err != nil {
				return err
			}
			writer := ctx
			if heldByAnotherThanOwner(privateTo, assignee) {
				// Every record a private thread is filed under is its owner's, so
				// the holder could attach none of them. The links and claims are
				// the extractor's and cannot be edited, so the reader files them.
				writer = principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalSystem, ID: signalScanActor})
			}
			if proposal.ExistingTaskID != nil {
				return attachCommitmentRecipients(writer, tx, tasks, claims, proposal)
			}
			task := commitmentTask{
				Locator: proposal.Locator,
				Summary: proposal.Summary, Body: proposal.Body,
				SourceActivityID: proposal.SourceActivityID, Links: proposal.Links,
				DueDate: proposal.DueDate, DueZone: proposal.DueZone, Assignee: assignee, ClaimID: proposal.ClaimID, ClaimIDs: proposal.ClaimIDs,
				PrivateTo: privateTo, Origin: activities.OriginAgent,
			}
			if heldByAnotherThanOwner(privateTo, assignee) {
				task.Extractor = signalScanActor
			}
			_, err = writeCommitmentTask(writer, tx, tasks, claims, task)
			return err
		})
	}
}

// privateWorkHeldNow is who the source mail is private to now, once it is
// known that the holder may still hold work from it. A sent copy turned back to
// received mail since staging refuses the acceptance. Read from the rows as
// they stand, never from the staged payload.
func privateWorkHeldNow(ctx context.Context, tx pgx.Tx, proposal CommitmentTaskProposal, holder ids.UUID) (*ids.UUID, error) {
	privateTo, err := sourcePrivateToNow(ctx, tx, proposal.SourceActivityID, proposal.PrivateTo)
	if err != nil {
		return nil, err
	}
	may, err := auth.SeatMayHoldPrivateWorkTx(ctx, tx, proposal.SourceActivityID, privateTo, holder)
	if err != nil {
		return nil, err
	}
	if !may {
		return nil, fmt.Errorf("compose: a commitment read out of private mail is for a colleague who may open that mail: %w",
			apperrors.ErrPermissionDenied)
	}
	return privateTo, nil
}

// heldByAnotherThanOwner reports a task from private mail held by somebody
// other than the mailbox owner, such as the colleague who sent the message.
func heldByAnotherThanOwner(privateTo *ids.UUID, holder ids.UUID) bool {
	return privateTo != nil && *privateTo != holder
}

// sourcePrivateToNow is the member the proposal's source mail is private to as
// the extractor's rule reads it at acceptance. It fails closed: a source
// message gone or held, or a mail thread the rule no longer reads, refuses the
// acceptance rather than writing a task whose audience nobody can decide. A
// thread made private since staging narrows the task to its owner, and a
// narrowing set at staging is never widened.
func sourcePrivateToNow(ctx context.Context, tx pgx.Tx, source ids.UUID, staged *ids.UUID) (*ids.UUID, error) {
	var kind string
	var key *string
	var gone bool
	err := tx.QueryRow(ctx, `
		SELECT kind, thread_key, archived_at IS NOT NULL OR restricted_at IS NOT NULL
		  FROM activity WHERE id = $1`, source).Scan(&kind, &key, &gone)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && gone) {
		return nil, fmt.Errorf("compose: the conversation this commitment was read from is gone: %w",
			apperrors.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("compose: reading a commitment's source thread: %w", err)
	}
	if kind != string(crmcontracts.ActivityKindEmail) || key == nil {
		return staged, nil
	}
	_, owner, offered, err := threadReaderNow(ctx, tx, *key)
	if err != nil {
		return nil, err
	}
	if !offered {
		return nil, fmt.Errorf("compose: the conversation this commitment was read from changed since, "+
			"and who may read it is no longer known — review conversation access before retrying: %w",
			apperrors.ErrConflict)
	}
	if staged != nil || owner.IsZero() {
		return staged, nil
	}
	return &owner, nil
}

// commitmentDecider is the member deciding this proposal, when the proposal is
// for them. A proposal for somebody else, or for nobody, is refused as a
// conflict, and the card stays pending.
func commitmentDecider(ctx context.Context, proposal json.RawMessage) (ids.UUID, error) {
	decider, ok := principal.Actor(ctx)
	if !ok {
		return ids.Nil, errors.New("compose: commitment proposal decided without a deciding principal")
	}
	asked, named := approvalsubject.CommitmentAskedOf(proposal)
	if !named || asked != decider.UserID {
		return ids.Nil, fmt.Errorf("compose: only the colleague who made this promise may accept it: %w",
			apperrors.ErrConflict)
	}
	return asked, nil
}

// commitmentTaskPrecheck refuses a proposal that is not the decider's. It also
// refuses one from private mail the decider may no longer hold work from, and
// one filed under a record the decider cannot see. It runs before the decision,
// so the card stays pending. The effect asks again as it writes.
func commitmentTaskPrecheck(db *database.DB, tasks *activities.Store) approvals.ReleasePrecheck {
	return func(ctx context.Context, staged, edited json.RawMessage) error {
		var proposal CommitmentTaskProposal
		if err := json.Unmarshal(staged, &proposal); err != nil {
			return fmt.Errorf("compose: unmarshal staged commitment proposal: %w", err)
		}
		holder, err := commitmentDecider(ctx, staged)
		if err != nil {
			return err
		}
		var privateTo *ids.UUID
		if err := db.Tx(ctx, func(tx pgx.Tx) error {
			var heldErr error
			privateTo, heldErr = privateWorkHeldNow(ctx, tx, proposal, holder)
			return heldErr
		}); err != nil {
			return err
		}
		if !heldByAnotherThanOwner(privateTo, holder) {
			if err := tasks.ProbeLinkWrites(ctx, proposal.Links); err != nil {
				return err
			}
		}
		return refuseCommitmentEdit(staged, edited)
	}
}

// refuseCommitmentEdit refuses an edit that reaches past the promise's
// wording and its day. Everything else is what the reviewer agreed to. The
// locator is the key that remembers the promise: edited, it would let a task
// escape that memory or block another.
func refuseCommitmentEdit(staged, edited json.RawMessage) error {
	if len(edited) == 0 {
		return nil
	}
	var before, after CommitmentTaskProposal
	if err := json.Unmarshal(staged, &before); err != nil {
		return fmt.Errorf("compose: unmarshal staged commitment proposal: %w", err)
	}
	if err := json.Unmarshal(edited, &after); err != nil {
		return &approvals.InvalidEditError{Cause: err}
	}
	if strings.TrimSpace(after.Summary) == "" {
		return &approvals.InvalidEditError{Cause: errors.New("a task needs a summary — say what was committed to")}
	}
	if _, err := time.Parse(time.DateOnly, after.DueDate); after.DueDate != "" && err != nil {
		return &approvals.InvalidEditError{Cause: fmt.Errorf(
			"the due date %q is not a date — write it as YYYY-MM-DD", after.DueDate)}
	}
	if before.ExistingTaskID == nil {
		after.Summary, after.DueDate = before.Summary, before.DueDate
	}
	pinned, err := json.Marshal(after)
	if err != nil {
		return fmt.Errorf("compose: marshal edited commitment proposal: %w", err)
	}
	original, err := json.Marshal(before)
	if err != nil {
		return fmt.Errorf("compose: marshal staged commitment proposal: %w", err)
	}
	if string(pinned) != string(original) {
		return &approvals.InvalidEditError{Cause: errors.New("only the promise's wording and its due date may be edited")}
	}
	return nil
}

func commitmentClaimIDs(single *ids.UUID, plural []ids.UUID) []ids.UUID {
	named := slices.Clone(plural)
	if single != nil && !slices.Contains(named, *single) {
		named = append(named, *single)
	}
	return named
}

func attachCommitmentRecipients(ctx context.Context, tx pgx.Tx, tasks *activities.Store, claims *contacts.Store, proposal CommitmentTaskProposal) error {
	target, err := commitmentRecipientTarget(ctx, tx, tasks, proposal)
	if err != nil {
		return err
	}
	for _, claim := range commitmentClaimIDs(proposal.ClaimID, proposal.ClaimIDs) {
		if err := claims.SetClaimTaskTx(ctx, tx, claim, target); err != nil {
			return err
		}
	}
	return nil
}

func commitmentRecipientTarget(ctx context.Context, tx pgx.Tx, tasks *activities.Store, proposal CommitmentTaskProposal) (ids.UUID, error) {
	target, err := tasks.OpenCommitmentTaskTx(ctx, tx, proposal.Locator, proposal.PrivateTo)
	if err != nil {
		return ids.Nil, err
	}
	if proposal.ExistingTaskID == nil || target.ID.IsZero() || target.ID != *proposal.ExistingTaskID || derefSeat(target.Assignee) != derefSeat(proposal.SeatID) {
		return ids.Nil, apperrors.ErrVersionSkew
	}
	return target.ID, nil
}
