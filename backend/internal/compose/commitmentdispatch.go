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
//   - A promise whose colleague cannot be given the task — nobody named, an
//     agent seat, a seat that left, or private mail they may not read — stays
//     as the open claim on the contact. Nobody else is asked for it.
//
// The task is keyed on the promise's evidence locator, archived tasks
// included, so a task a rep dismissed is never written again.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// CommitmentTaskKind is the staging kind for a promise read too unsurely to
// become a task without asking.
const CommitmentTaskKind = approvalsubject.KindCommitmentTask

// CommitmentTaskConfidence separates automatic tasks from proposals after semantic admission.
// Certified firm promises must clear this threshold; confidence cannot admit a non-action.
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
	// absentColleague says the reply named the sender for a colleague's work
	// and the promiser was cleared (senderOwesAnAbsentColleaguesWork).
	absentColleague bool
	Meaning         *commitmentMeaning
	AsOf            time.Time
	ReadMessages    []ids.UUID
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
	Recipients []ids.ContactID
	// DueDate is the day the source stated, YYYY-MM-DD, or empty.
	DueDate string
	// DueZone is the IANA zone that day ends in: the promiser's own when the
	// day was read from their week, the installation's when empty.
	DueZone    string
	Confidence float64
	// NeedsConfirmation keeps reported promises from assigning work without review.
	NeedsConfirmation bool
	Links             []activities.ActivityLinkInput
	// PrivateTo is the one member the conversation answers to, when it is
	// private mail. A promise another colleague made there is theirs only when
	// they may open its message, and its task is read by the two of them.
	PrivateTo *ids.UUID
	// Locator identifies the promise across readings; see commitmentLocator.
	Locator string
	Span    string
	parties []commitmentParty
	// Body says where the task came from, in words a rep can go and check.
	Body            string
	Evidence        approvals.Evidence
	RenewalEvidence *approvals.Evidence
}

// CommitmentOutcome is what the rule did with one promise.
type CommitmentOutcome string

const (
	// CommitmentRemembered means it was already a task, already proposed or
	// refused, dismissed by a human, or nobody is responsible for it. Nothing
	// new was written.
	CommitmentRemembered CommitmentOutcome = "remembered"
	// CommitmentWatched is the customer's promise, filed on their record.
	CommitmentWatched CommitmentOutcome = "watched"
	// CommitmentRetained records a current promise already represented by work.
	CommitmentRetained CommitmentOutcome = "retained"
	// CommitmentTaskWritten is the colleague's task, written directly.
	CommitmentTaskWritten CommitmentOutcome = "task_written"
	// CommitmentProposed is staged for a human to accept.
	CommitmentProposed CommitmentOutcome = "proposed"
	// CommitmentHistoricalProposed is a promise the 30-day horizon stopped on
	// evidence a colleague logged by hand: staged for its promiser to accept.
	// Like any pending proposal its decision stays open, because the card
	// rests on it; no task exists until the promiser accepts.
	CommitmentHistoricalProposed CommitmentOutcome = "historical_proposed"
	// CommitmentPromiserUnassignable is a promise nobody may be asked about. Its
	// maker is unknown, has left, or does not own the private mail it is in. It
	// is recorded on its source as a finding, and no work is created for anyone.
	CommitmentPromiserUnassignable CommitmentOutcome = "promiser_not_assignable"
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
// A proposal is staged under ctx's principal, on behalf of the colleague who
// made the promise. The id is the approval when one was staged, the task when
// one was written.
func (d *CommitmentDispatcher) DispatchTx(
	ctx context.Context, tx pgx.Tx, c Commitment, bundleID ids.UUID,
) (CommitmentOutcome, ids.UUID, error) {
	c, err := withStatedWeek(ctx, tx, c)
	if err != nil {
		return CommitmentRemembered, ids.UUID{}, err
	}
	outcome, id, err := d.routeCommitment(ctx, tx, c, bundleID)
	if err != nil || outcome == CommitmentRemembered || outcome == CommitmentOutcome(activities.CommitmentSourceUnavailable) {
		return outcome, id, err
	}
	evidence := make([]openapi_types.UUID, 0, len(c.ReadMessages))
	for _, message := range c.ReadMessages {
		// The recorded decision is shown with the message, and the colleague
		// who sent it must not learn which other mail on the thread exists.
		if promisedOnAnothersPrivateThread(c) && message != c.SourceActivityID {
			continue
		}
		evidence = append(evidence, openapi_types.UUID(message))
	}
	decision := crmcontracts.CommitmentAdmission{Locator: c.Locator, Summary: c.Summary, SourceQuote: c.Quote, Outcome: string(outcome), CurrentWork: !outcome.withheld(), RuleVersion: activities.CommitmentAdmissionVersion, EvidenceIds: evidence, EvaluatedAt: c.AsOf}
	if c.Meaning != nil {
		decision.Meaning = new(crmcontracts.CommitmentMeaning(*c.Meaning))
	}
	if c.AdmissionSourceID != nil {
		decision.RenewedBy = new(openapi_types.UUID(*c.AdmissionSourceID))
		decision.RenewedAt = c.RenewedAt
	}
	err = activities.RecordCommitmentAdmissionTx(extractorContext(ctx, c.Extractor), tx, ids.From[ids.ActivityKind](c.SourceActivityID), decision)
	return outcome, id, err
}

func (d *CommitmentDispatcher) routeCommitment(ctx context.Context, tx pgx.Tx, c Commitment, bundleID ids.UUID) (CommitmentOutcome, ids.UUID, error) {
	filing := extractorContext(ctx, c.Extractor)
	read, err := d.admission(ctx, tx, c)
	if err != nil {
		return CommitmentRemembered, ids.UUID{}, err
	}
	reason, late := read.Reason, c.proposeLate(read)
	if reason == activities.CommitmentSourceUnavailable {
		return CommitmentOutcome(reason), ids.UUID{}, nil
	}
	if reason == activities.CommitmentCurrent || late {
		offered, err := d.offeredBefore(ctx, tx, c)
		if err != nil {
			return CommitmentRemembered, ids.Nil, err
		}
		if offered != nil {
			return d.reconcilePendingRecipients(ctx, tx, c, *offered, bundleID, late)
		}
	}
	if c.Theirs == nil {
		if c.Seat == nil {
			written, err := d.resolvedWorkBefore(filing, tx, c)
			if err != nil || written {
				return CommitmentRemembered, ids.UUID{}, err
			}
		}
		written, err := d.tasks.CommitmentTaskWritten(filing, tx, c.Locator)
		if err != nil {
			return CommitmentRemembered, ids.UUID{}, err
		}
		if written {
			outcome, id, err := d.reconcileTaskRecipients(filing, tx, c, reason, bundleID)
			if err != nil || outcome == CommitmentProposed {
				return outcome, id, err
			}
			return d.renewTask(ctx, tx, c, reason)
		}
	}
	var recorded bool
	if reason != activities.CommitmentSourceUnavailable {
		var held bool
		renewedBy, renewedAt := c.AdmissionSourceID, c.RenewedAt
		if c.Meaning.withheldOutcome() != "" {
			renewedBy, renewedAt = nil, nil
		}
		held, recorded, err = activities.CommitmentAdmissionStateTx(ctx, tx, ids.From[ids.ActivityKind](c.SourceActivityID), c.Locator, c.AsOf, renewedBy, renewedAt)
		if err != nil || held {
			return CommitmentRemembered, ids.UUID{}, err
		}
	}
	if outcome := c.Meaning.withheldOutcome(); outcome != "" {
		return outcome, ids.UUID{}, nil
	}
	if late {
		c.NeedsConfirmation = true
		outcome, id, err := d.createOurCommitment(ctx, tx, c, bundleID, true)
		if outcome == CommitmentProposed {
			outcome = CommitmentHistoricalProposed
		}
		return outcome, id, err
	}
	if reason != activities.CommitmentCurrent {
		return d.retainWithheld(ctx, tx, c, reason, recorded)
	}
	if c.Theirs != nil {
		return d.watchCommitment(filing, tx, c, reason)
	}

	return d.createOurCommitment(ctx, tx, c, bundleID, false)
}

// createOurCommitment writes or proposes one of our colleagues' promises. A
// late one is read late, so it waits the whole horizon for its promiser.
func (d *CommitmentDispatcher) createOurCommitment(ctx context.Context, tx pgx.Tx, c Commitment, bundleID ids.UUID, late bool) (CommitmentOutcome, ids.UUID, error) {
	filing := extractorContext(ctx, c.Extractor)

	claimIDs, open, err := d.fileRecipientClaims(filing, tx, c, activities.CommitmentCurrent, false)
	if err != nil || !open {
		return CommitmentRemembered, ids.UUID{}, err
	}
	var claimID *ids.UUID
	if len(claimIDs) == 1 {
		claimID, claimIDs = &claimIDs[0], nil
	}

	// Whether the promiser can be GIVEN the task, asked before writing one. A
	// refused write would fail the conversation and every later pass over it.
	// Asked with the create door's rule, so an invited colleague counts.
	asked, state, err := commitmentAskedOf(ctx, tx, c)
	if err != nil {
		return CommitmentRemembered, ids.UUID{}, err
	}
	if asked.IsZero() {
		return CommitmentPromiserUnassignable, ids.UUID{}, nil
	}
	if !c.NeedsConfirmation && c.Confidence >= CommitmentTaskConfidence {
		task, err := writeCommitmentTask(ctx, tx, d.tasks, d.claims, commitmentTask{
			Extractor: c.Extractor, Locator: c.Locator, Summary: c.Summary, Body: c.Body,
			SourceActivityID: c.SourceActivityID, Links: c.Links, DueDate: c.DueDate, DueZone: c.DueZone,
			Assignee: asked, ClaimID: claimID, ClaimIDs: claimIDs, PrivateTo: c.PrivateTo,
			Origin: activities.OriginAgent,
		})
		return CommitmentTaskWritten, task, err
	}
	proposal := CommitmentTaskProposal{
		SourceActivityID: c.SourceActivityID, Summary: c.Summary, Party: c.Party,
		SeatID: &asked, PromiserSeatID: &asked, DueDate: c.DueDate, DueZone: c.DueZone, Links: c.Links, Quote: c.Quote,
		Locator: c.Locator, ClaimID: claimID, ClaimIDs: claimIDs, Body: c.Body, PrivateTo: c.PrivateTo,
	}
	return d.stageCommitmentProposal(ctx, tx, c, proposal, bundleID, asked, commitmentWindow(state, late))
}

// commitmentWindow is how long a proposal waits for its promiser. An invited
// colleague finds the card when they first sign in, which may be weeks away,
// and the default 72 hours would expire it unseen. A promise from a late
// report is as old as the horizon already, so it gets the same wait.
func commitmentWindow(state activities.SeatWorkState, late bool) *time.Duration {
	if state != activities.SeatInvited && !late {
		return nil
	}
	return new(time.Duration(provenance.CommitmentHorizonDays) * 24 * time.Hour)
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
		if err != nil || offered != nil {
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
//
//nolint:nilnil // No prior matching offer allows the dispatcher to create one.
func (d *CommitmentDispatcher) offeredBefore(ctx context.Context, tx pgx.Tx, c Commitment) (*approvals.Offer, error) {
	offers, err := d.approval.OffersForTx(ctx, tx, CommitmentTaskKind, c.SourceActivityID)
	if err != nil {
		return nil, err
	}
	for _, held := range offers {
		if !held.Pending() && !held.Rejected() && !held.Approved() {
			continue
		}
		if c.Theirs != nil && held.Rejected() {
			continue
		}
		var offer CommitmentTaskProposal
		if err := json.Unmarshal(held.Change, &offer); err != nil {
			return nil, fmt.Errorf("compose: unmarshal a commitment proposal: %w", err)
		}
		if offer.Locator == c.Locator {
			if !held.Approved() {
				return &held, nil
			}
			written, err := d.tasks.CommitmentTaskWritten(ctx, tx, c.Locator)
			if err != nil || written {
				return nil, err
			}
			return &held, nil
		}
		if samePromiseBeforeAttributionChanged(c, offer) {
			return &held, nil
		}
	}
	return nil, nil
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
		due, err := commitmentDueInstant(ctx, tx, c.DueDate, c.DueZone)
		if err != nil {
			return crmcontracts.ConversationClaim{}, false, err
		}
		in.DueAt = &due
	}
	return d.claims.RecordConversationClaimTx(ctx, tx, in)
}

// stageCommitmentProposal puts one promise to the one colleague asked. With
// nobody asked it stages nothing: a promise is never left for whoever reads it.
// A proposal refused before, or one still waiting, is not staged again: the
// identity is the locator.
func (d *CommitmentDispatcher) stageCommitmentProposal(
	ctx context.Context, tx pgx.Tx, c Commitment, proposal CommitmentTaskProposal, bundleID ids.UUID, asked ids.UUID, window *time.Duration,
) (CommitmentOutcome, ids.UUID, error) {
	if asked.IsZero() {
		return CommitmentPromiserUnassignable, ids.UUID{}, nil
	}
	ctx = onBehalfOf(ctx, asked)
	raw, err := json.Marshal(proposal)
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
		Summary:        proposal.Summary,
		Evidence:       evidence,
		BundleID:       bundleID,
		JoinPending:    true,
		TTL:            window,
	})
	if errors.Is(err, approvals.ErrNoDecider) {
		return CommitmentRemembered, ids.UUID{}, nil
	}
	if err != nil || !staged {
		return CommitmentRemembered, ids.UUID{}, err
	}
	return CommitmentProposed, approvalID.UUID, nil
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
