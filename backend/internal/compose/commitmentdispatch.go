// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// One rule for every promise an extractor reads out of a conversation, whether
// the conversation was a meeting or a mail thread.
//
//   - A promise the CUSTOMER made is filed as a claim on them and watched. It
//     is never a task: nobody here can do it.
//   - A promise one of OUR colleagues made becomes their task. Read confidently
//     enough, it is written directly and marked as the extractor's; otherwise
//     it is proposed to them and written when they accept.
//   - A promise nobody can be named for is proposed to whoever the reading
//     belongs to, and accepting it makes it theirs.
//
// The task is keyed on the promise's evidence locator, archived tasks
// included, so a task a rep dismissed is never written again.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// CommitmentTaskKind is the staging kind for a promise read too unsurely to
// become a task without asking.
const CommitmentTaskKind = "commitment_task"

// CommitmentTaskConfidence is the reading at or above which a promise a named
// colleague made becomes their task without asking. Below it, down to the
// extractor's own floor, the promise is proposed instead.
const CommitmentTaskConfidence = 0.85

// The claim kinds a commitment is filed under.
const (
	claimKindOurs   = string(crmcontracts.ConversationClaimKindCommitmentOurs)
	claimKindTheirs = string(crmcontracts.ConversationClaimKindCommitmentTheirs)
)

// commitmentIdentityLocator is the payload key the staging identity is drawn
// from; it must spell what CommitmentTaskProposal's tag spells.
const commitmentIdentityLocator = "locator"

// Commitment is one promise as an extractor hands it to the rule.
type Commitment struct {
	// Extractor is the principal id the claim and the task are captured by,
	// which is what marks the task as the extractor's rather than a human's.
	Extractor        string
	SourceActivityID ids.UUID
	Summary          string
	// Quote is the source's own words, shown beside the promise.
	Quote string
	// Party is the promiser as the source names them.
	Party string
	// Seat is the colleague who made the promise, when one could be named.
	Seat *ids.UUID
	// Theirs is the customer who made it. Set means the promise is theirs.
	Theirs *ids.ContactID
	// PromisedTo is who one of our promises was made to, so it is filed on
	// their record as well. Nil when the source does not name exactly one.
	PromisedTo *ids.ContactID
	// DueDate is the day the source stated, YYYY-MM-DD, or empty.
	DueDate    string
	Confidence float64
	Links      []activities.ActivityLinkInput
	// Locator identifies the promise across readings; see commitmentLocator.
	Locator string
	// Body says where the task came from, in words a rep can go and check.
	Body     string
	Evidence approvals.Evidence
}

// CommitmentOutcome is what the rule did with one promise.
type CommitmentOutcome int

const (
	// CommitmentRemembered means it was already a task, already proposed or
	// refused, or dismissed by a human. Nothing new was written.
	CommitmentRemembered CommitmentOutcome = iota
	// CommitmentWatched is the customer's promise, filed on their record.
	CommitmentWatched
	// CommitmentTaskWritten is the colleague's task, written directly.
	CommitmentTaskWritten
	// CommitmentProposed is staged for a human to accept.
	CommitmentProposed
)

// commitmentLocator names one promise across readings of one source: the same
// source, the same side, the same party and the same evidence span. It is
// built by the server from what it holds, never from the model's wording, so a
// re-read that paraphrases the summary still finds the promise it read before.
func commitmentLocator(sourceActivity ids.UUID, kind, party, span string) string {
	sum := sha256.Sum256([]byte(sourceActivity.String() + "\x00" + kind + "\x00" + party + "\x00" + span))
	return hex.EncodeToString(sum[:])
}

// CommitmentDispatcher applies the rule. Every extractor calls the same one.
type CommitmentDispatcher struct {
	claims   *contacts.Store
	tasks    *activities.Store
	approval *approvals.Service
}

// NewCommitmentDispatcher builds the rule over the stores it writes and the
// approvals service its proposals are decided on.
func NewCommitmentDispatcher(pool *pgxpool.Pool, approval *approvals.Service) *CommitmentDispatcher {
	db := InstallationDB(pool)
	return &CommitmentDispatcher{
		claims: contacts.NewStore(db), tasks: activities.NewStore(db), approval: approval,
	}
}

// DispatchTx applies the rule to one promise inside the caller's transaction.
// A proposal is staged under ctx's principal, on behalf of the colleague when
// one is named; the caller decides who an unnamed promise is staged for by
// what ctx carries. The id is the approval when one was staged, the task when
// one was written.
func (d *CommitmentDispatcher) DispatchTx(
	ctx context.Context, tx pgx.Tx, c Commitment, bundleID ids.UUID,
) (CommitmentOutcome, ids.UUID, error) {
	filing := extractorContext(ctx, c.Extractor)
	if c.Theirs != nil {
		if _, err := d.fileClaim(filing, tx, c, *c.Theirs, claimKindTheirs); err != nil {
			return 0, ids.UUID{}, err
		}
		return CommitmentWatched, ids.UUID{}, nil
	}
	written, err := d.tasks.CommitmentTaskWritten(filing, tx, c.Locator)
	if err != nil || written {
		return CommitmentRemembered, ids.UUID{}, err
	}
	var claimID *ids.UUID
	if c.PromisedTo != nil {
		claim, err := d.fileClaim(filing, tx, c, *c.PromisedTo, claimKindOurs)
		if err != nil {
			return 0, ids.UUID{}, err
		}
		if claim.Status == crmcontracts.ConversationClaimStatusDismissed {
			return CommitmentRemembered, ids.UUID{}, nil
		}
		id := ids.UUID(claim.Id)
		claimID = &id
	}
	refused, err := d.refusedBefore(ctx, tx, c)
	if err != nil || refused {
		return CommitmentRemembered, ids.UUID{}, err
	}
	if c.Seat != nil && c.Confidence >= CommitmentTaskConfidence {
		task, err := writeCommitmentTask(ctx, tx, d.tasks, d.claims, commitmentTask{
			Extractor: c.Extractor, Locator: c.Locator, Summary: c.Summary, Body: c.Body,
			SourceActivityID: c.SourceActivityID, Links: c.Links, DueDate: c.DueDate,
			Assignee: *c.Seat, ClaimID: claimID,
		})
		return CommitmentTaskWritten, task, err
	}
	return d.propose(ctx, tx, c, claimID, bundleID)
}

// refusedBefore reports whether a human already turned this promise down as a
// proposal. A later, surer reading of the same words must not write the task
// that human refused. The read locks every offer on the source, so a refusal
// landing while this runs is either seen here or waits for this to commit.
func (d *CommitmentDispatcher) refusedBefore(ctx context.Context, tx pgx.Tx, c Commitment) (bool, error) {
	rejected, err := d.approval.RejectedChangesForTx(ctx, tx, CommitmentTaskKind, c.SourceActivityID)
	if err != nil {
		return false, err
	}
	for _, raw := range rejected {
		var offer CommitmentTaskProposal
		if err := json.Unmarshal(raw, &offer); err != nil {
			return false, fmt.Errorf("compose: unmarshal a refused commitment proposal: %w", err)
		}
		if offer.Locator == c.Locator {
			return true, nil
		}
	}
	return false, nil
}

// fileClaim records the promise on a contact's record, keyed on its locator.
func (d *CommitmentDispatcher) fileClaim(
	ctx context.Context, tx pgx.Tx, c Commitment, contact ids.ContactID, kind string,
) (crmcontracts.ConversationClaim, error) {
	in := contacts.ClaimInput{
		ContactID: contact, Kind: kind, Body: c.Summary, ActivityID: c.SourceActivityID,
		Quote: c.Quote, Source: c.Extractor, Locator: c.Locator,
	}
	if c.DueDate != "" {
		due, err := commitmentDueInstant(ctx, tx, c.DueDate)
		if err != nil {
			return crmcontracts.ConversationClaim{}, err
		}
		in.DueAt = &due
	}
	claim, _, err := d.claims.RecordConversationClaimTx(ctx, tx, in)
	return claim, err
}

// propose stages the promise for a human. A proposal refused before, or one
// still waiting, is not staged again: the identity is the locator.
func (d *CommitmentDispatcher) propose(
	ctx context.Context, tx pgx.Tx, c Commitment, claimID *ids.UUID, bundleID ids.UUID,
) (CommitmentOutcome, ids.UUID, error) {
	if c.Seat != nil {
		ctx = onBehalfOf(ctx, *c.Seat)
	}
	raw, err := json.Marshal(CommitmentTaskProposal{
		SourceActivityID: c.SourceActivityID, Summary: c.Summary, Party: c.Party,
		SeatID: c.Seat, DueDate: c.DueDate, Links: c.Links, Quote: c.Quote,
		Locator: c.Locator, ClaimID: claimID, Body: c.Body,
	})
	if err != nil {
		return 0, ids.UUID{}, fmt.Errorf("compose: marshal commitment proposal: %w", err)
	}
	canonical, hash, err := diffhash.Canonical(raw)
	if err != nil {
		return 0, ids.UUID{}, fmt.Errorf("compose: canonicalize commitment proposal: %w", err)
	}
	identity, err := json.Marshal(map[string]string{commitmentIdentityLocator: c.Locator})
	if err != nil {
		return 0, ids.UUID{}, fmt.Errorf("compose: marshal commitment identity: %w", err)
	}
	approvalID, staged, err := d.approval.StageUnlessDeclinedTx(ctx, tx, approvals.StageInput{
		Kind:           CommitmentTaskKind,
		ProposedChange: canonical,
		DiffHash:       hash,
		Identity:       identity,
		TargetType:     string(recordTypeActivity),
		TargetID:       c.SourceActivityID,
		Summary:        c.Summary,
		Evidence:       []approvals.Evidence{c.Evidence},
		BundleID:       bundleID,
		JoinPending:    true,
	})
	if err != nil || !staged {
		return CommitmentRemembered, ids.UUID{}, err
	}
	return CommitmentProposed, approvalID.UUID, nil
}

// extractorContext files under the extractor's own name, so the claim and the
// task say a reader wrote them. It renames the product's own pass and nothing
// else: a human stays who they are, and a context with no principal stays
// without one, so every gate below answers for the caller that is really
// there rather than for a principal this function made up.
func extractorContext(ctx context.Context, extractor string) context.Context {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalSystem {
		return ctx
	}
	actor.ID = extractor
	return principal.WithActor(ctx, actor)
}
