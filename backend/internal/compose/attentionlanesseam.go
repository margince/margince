// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The OPTIONAL attention lanes' seams over the engines that already own
// what they read. Three are bound today; commitments is deliberately not
// (newAttentionService passes nil until its production writer, issue #849,
// exists), and its seam stays compiled here for that rebinding.
//
// Each is a binding rather than an implementation, which is the point: the
// promises come from the people module's claim read, the deal risk from the
// same candidate engine whats_slipping_this_week reads, and the meetings from
// the activities list every other activity surface reads. A lane that derived
// its own answer here would be a second opinion the product would have to keep
// agreeing with.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/idlebase"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// attentionCommitments reads the acting rep's own promises through the people
// store.
//
// A claim carries no assignee, so ownership rides the person it was made to:
// the rep who holds the relationship is the one who made the promise in their
// own captured conversation. A principal with no human behind it has no
// promises of its own to keep, which is a refusal rather than an empty lane —
// the feed omits and NAMES the lane instead of reporting a clear day.
type attentionCommitments struct{ store *people.Store }

// The binding is deliberately not wired today (newAttentionService passes nil
// — the lane's production writer is issue #849's), so this assertion is the
// one thing keeping the seam compiled against the interface it will be
// rebound as. The store read behind it keeps its own integration test; the
// seam wiring itself is untested exactly because nothing wires it.
var _ attention.Commitments = attentionCommitments{}

func (c attentionCommitments) DueBy(ctx context.Context, by time.Time, limit int) ([]attention.Commitment, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID.IsZero() {
		return nil, apperrors.ErrPermissionDenied
	}
	due, err := c.store.OpenCommitmentsDue(ctx, ids.From[ids.UserKind](actor.UserID), by, limit)
	if err != nil {
		return nil, err
	}
	promises := make([]attention.Commitment, 0, len(due))
	for _, row := range due {
		promises = append(promises, attention.Commitment{
			ID:          row.ID,
			PersonID:    row.PersonID.UUID,
			Body:        row.Body,
			Quote:       row.SourceQuote,
			SourceLabel: row.SourceLabel,
			OccurredAt:  row.OccurredAt,
			DueAt:       row.DueAt,
		})
	}
	return promises, nil
}

// attentionAtRisk reads the pipeline's own risk candidates at the morning
// queue's shorter idle window.
//
// It calls quietDealLister, the SAME engine whats_slipping_this_week reads, so
// there is one at-risk rule in the product and the two surfaces cannot come to
// disagree about which deals are in trouble. Only the patience differs, and it
// is named here rather than buried: a queue exists to warn, and the stalled
// threshold is a status rather than a warning.
// The depth-reporting scanner, not the plain SlippingLister: the team board
// needs to know whether the sweep was cut, and that cannot be recovered from the
// rows it returns.
type attentionAtRisk struct {
	lister func(context.Context) ([]agents.SlippingDeal, bool, error)
}

func (a attentionAtRisk) Quiet(ctx context.Context) ([]attention.RiskyDeal, bool, error) {
	candidates, cut, err := a.lister(ctx)
	if err != nil {
		return nil, false, err
	}
	now := clockNow()
	risky := make([]attention.RiskyDeal, 0, len(candidates))
	for _, deal := range candidates {
		risky = append(risky, attention.RiskyDeal{
			DealID:            deal.DealID,
			Name:              deal.Name,
			StageID:           deal.StageID,
			OwnerID:           deal.OwnerID,
			AmountMinor:       deal.AmountMinor,
			Currency:          deal.Currency,
			QuietDays:         idleDaysOf(deal, now),
			CloseOverdue:      deal.CloseOverdue,
			ExpectedCloseDate: deal.ExpectedCloseDate,
		})
	}
	return risky, cut, nil
}

// idleDaysOf is how long the deal has been quiet, counted from the base the
// idle RULE measures from — through idlebase.Since rather than by repeating its
// fallback here. A card that computed the base itself would agree with the
// stalled rule by inspection and then drift the first time either moved, which
// is what the one-spelling gate exists to stop.
func idleDaysOf(deal agents.SlippingDeal, now time.Time) int {
	days := int(now.Sub(idlebase.Since(deal.CreatedAt, deal.LastActivityAt)).Hours() / 24)
	if days < 0 {
		return 0
	}
	return days
}

// decayCandidateCap bounds the candidate set the decay lane derives.
//
// It is a bound on WORK, not a display cap: the projection answers "whose
// silence is oldest" cheaply over an index, and the §4 derivation that follows
// costs a pass over each candidate's interactions. Capping between the two is
// what keeps the lane from becoming the walk over every person that the change
// engine warns against, and the oldest silences are the ones worth the passes.
const decayCandidateCap = 40

// attentionDecay reads the acting rep's own lapsed relationships.
//
// TWO steps, and the order is the design. The projection narrows to the
// reader's own edges that have been silent past the §4 threshold — one indexed
// range rather than a sweep. Only then does the people module derive what
// actually changed about those few, through the SAME engine the contact's own
// page reads, so the lane and that page cannot come to disagree about when
// somebody went quiet.
//
// A principal with no human behind it holds no relationships of its own, which
// is a refusal rather than an empty lane: the feed omits and NAMES it instead
// of reporting a clear day. Same rule the commitments lane keeps.
type attentionDecay struct {
	pool  *pgxpool.Pool
	store *people.Store
	now   func() time.Time
}

func (d attentionDecay) Lapsed(ctx context.Context) ([]attention.QuietRelationship, error) {
	// The read refuses a principal with no human of its own, so this is the
	// same refusal taken one step earlier: before a transaction is opened for
	// a lane that cannot produce a row. It is not the security boundary — that
	// lives in QuietEdgesForUser, where the reader is bound to the actor.
	if actor, ok := principal.Actor(ctx); !ok || actor.UserID.IsZero() {
		return nil, apperrors.ErrPermissionDenied
	}
	now := d.now()
	var lapsed []attention.QuietRelationship
	err := database.WithWorkspaceTx(ctx, d.pool, func(tx pgx.Tx) error {
		quiet, err := search.QuietEdgesForUser(
			ctx, tx,
			now.AddDate(0, 0, -relstrength.QuietDays),
			decayCandidateCap,
		)
		if err != nil {
			return err
		}
		candidates := make([]ids.PersonID, 0, len(quiet))
		for _, edge := range quiet {
			candidates = append(candidates, ids.From[ids.PersonKind](edge.PersonID))
		}
		changed, err := d.store.RelationshipChangesForPeople(ctx, tx, candidates, now)
		if err != nil {
			return err
		}
		lapsed = quietRelationships(quiet, changed)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return lapsed, nil
}

// quietRelationships keeps only the contacts the DERIVATION calls quiet.
//
// The projection's threshold admitted the candidates; this is the verdict. A
// candidate whose derived changes say something else — they replied, the band
// moved — is not on this lane, because the lane's sentence would then be a
// second opinion about a relationship the contact's own page describes
// differently.
//
// The walk follows the EDGES, not the derivations: the projection handed the
// candidates over oldest silence first, the derivation returns them in its own
// order, and the lane owes the rep the projection's — the longest-lapsed
// contact on top.
func quietRelationships(
	quiet []search.InteractionEdge,
	changed []people.PersonChanges,
) []attention.QuietRelationship {
	byPerson := make(map[ids.UUID]people.PersonChanges, len(changed))
	for _, row := range changed {
		byPerson[row.PersonID.UUID] = row
	}
	lapsed := make([]attention.QuietRelationship, 0, len(changed))
	for _, edge := range quiet {
		row, ok := byPerson[edge.PersonID]
		if !ok {
			continue
		}
		for _, change := range row.Changes {
			if change.Kind != relstrength.ChangeWentQuiet {
				continue
			}
			// BOTH halves of the sentence come from the derivation, never one
			// from each source. The projection folds only workspace-audience
			// activity and only this rep's own participation, while the
			// derivation folds the contact's qualifying interactions, so their
			// last-touch instants can differ. Taking the number from one and
			// the date from the other renders "quiet 46 days — last on
			// <120 days ago>": a card disagreeing with itself, and with the
			// contact's own page. `change.At` is the touch `change.Days`
			// counts from, so the two agree by construction.
			lapsed = append(lapsed, attention.QuietRelationship{
				PersonID:  row.PersonID.UUID,
				Name:      row.DisplayName,
				QuietDays: change.Days,
				LastAt:    change.At,
			})
			break
		}
	}
	return lapsed
}

// attentionWaiting binds the who-is-waiting lane to the activities module's
// own gated read. The thread walk, the discover gate and the audience arm all
// live there; nothing about who may see what is decided here.
type attentionWaiting struct {
	store *activities.Store
	now   attention.Clock
}

// The instant comes from the caller so the whole read is one snapshot. Asking
// the clock again here would let the anti-joins judge against a moment the rest
// of the day was not read at.
func (w attentionWaiting) Unanswered(ctx context.Context, asOf time.Time) ([]attention.WaitingCustomer, error) {
	rows, err := w.store.WaitingReplies(ctx, asOf)
	if err != nil {
		return nil, err
	}
	out := make([]attention.WaitingCustomer, 0, len(rows))
	for _, row := range keepWaitingCustomers(rows) {
		out = append(out, attention.WaitingCustomer{
			ActivityID:     row.ActivityID,
			Subject:        row.Subject,
			Since:          row.OccurredAt,
			PersonID:       row.PersonID,
			OrganizationID: row.OrganizationID,
			DealID:         row.DealID,
			HasOpenDeal:    row.HasOpenDeal,
			OwnerID:        row.OwnerID,
		})
	}
	return out, nil
}

// keepWaitingCustomers keeps the rows that are a PERSON waiting on this reader.
//
// Two rules, both learned from the live page.
//
// A machine is not a customer. Judged by capture's own address rule rather than
// a second one spelled here: an e-signature notification, a shared-folder
// notice and a booking confirmation opened a rep's day, and a queue that asks
// somebody to answer a no-reply address teaches them to stop reading it.
//
// One subject FROM ONE SENDER is one row. A notification service sends the same
// request on several threads, and two rows reading identically are two
// obligations to somebody scanning the page.
//
// Keyed on sender AND subject, never subject alone: two customers both writing
// "Re: proposal" are two people waiting, and folding them would drop the second
// one silently — the worst failure this queue has, because nothing on the page
// would say a customer had been hidden.
//
// An UNTITLED message is never folded, because several untitled waits are
// several customers and collapsing them would hide all but one behind an empty
// string.
func keepWaitingCustomers(rows []activities.WaitingReply) []activities.WaitingReply {
	kept := make([]activities.WaitingReply, 0, len(rows))
	seen := make(map[string]bool, len(rows))
	for _, row := range rows {
		if capture.IsMachineAddress(row.Sender) {
			continue
		}
		if row.Subject != "" {
			key := row.Sender + "\x00" + row.Subject
			if seen[key] {
				continue
			}
			seen[key] = true
		}
		kept = append(kept, row)
	}
	return kept
}

// attentionDealFacts reads deal figures through the deals store, under the
// reader's own grants: a deal they may not see is absent from the answer and
// the row simply states less about it.
type attentionDealFacts struct{ store *deals.Store }

func (f attentionDealFacts) Figures(
	ctx context.Context, dealIDs []ids.UUID,
) (map[ids.UUID]attention.DealFigures, error) {
	found, err := f.store.Figures(ctx, dealIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[ids.UUID]attention.DealFigures, len(found))
	for id, figures := range found {
		out[id] = attention.DealFigures{
			StageID:           figures.StageID,
			OwnerID:           figures.OwnerID,
			AmountMinor:       figures.AmountMinor,
			Currency:          figures.Currency,
			ExpectedCloseDate: figures.ExpectedCloseDate,
		}
	}
	return out, nil
}

// attentionTasks reads open tasks through the activities store. A task is an
// activity of kind `task`, so this is the same read the task queue makes.
type attentionTasks struct{ store *activities.Store }

func (t attentionTasks) OpenForViewer(
	ctx context.Context, until time.Time, limit int, scope attention.TaskScope, owner ids.UUID,
) ([]attention.Task, error) {
	// The store answers "open and due by then" itself, so the limit bounds the
	// rows that QUALIFY. This used to read ten times the lane and narrow
	// afterwards, which put the bound on the wrong set: a pile of completed
	// tasks filled the scan, the overdue promise underneath never reached the
	// reader, and the day rendered clear while the work was still there.
	in := activities.ListActivitiesInput{OpenAndDueBy: &until, Limit: &limit}
	// Narrowed in the QUERY, so the store's own bound applies to the rows that
	// qualify. Filtering the answer instead would let a colleague's twelve
	// tasks fill the page and hide the reader's own overdue one behind them.
	switch scope {
	case attention.TasksMine:
		actor, ok := principal.Actor(ctx)
		if !ok || actor.UserID.IsZero() {
			// No human, no "own work" to answer for. Reading every task and
			// calling the result theirs is the widening this narrowing exists
			// to prevent.
			return nil, nil
		}
		// Exactly theirs. A task they wrote themselves carries their name from
		// the moment it is written, so this needs no unassigned arm — and the
		// arm it used to have is what put an automation's follow-up on every
		// colleague's queue.
		assignee := ids.From[ids.UserKind](actor.UserID)
		in.OwnQueueOf = &assignee
	case attention.TasksUnassigned:
		in.UnassignedQueue = true
	case attention.TasksOwnedBy:
		// One named person's open work. The scope resolver already refused a
		// reader whose tier does not reach past themselves, and the store's own
		// row-scope gate still applies underneath — this narrows, never widens.
		named := ids.From[ids.UserKind](owner)
		in.OwnQueueOf = &named
	case attention.TasksVisible:
		// Every open task the reader may see; the row-scope gate in the store
		// is the only narrowing.
	}
	rows, _, err := t.store.ListActivities(ctx, in)
	if err != nil {
		return nil, err
	}
	open := make([]attention.Task, 0, len(rows))
	for _, row := range rows {
		// The filter above answers only dated rows, so this skip is unreachable
		// today. It is here because the alternative to a skip is a nil deref
		// that panics the WHOLE day's page, and the guarantee lives in a WHERE
		// clause one package away — too far for the next reader of this loop to
		// see it.
		if row.DueAt == nil {
			continue
		}
		due := *row.DueAt
		linkType, linkID := primaryLink(row)
		open = append(open, attention.Task{
			ID:       ids.UUID(row.Id),
			Subject:  subjectOfActivity(row),
			DueAt:    &due,
			LinkType: linkType,
			LinkID:   linkID,
		})
	}
	return open, nil
}

// attentionOverdue counts overdue tasks per assignee for the team board.
//
// A second reader over the same table attentionTasks lists from, and the reason
// is the bound rather than the rows: that lane stops at a dozen, so a board that
// counted its answer would report every loaded rep as holding exactly twelve.
// The store's aggregate is unbounded and answers the count itself.
type attentionOverdue struct{ store *activities.Store }

func (o attentionOverdue) OverduePerAssignee(
	ctx context.Context, asOf time.Time,
) (map[ids.UUID]int, error) {
	rows, err := o.store.OverdueLoadByAssignee(ctx, asOf)
	if err != nil {
		return nil, err
	}
	per := make(map[ids.UUID]int, len(rows))
	for _, row := range rows {
		per[row.OwnerID] = row.Overdue
	}
	return per, nil
}
