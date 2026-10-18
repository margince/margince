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
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	openapi_types "github.com/oapi-codegen/runtime/types"

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
//
// It sits inside the gap the certification records show: a plain dated promise
// is read at 0.9 or more, a soft one at 0.3 or less, and the extractors drop
// anything under 0.7. TestCommitmentTaskConfidenceSitsInTheCertifiedGap holds it.
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
	AsOf         time.Time
	ReadMessages []ids.UUID
	// Extractor is the principal id the claim and the task are captured by,
	// which is what marks the task as the extractor's rather than a human's.
	Extractor        string
	SourceActivityID ids.UUID
	// AdmissionSourceID names verified later evidence that renews this same promise.
	AdmissionSourceID *ids.UUID
	RenewedAt         *time.Time
	Summary           string
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
	// NeedsConfirmation keeps reported promises from assigning work without review.
	NeedsConfirmation bool
	Links             []activities.ActivityLinkInput
	// PrivateTo is the one member the conversation answers to, when it is
	// private mail. The task is theirs alone to read, and nobody else's to
	// hold: a promise in mail only they may read is proposed to them.
	PrivateTo *ids.UUID
	// Locator identifies the promise across readings; see commitmentLocator.
	Locator string
	Span    string
	// Body says where the task came from, in words a rep can go and check.
	Body            string
	Evidence        approvals.Evidence
	RenewalEvidence *approvals.Evidence
}

// CommitmentOutcome is what the rule did with one promise.
type CommitmentOutcome string

const (
	// CommitmentRemembered means it was already a task, already proposed or
	// refused, or dismissed by a human. Nothing new was written.
	CommitmentRemembered CommitmentOutcome = "remembered"
	// CommitmentWatched is the customer's promise, filed on their record.
	CommitmentWatched CommitmentOutcome = "watched"
	// CommitmentRetained records a current promise already represented by work.
	CommitmentRetained CommitmentOutcome = "retained"
	// CommitmentTaskWritten is the colleague's task, written directly.
	CommitmentTaskWritten CommitmentOutcome = "task_written"
	// CommitmentProposed is staged for a human to accept.
	CommitmentProposed CommitmentOutcome = "proposed"
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
	outcome, id, err := d.routeCommitment(ctx, tx, c, bundleID)
	if err != nil || outcome == CommitmentRemembered || outcome == CommitmentOutcome(activities.CommitmentSourceUnavailable) {
		return outcome, id, err
	}
	evidence := make([]openapi_types.UUID, 0, len(c.ReadMessages))
	for _, message := range c.ReadMessages {
		evidence = append(evidence, openapi_types.UUID(message))
	}
	decision := crmcontracts.CommitmentAdmission{Locator: c.Locator, Summary: c.Summary, SourceQuote: c.Quote, Outcome: string(outcome), CurrentWork: !outcome.withheld(), RuleVersion: activities.CommitmentAdmissionVersion, EvidenceIds: evidence, EvaluatedAt: c.AsOf}
	if c.AdmissionSourceID != nil {
		decision.RenewedBy = new(openapi_types.UUID(*c.AdmissionSourceID))
		decision.RenewedAt = c.RenewedAt
	}
	err = activities.RecordCommitmentAdmissionTx(extractorContext(ctx, c.Extractor), tx, ids.From[ids.ActivityKind](c.SourceActivityID), decision)
	return outcome, id, err
}

func (d *CommitmentDispatcher) routeCommitment(ctx context.Context, tx pgx.Tx, c Commitment, bundleID ids.UUID) (CommitmentOutcome, ids.UUID, error) {
	filing := extractorContext(ctx, c.Extractor)
	reason, err := d.admission(ctx, tx, c)
	if err != nil {
		return CommitmentRemembered, ids.UUID{}, err
	}
	if reason == activities.CommitmentSourceUnavailable {
		return CommitmentOutcome(reason), ids.UUID{}, nil
	}
	offered, err := d.offeredBefore(ctx, tx, c)
	if err != nil || offered {
		return CommitmentRemembered, ids.UUID{}, err
	}
	if c.Theirs == nil {
		written, err := d.tasks.CommitmentTaskWritten(filing, tx, c.Locator)
		if err != nil {
			return CommitmentRemembered, ids.UUID{}, err
		}
		if written {
			return d.renewTask(ctx, tx, c, reason)
		}
	}
	var recorded bool
	if reason != activities.CommitmentSourceUnavailable {
		var held bool
		held, recorded, err = activities.CommitmentAdmissionStateTx(ctx, tx, ids.From[ids.ActivityKind](c.SourceActivityID), c.Locator, c.AsOf, c.AdmissionSourceID, c.RenewedAt)
		if err != nil || held {
			return CommitmentRemembered, ids.UUID{}, err
		}
	}
	if reason != activities.CommitmentCurrent {
		return d.retainWithheld(ctx, tx, c, reason, recorded)
	}
	if c.Theirs != nil {
		return d.watchCommitment(filing, tx, c, reason)
	}

	return d.createOurCommitment(ctx, tx, c, bundleID)
}

func (d *CommitmentDispatcher) createOurCommitment(ctx context.Context, tx pgx.Tx, c Commitment, bundleID ids.UUID) (CommitmentOutcome, ids.UUID, error) {
	filing := extractorContext(ctx, c.Extractor)

	var claimID *ids.UUID
	if c.PromisedTo != nil {
		claim, _, err := d.fileClaim(filing, tx, c, *c.PromisedTo, claimKindOurs, activities.CommitmentCurrent)
		if err != nil {
			return CommitmentRemembered, ids.UUID{}, err
		}
		if claim.Status == crmcontracts.ConversationClaimStatusDismissed {
			return CommitmentRemembered, ids.UUID{}, nil
		}
		id := ids.UUID(claim.Id)
		claimID = &id
	}
	// Whether the seat can be GIVEN the task, asked before writing one. An automatic
	// writer may only assign to an active colleague, and most seats on imported mail
	// never signed in or have left — so a promise sent by one of them reaches the
	// writer and is refused, which fails the whole conversation and every later pass
	// over it.
	holdsWork := false
	if c.Seat != nil {
		var err error
		if holdsWork, err = activities.SeatCanHoldAutomaticWork(ctx, tx, ids.From[ids.UserKind](*c.Seat)); err != nil {
			return CommitmentRemembered, ids.UUID{}, err
		}
	}
	if !c.NeedsConfirmation && holdsWork && c.Confidence >= CommitmentTaskConfidence && mayHoldPrivateTask(c.PrivateTo, *c.Seat) {
		task, err := writeCommitmentTask(ctx, tx, d.tasks, d.claims, commitmentTask{
			Extractor: c.Extractor, Locator: c.Locator, Summary: c.Summary, Body: c.Body,
			SourceActivityID: c.SourceActivityID, Links: c.Links, DueDate: c.DueDate,
			Assignee: *c.Seat, ClaimID: claimID, PrivateTo: c.PrivateTo,
		})
		return CommitmentTaskWritten, task, err
	}
	return d.propose(ctx, tx, c, holdsWork, claimID, bundleID)
}

func (d *CommitmentDispatcher) watchCommitment(ctx context.Context, tx pgx.Tx, c Commitment, reason activities.CommitmentAdmissionReason) (CommitmentOutcome, ids.UUID, error) {
	claim, created, err := d.fileClaim(ctx, tx, c, *c.Theirs, claimKindTheirs, reason)
	if err != nil || claim.Status != crmcontracts.ConversationClaimStatusOpen {
		return CommitmentRemembered, ids.UUID{}, err
	}
	if !created {
		return CommitmentRetained, ids.UUID{}, nil
	}
	return CommitmentWatched, ids.UUID{}, nil
}

func (d *CommitmentDispatcher) retainWithheld(ctx context.Context, tx pgx.Tx, c Commitment, reason activities.CommitmentAdmissionReason, recorded bool) (CommitmentOutcome, ids.UUID, error) {
	filing := extractorContext(ctx, c.Extractor)
	if reason == activities.CommitmentSourceUnavailable {
		return CommitmentOutcome(reason), ids.UUID{}, nil
	}
	if c.AdmissionSourceID != nil {
		return CommitmentRemembered, ids.UUID{}, nil
	}
	contact, kind := c.PromisedTo, claimKindOurs
	if c.Theirs != nil {
		contact, kind = c.Theirs, claimKindTheirs
	}
	if !recorded && contact == nil {
		offered, err := d.offeredBefore(ctx, tx, c)
		if err != nil || offered {
			return CommitmentRemembered, ids.UUID{}, err
		}
	}
	if contact != nil {
		claim, created, err := d.fileClaim(filing, tx, c, *contact, kind, reason)
		if err != nil || claim.Status == crmcontracts.ConversationClaimStatusDismissed {
			return CommitmentRemembered, ids.UUID{}, err
		}
		if !created && !recorded && claim.Status == crmcontracts.ConversationClaimStatusOpen {
			return CommitmentRetained, ids.UUID{}, nil
		}
	}
	return CommitmentOutcome(reason), ids.UUID{}, nil
}

// offeredBefore reports whether this promise was already put to a human, who
// refused it or has not answered yet. A later, surer reading of the same words
// must not write the task that human refused, nor write it around a proposal
// they may still refuse. The read locks every offer on the source, so a
// decision landing while this runs is either seen here or waits for this to
// commit.
func (d *CommitmentDispatcher) offeredBefore(ctx context.Context, tx pgx.Tx, c Commitment) (bool, error) {
	offers, err := d.approval.OffersForTx(ctx, tx, CommitmentTaskKind, c.SourceActivityID)
	if err != nil {
		return false, err
	}
	for _, held := range offers {
		if !held.Pending() && !held.Rejected() && !held.Approved() {
			continue
		}
		var offer CommitmentTaskProposal
		if err := json.Unmarshal(held.Change, &offer); err != nil {
			return false, fmt.Errorf("compose: unmarshal a commitment proposal: %w", err)
		}
		if (offer.Locator == c.Locator && !held.Approved()) || resolvedPriorPromise(c, offer) {
			return true, nil
		}
	}
	return false, nil
}

// fileClaim records the promise on a contact's record, keyed on its locator,
// and reports whether this call created it.
func (d *CommitmentDispatcher) fileClaim(
	ctx context.Context, tx pgx.Tx, c Commitment, contact ids.ContactID, kind string, reason activities.CommitmentAdmissionReason,
) (crmcontracts.ConversationClaim, bool, error) {
	in := contacts.ClaimInput{
		ContactID: contact, Kind: kind, Body: c.Summary, ActivityID: c.SourceActivityID,
		Quote: c.Quote, Source: c.Extractor, Locator: c.Locator,
		Renewal: c.AdmissionSourceID != nil && reason == activities.CommitmentCurrent,
	}
	if c.DueDate != "" {
		due, err := commitmentDueInstant(ctx, tx, c.DueDate)
		if err != nil {
			return crmcontracts.ConversationClaim{}, false, err
		}
		in.DueAt = &due
	}
	return d.claims.RecordConversationClaimTx(ctx, tx, in)
}

// propose stages the promise for a human. A proposal refused before, or one
// still waiting, is not staged again: the identity is the locator.
func (d *CommitmentDispatcher) propose(
	ctx context.Context, tx pgx.Tx, c Commitment, holdsWork bool, claimID *ids.UUID, bundleID ids.UUID,
) (CommitmentOutcome, ids.UUID, error) {
	seat := c.Seat
	if !holdsWork {
		seat = nil
	}
	if c.PrivateTo != nil && !mayHoldPrivateTask(c.PrivateTo, derefSeat(seat)) {
		seat = c.PrivateTo
	}
	if seat != nil {
		ctx = onBehalfOf(ctx, *seat)
	}
	raw, err := json.Marshal(CommitmentTaskProposal{
		SourceActivityID: c.SourceActivityID, Summary: c.Summary, Party: c.Party,
		SeatID: seat, PromiserSeatID: c.Seat, PromiserUnresolved: c.Seat == nil, DueDate: c.DueDate, Links: c.Links, Quote: c.Quote,
		Locator: c.Locator, ClaimID: claimID, Body: c.Body, PrivateTo: c.PrivateTo,
	})
	if err != nil {
		return CommitmentRemembered, ids.UUID{}, fmt.Errorf("compose: marshal commitment proposal: %w", err)
	}
	canonical, hash, err := diffhash.Canonical(raw)
	if err != nil {
		return CommitmentRemembered, ids.UUID{}, fmt.Errorf("compose: canonicalize commitment proposal: %w", err)
	}
	identity, err := json.Marshal(map[string]string{commitmentIdentityLocator: c.Locator})
	if err != nil {
		return CommitmentRemembered, ids.UUID{}, fmt.Errorf("compose: marshal commitment identity: %w", err)
	}
	evidence := []approvals.Evidence{c.Evidence}
	if c.RenewalEvidence != nil {
		evidence = append(evidence, *c.RenewalEvidence)
	}
	approvalID, staged, err := d.approval.StageUnlessDeclinedTx(ctx, tx, approvals.StageInput{
		Kind:           CommitmentTaskKind,
		ProposedChange: canonical,
		DiffHash:       hash,
		Identity:       identity,
		TargetType:     string(recordTypeActivity),
		TargetID:       c.SourceActivityID,
		Summary:        c.Summary,
		Evidence:       evidence,
		BundleID:       bundleID,
		JoinPending:    true,
	})
	if err != nil || !staged {
		return CommitmentRemembered, ids.UUID{}, err
	}
	return CommitmentProposed, approvalID.UUID, nil
}

// mayHoldPrivateTask reports whether a member may hold a task about this
// conversation: any member when it is shared, only its owner when it is private.
func mayHoldPrivateTask(privateTo *ids.UUID, seat ids.UUID) bool {
	return privateTo == nil || *privateTo == seat
}

// derefSeat is the seat or the zero id, which no private conversation names.
func derefSeat(seat *ids.UUID) ids.UUID {
	if seat == nil {
		return ids.UUID{}
	}
	return *seat
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
