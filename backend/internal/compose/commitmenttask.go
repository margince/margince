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
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// CommitmentTaskProposal is the staged payload of a commitment_task.
//
// The links are frozen at staging time: a rep confirms the proposal they were
// shown, and a relink between the two moments must not move the task.
type CommitmentTaskProposal struct {
	SourceActivityID ids.UUID `json:"source_activity_id"`
	Summary          string   `json:"summary"`
	// Party is who made the promise, as the source names them.
	Party string `json:"party"`
	// SeatID is the colleague the task is for. Nil when nobody could be named,
	// and then accepting it makes it the decider's.
	SeatID *ids.UUID `json:"seat_id,omitempty"`
	// DueDate is a day, YYYY-MM-DD, or empty. Text rather than an instant, so a
	// reviewer edits a day and acceptance decides when that day ends.
	DueDate string                         `json:"due_date,omitempty"`
	Links   []activities.ActivityLinkInput `json:"links"`
	Quote   string                         `json:"quote"`
	Locator string                         `json:"locator"`
	ClaimID *ids.UUID                      `json:"claim_id,omitempty"`
	Body    string                         `json:"body"`
	// PrivateTo is the one member the source mail answers to, whose alone the
	// task is to read. Nil for a shared conversation.
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
	Assignee         ids.UUID
	ClaimID          *ids.UUID
	PrivateTo        *ids.UUID
}

// writeCommitmentTask writes the task for a promise, keyed on its locator, and
// points its claim at it.
//
// A task already written under the locator — archived by a rep included — is
// handed back rather than written again (activities.replayedActivity).
func writeCommitmentTask(
	ctx context.Context, tx pgx.Tx, tasks *activities.Store, claims *contacts.Store, t commitmentTask,
) (ids.UUID, error) {
	execCtx := extractorContext(onBehalfOf(ctx, t.Assignee), t.Extractor)
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
		Origin:           activities.OriginAgent,
	}
	if t.PrivateTo != nil {
		in.VisibleOnlyTo(*t.PrivateTo)
	}
	if t.DueDate != "" {
		due, err := commitmentDueInstant(ctx, tx, t.DueDate)
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
	if t.ClaimID != nil {
		if err := claims.SetClaimTaskTx(execCtx, tx, *t.ClaimID, taskID); err != nil {
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
func commitmentDueInstant(ctx context.Context, tx pgx.Tx, day string) (time.Time, error) {
	zone, err := identity.TimezoneOf(ctx, tx)
	if err != nil {
		return time.Time{}, err
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return time.Time{}, fmt.Errorf("compose: installation timezone %q: %w", zone, err)
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
// The task goes to the colleague the proposal named. A proposal that named
// nobody goes to the human who accepted it: it was staged for them, and a task
// nobody holds reminds nobody. They can hand it on like any task.
func commitmentTaskEffect(
	svc *approvals.Service, tasks *activities.Store, claims *contacts.Store,
) approvals.ApprovedEffect {
	return func(ctx context.Context, approvalID ids.ApprovalID, proposedChange json.RawMessage, diffHash string) error {
		var proposal CommitmentTaskProposal
		if err := json.Unmarshal(proposedChange, &proposal); err != nil {
			return fmt.Errorf("compose: unmarshal commitment proposal: %w", err)
		}
		decider, ok := principal.Actor(ctx)
		if !ok {
			return fmt.Errorf("compose: commitment proposal effect without a deciding principal")
		}
		assignee := decider.UserID
		if proposal.SeatID != nil {
			assignee = *proposal.SeatID
		}
		// Refused before the approval is touched when what was staged already
		// says so; asked again inside the transaction for a thread made private
		// since.
		if !mayHoldPrivateTask(proposal.PrivateTo, assignee) {
			return fmt.Errorf("compose: a commitment read out of private mail is its owner's to accept: %w",
				apperrors.ErrPermissionDenied)
		}
		// The human decided; the write is the reader's, done for them, and the
		// decider is on the approval's own audit row. The decision grants of
		// this kind (activity:create, contact:update) are what admitted the
		// human, and RedeemAndApply spends the approval in the same
		// transaction, so this principal exists only inside that act.
		execCtx := principal.WithActor(ctx, principal.Principal{
			Type: principal.PrincipalSystem, ID: commitmentAcceptActor,
		})
		return svc.RedeemAndApply(ctx, approvalID, CommitmentTaskKind, diffHash, func(tx pgx.Tx) error {
			privateTo, err := sourcePrivateToNow(ctx, tx, proposal.SourceActivityID, proposal.PrivateTo)
			if err != nil {
				return err
			}
			if !mayHoldPrivateTask(privateTo, assignee) {
				return fmt.Errorf("compose: a commitment read out of private mail is its owner's to accept: %w",
					apperrors.ErrPermissionDenied)
			}
			_, err = writeCommitmentTask(execCtx, tx, tasks, claims, commitmentTask{
				Extractor: commitmentAcceptActor, Locator: proposal.Locator,
				Summary: proposal.Summary, Body: proposal.Body,
				SourceActivityID: proposal.SourceActivityID, Links: proposal.Links,
				DueDate: proposal.DueDate, Assignee: assignee, ClaimID: proposal.ClaimID,
				PrivateTo: privateTo,
			})
			return err
		})
	}
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
	if kind != "email" || key == nil {
		return staged, nil
	}
	_, owner, offered, err := threadReaderNow(ctx, tx, *key)
	if err != nil {
		return nil, err
	}
	if !offered {
		return nil, fmt.Errorf("compose: the conversation this commitment was read from changed since, "+
			"and who may read it is no longer known — dismiss this card and let the next reading propose it again: %w",
			apperrors.ErrConflict)
	}
	if staged != nil || owner.IsZero() {
		return staged, nil
	}
	return &owner, nil
}

// commitmentTaskPrecheck refuses an edit that reaches past the promise's
// wording and its day. Everything else is what the reviewer agreed to: the
// locator in particular is the key that remembers the promise, and an edited
// one would let an accepted task escape that memory or block another.
func commitmentTaskPrecheck() approvals.ReleasePrecheck {
	return func(_ context.Context, staged, edited json.RawMessage) error {
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
			return &approvals.InvalidEditError{Cause: fmt.Errorf("a task needs a summary — say what was committed to")}
		}
		if _, err := time.Parse(time.DateOnly, after.DueDate); after.DueDate != "" && err != nil {
			return &approvals.InvalidEditError{Cause: fmt.Errorf(
				"the due date %q is not a date — write it as YYYY-MM-DD", after.DueDate)}
		}
		after.Summary, after.DueDate = before.Summary, before.DueDate
		pinned, err := json.Marshal(after)
		if err != nil {
			return fmt.Errorf("compose: marshal edited commitment proposal: %w", err)
		}
		original, err := json.Marshal(before)
		if err != nil {
			return fmt.Errorf("compose: marshal staged commitment proposal: %w", err)
		}
		if string(pinned) != string(original) {
			return &approvals.InvalidEditError{Cause: fmt.Errorf(
				"only the promise's wording and its due date may be edited")}
		}
		return nil
	}
}

// commitmentAcceptActor captures a task a human accepted from a proposal. The
// promise is still the reader's suggestion; the human is on the decision's
// audit row, which is where "who approved this" belongs.
const commitmentAcceptActor = "agent:commitment-reader"
