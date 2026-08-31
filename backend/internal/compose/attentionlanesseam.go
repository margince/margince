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
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/attention"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/agents"
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
type attentionAtRisk struct{ lister agents.SlippingLister }

func (a attentionAtRisk) Quiet(ctx context.Context) ([]attention.RiskyDeal, error) {
	candidates, err := a.lister(ctx)
	if err != nil {
		return nil, err
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
	return risky, nil
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
	for _, row := range rows {
		out = append(out, attention.WaitingCustomer{
			ActivityID:     row.ActivityID,
			Subject:        row.Subject,
			Since:          row.OccurredAt,
			PersonID:       row.PersonID,
			OrganizationID: row.OrganizationID,
			DealID:         row.DealID,
		})
	}
	return out, nil
}

// attentionMeetings reads today's remaining meetings through the activities
// store — the same gated list every other activity surface reads.
//
// The WINDOW is applied in SQL, not here. An earlier cut read ten times the
// lane and narrowed the time range in Go, which is lossy in the one direction
// that hides itself: a day with more than the scan's worth of later activity
// pushes a real meeting off the page, and the lane draws a free afternoon over
// a booked one. ListActivitiesInput carries OccurredAfter/OccurredBefore and
// the store applies both as predicates, so the bound is the day rather than a
// guess about how busy the day might be.
//
// The STATUS filter stays in Go: the store has no dial for it, and the set it
// removes is bounded by the window the database already applied.
type attentionMeetings struct{ store *activities.Store }

func (m attentionMeetings) Today(
	ctx context.Context, from, until time.Time, limit int,
) ([]attention.Meeting, error) {
	kind := string(crmcontracts.ActivityKindMeeting)
	rows, _, err := m.store.ListActivities(ctx, activities.ListActivitiesInput{
		Kind: &kind, OccurredAfter: &from, OccurredBefore: &until, Limit: &limit,
	})
	if err != nil {
		return nil, err
	}
	ahead := make([]attention.Meeting, 0, len(rows))
	for _, row := range rows {
		if !meetingStillWorthPreparing(row) {
			continue
		}
		ahead = append(ahead, attention.Meeting{
			ID: ids.UUID(row.Id), Subject: subjectOfMeeting(row), StartsAt: row.OccurredAt,
		})
	}
	// Soonest first: the lane is a countdown, and the store returns activities
	// newest-first, which is the opposite order for a day still ahead.
	sort.SliceStable(ahead, func(i, j int) bool { return ahead[i].StartsAt.Before(ahead[j].StartsAt) })
	return ahead, nil
}

// meetingStillWorthPreparing keeps the meetings a rep can still do something
// about: booked, rather than held, cancelled or a no-show. The time window is
// the database's to apply.
//
// A meeting with no status is treated as booked. Capture writes calendar events
// without one, and dropping them would empty this lane on exactly the
// installations whose calendars are connected.
func meetingStillWorthPreparing(row crmcontracts.Activity) bool {
	return row.MeetingStatus == nil || *row.MeetingStatus == crmcontracts.ActivityMeetingStatusBooked
}

// subjectOfMeeting is the line a meeting shows. Unlike a task, a meeting may
// honestly have no subject — a calendar event with a blank title is a real
// thing a provider hands over — so the fallback is a routine case here.
func subjectOfMeeting(row crmcontracts.Activity) string {
	if row.Subject != nil && *row.Subject != "" {
		return *row.Subject
	}
	return "(untitled meeting)"
}
