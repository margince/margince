// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Optional attention lanes read the engines that own their records.
//
// Each is a binding rather than an implementation, which is the point: the
// promises come from the contacts module's claim read, the deal risk from the
// same candidate engine whats_slipping_this_week reads, and the meetings from
// the activities list every other activity surface reads. A lane that derived
// its own answer here would be a second opinion the product would have to keep
// agreeing with.

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/contact360"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/elapsed"
	"github.com/margince/margince/backend/internal/shared/kernel/idlebase"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

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
//
// The lane reads the SERVICE's clock, the same one the day is judged and
// priced at, so one read — the hourly verdict pass included — takes its
// at-risk set, its prices and its day from a single instant.
type attentionAtRisk struct {
	lister func(context.Context) ([]agents.SlippingDeal, bool, error)
	pool   *pgxpool.Pool
	now    attention.Clock
}

func (a attentionAtRisk) Quiet(ctx context.Context) ([]attention.RiskyDeal, bool, error) {
	candidates, cut, err := a.lister(ctx)
	if err != nil {
		return nil, false, err
	}
	now := a.now().UTC()
	cover, err := a.championCover(ctx, candidates, now)
	if err != nil {
		return nil, false, err
	}
	var zone *time.Location
	if err := database.WithWorkspaceTx(ctx, a.pool, func(tx pgx.Tx) error {
		var zoneErr error
		zone, zoneErr = identity.ZoneAppliedTx(ctx, tx)
		return zoneErr
	}); err != nil {
		return nil, false, err
	}
	risky := make([]attention.RiskyDeal, 0, len(candidates))
	for _, deal := range candidates {
		risky = append(risky, attention.RiskyDeal{
			DealID:               deal.DealID,
			Name:                 deal.Name,
			StageID:              deal.StageID,
			OwnerID:              deal.OwnerID,
			AmountMinor:          deal.AmountMinor,
			Currency:             deal.Currency,
			QuietDays:            idleDaysOf(deal, now, zone),
			LastTouchedAt:        idlebase.Since(deal.CreatedAt, deal.LastActivityAt),
			CloseOverdue:         deal.CloseOverdue,
			ExpectedCloseDate:    deal.ExpectedCloseDate,
			CloseDateProvisional: deal.CloseDateProvisional,
			ForecastCategory:     deal.ForecastCategory,
			NoChampion:           cover.noChampion(deal.DealID),
			ChampionUnknown:      cover.championUnknown(deal.DealID),
			NoNextStep:           deal.NoOpenNextStep,
		})
	}
	return risky, cut, nil
}

// idleDaysOf is how long the deal has been quiet, counted from the base the
// idle RULE measures from — through idlebase.Since rather than by repeating its
// fallback here. A card that computed the base itself would agree with the
// stalled rule by inspection and then drift the first time either moved, which
// is what the one-spelling gate exists to stop.
func idleDaysOf(deal agents.SlippingDeal, now time.Time, zone *time.Location) int {
	return max(elapsed.Days(idlebase.Since(deal.CreatedAt, deal.LastActivityAt), now, zone), 0)
}

// decayCandidateCap bounds the candidate set the decay lane derives.
//
// It is a bound on WORK, not a display cap: the projection answers "whose
// silence is oldest" cheaply over an index, and the §4 derivation that follows
// costs a pass over each candidate's interactions. Capping between the two is
// what keeps the lane from becoming the walk over every contact that the change
// engine warns against, and the oldest silences are the ones worth the passes.
const decayCandidateCap = 40

// decayLaneCap is how many reconnects reach the reader. Five is what a rep can
// write in a morning; the candidate set above it is wider so the ranking has
// something to choose from.
const decayLaneCap = 5

// attentionDecay reads the lapsed relationships, routed to each contact's owner.
//
// Two steps. The projection narrows to contacts whose every live edge has been
// silent past relstrength.QuietDays. Then the contacts module derives what
// changed about those few. It uses the engine the contact's own page reads, so
// the lane and that page agree about when somebody went quiet.
//
// A principal with no human behind it is refused, not given an empty lane. The
// feed then names the lane as omitted instead of reporting a clear day.
type attentionDecay struct {
	pool  *pgxpool.Pool
	store *contacts.Store
	now   func() time.Time
}

// quietSuggestionCeilingDays is how long after the last qualifying touch a
// quiet relationship stays a suggestion. Past it the contact leaves the
// Worklist and its own page still shows the silence. A first mailbox import
// must not turn years of history into a morning of follow-ups.
const quietSuggestionCeilingDays = 180

func (d attentionDecay) Lapsed(
	ctx context.Context, scope attention.TaskScope, owner ids.UUID,
) ([]attention.QuietRelationship, bool, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID.IsZero() {
		return nil, false, apperrors.ErrPermissionDenied
	}
	now := d.now()
	// One day looser than the rule. The derivation counts calendar days in the
	// installation zone, so it can reach QuietDays before the instant does.
	read := search.QuietRead{
		QuietBefore:  now.AddDate(0, 0, -(relstrength.QuietDays - 1)),
		TouchedAfter: now.AddDate(0, 0, -quietSuggestionCeilingDays),
		Limit:        decayCandidateCap,
		// Contacts this reader set aside, and senders capture judged
		// personal, an advisor or noise, are removed before the cap. Each
		// module renders the rule over its own table, and search takes it as
		// a hole because it never imports a sibling.
		Exclude: func(arg func(any) int) (string, error) {
			dismissed, err := contacts.NotDismissedClause(ctx, "e", now, arg)
			if err != nil {
				return "", err
			}
			reader := fmt.Sprintf("$%d", arg(actor.UserID))
			return dismissed + " AND " + capture.PrivateSenderClause("e", reader), nil
		},
	}
	switch scope {
	case attention.TasksMine:
		read.Owner = actor.UserID
	case attention.TasksOwnedBy:
		if owner.IsZero() {
			return nil, false, nil
		}
		read.Owner = owner
	case attention.TasksUnassigned:
		read.Unowned = true
	case attention.TasksVisible:
	}
	var lapsed []attention.QuietRelationship
	exhausted := false
	err := database.WithWorkspaceTx(ctx, d.pool, func(tx pgx.Tx) error {
		for page := 0; page < decayCandidatePages && len(lapsed) < decayLaneCap; page++ {
			read.Offset = page * decayCandidateCap
			found, err := d.quietPage(ctx, tx, read, now)
			if err != nil {
				return err
			}
			lapsed = append(lapsed, found.lapsed...)
			if found.read < decayCandidateCap {
				exhausted = true
				break
			}
		}
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return rankReconnects(lapsed), !exhausted, nil
}

// decayCandidatePages is how many candidate pages one read may derive before
// it stops and reports the lane as possibly incomplete. Candidates the
// derivation rejects would otherwise hide a valid contact behind them.
const decayCandidatePages = 3

// quietPageResult is one candidate page: how many candidates it read, and the
// ones the derivation calls quiet.
type quietPageResult struct {
	read   int
	lapsed []attention.QuietRelationship
}

// quietPage reads one page of candidates and keeps the derived silences.
func (d attentionDecay) quietPage(
	ctx context.Context, tx pgx.Tx, read search.QuietRead, now time.Time,
) (quietPageResult, error) {
	quiet, err := search.QuietContacts(ctx, tx, read)
	if err != nil {
		return quietPageResult{}, err
	}
	candidates := make([]ids.ContactID, 0, len(quiet))
	for _, found := range quiet {
		candidates = append(candidates, ids.From[ids.ContactKind](found.Edge.ContactID))
	}
	touched, err := contact360.LastTouchFor(ctx, tx, candidates, contact360.AssembleOptions{})
	if err != nil {
		return quietPageResult{}, err
	}
	touches := make(map[ids.ContactID]relstrength.Touches, len(touched))
	for id, touch := range touched {
		touches[id] = touch.Touches()
	}
	zone, err := identity.ZoneAppliedTx(ctx, tx)
	if err != nil {
		return quietPageResult{}, err
	}
	changed, err := d.store.RelationshipChangesForContacts(ctx, tx, candidates, now, touches, zone)
	if err != nil {
		return quietPageResult{}, err
	}
	// Which of them still carries money, read in the same transaction. A
	// reader without the deal grant loses this one fact and keeps the row.
	funded, err := deals.OpenDealContacts(ctx, tx, candidates)
	if err != nil && !errors.Is(err, apperrors.ErrPermissionDenied) {
		return quietPageResult{}, err
	}
	return quietPageResult{read: len(quiet), lapsed: quietSurvivors(quiet, changed, funded, now)}, nil
}

// quietRelationships keeps only the contacts the derivation calls quiet, and
// only while their last qualifying touch is inside the suggestion ceiling.
//
// The walk follows the edges, so the projection's order holds. The number and
// the date both come from the derivation: `change.Days` counts from
// `change.At`. The ceiling is measured from that same touch.
func quietRelationships(
	quiet []search.QuietContact,
	changed []contacts.ContactChanges,
	funded map[ids.UUID]bool,
	now time.Time,
) []attention.QuietRelationship {
	return rankReconnects(quietSurvivors(quiet, changed, funded, now))
}

// quietSurvivors is quietRelationships before the ranking and the lane cap.
func quietSurvivors(
	quiet []search.QuietContact,
	changed []contacts.ContactChanges,
	funded map[ids.UUID]bool,
	now time.Time,
) []attention.QuietRelationship {
	byContact := make(map[ids.UUID]contacts.ContactChanges, len(changed))
	for _, row := range changed {
		byContact[row.ContactID.UUID] = row
	}
	ceiling := now.AddDate(0, 0, -quietSuggestionCeilingDays)
	lapsed := make([]attention.QuietRelationship, 0, len(changed))
	for _, found := range quiet {
		edge := found.Edge
		row, ok := byContact[edge.ContactID]
		if !ok {
			continue
		}
		for _, change := range row.Changes {
			if change.Kind != relstrength.ChangeWentQuiet {
				continue
			}
			if !change.At.After(ceiling) {
				break
			}
			lapsed = append(lapsed, attention.QuietRelationship{
				ContactID:   row.ContactID.UUID,
				Name:        row.DisplayName,
				OwnerID:     found.OwnerID,
				QuietDays:   change.Days,
				LastAt:      change.At,
				Strength:    edge.StrengthOf(now),
				HasOpenDeal: funded[edge.ContactID],
			})
			break
		}
	}
	return lapsed
}

// rankReconnects puts the relationships worth reviving first and cuts to what a
// rep can actually act on in a morning.
//
// Money leads, then how strong the relationship was, then how long it has been
// quiet. The projection hands these over most-exchanged first, which is the
// right CANDIDATE order — it is what stops a one-off exchange from years ago
// crowding out a real lapse — but it says nothing about which of the survivors
// matters most to this rep today.
//
// The cap is the product rule: five reconnects a rep can write. The rest are
// not lost, they return tomorrow as the edges age, and a lane of forty names
// under a heading promising new revenue is one a reader learns to skip.
func rankReconnects(lapsed []attention.QuietRelationship) []attention.QuietRelationship {
	sort.SliceStable(lapsed, func(i, j int) bool {
		a, b := lapsed[i], lapsed[j]
		if a.HasOpenDeal != b.HasOpenDeal {
			return a.HasOpenDeal
		}
		if a.Strength.Strength != b.Strength.Strength {
			return a.Strength.Strength > b.Strength.Strength
		}
		return a.QuietDays > b.QuietDays
	})
	if len(lapsed) > decayLaneCap {
		return lapsed[:decayLaneCap]
	}
	return lapsed
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
			Closed:               figures.Closed,
			StageID:              figures.StageID,
			StageWinProbability:  figures.StageWinProbability,
			OwnerID:              figures.OwnerID,
			AmountMinor:          figures.AmountMinor,
			Currency:             figures.Currency,
			ExpectedCloseDate:    figures.ExpectedCloseDate,
			CloseDateProvisional: figures.CloseDateProvisional,
			ForecastCategory:     figures.ForecastCategory,
			CloseOverdue:         figures.CloseOverdue,
		}
	}
	return out, nil
}

// taskFromActivity carries one stored activity across the seam as a task.
//
// A function rather than a loop body so the crossing can be tested without a
// store: every field here is one the activity already held and this seam once
// dropped, and a copy that stops happening is invisible — the lane still
// returns the right NUMBER of rows, each one just quietly missing a fact.
//
// reader is whose reminder the row may be, zero for none. A row whose content
// is withheld is never raised, as the reminder read leaves it out.
func taskFromActivity(row crmcontracts.Activity, reader ids.UUID, asOf time.Time) attention.Task {
	linkType, linkID := primaryLink(row)
	task := attention.Task{
		LeadResponseEscalation: row.SourceSystem != nil && *row.SourceSystem == leadSLATaskSource,
		ID:                     ids.UUID(row.Id),
		Subject:                subjectOfActivity(row),
		DueAt:                  row.DueAt,
		LinkType:               linkType,
		LinkID:                 linkID,
		ReminderDue:            contentReadable(row) && activities.ReminderDueFor(row, reader, asOf),
	}
	if row.SourceActivityId != nil {
		task.SourceActivityID = new(ids.UUID(*row.SourceActivityId))
	}
	// Who holds it, already on the row the store returned and dropped here.
	// Two of this lane's three scopes put somebody else's task in front of the
	// reader, and one of them exists precisely to surface the tasks nobody has
	// taken.
	if row.AssigneeId != nil {
		assignee := ids.UUID(*row.AssigneeId)
		task.AssigneeID = &assignee
	}
	// And the version, for the same reason and from the same row: the lane's
	// verbs write to this task, and a press that names no version is one
	// somebody else's press can overwrite unannounced.
	if row.Version != nil {
		version := int64(*row.Version)
		task.Version = &version
	}
	return task
}

func contentReadable(row crmcontracts.Activity) bool {
	return row.ContentState == nil || *row.ContentState != crmcontracts.ActivityContentStateWithheld
}

// attentionOverdue counts overdue tasks per assignee for the team board.
//
// A second reader over the same table attentionTasks lists from, and the reason
// is the bound rather than the rows: that lane stops at a dozen, so a board that
// counted its answer would report every loaded rep as holding exactly twelve.
// The store's aggregate is unbounded and answers the count itself.
type attentionOverdue struct{ store *activities.Store }

func (o attentionOverdue) OverduePerAssignee(ctx context.Context, asOf time.Time) (map[ids.UUID]int, error) {
	rows, err := o.store.OverdueLoadByAssignee(ctx, asOf)
	return taskLoadMap(rows, err)
}

func (o attentionOverdue) CommitmentsDuePerAssignee(ctx context.Context, asOf time.Time) (map[ids.UUID]int, error) {
	rows, err := o.store.CommitmentLoadByAssignee(ctx, asOf)
	return taskLoadMap(rows, err)
}

func taskLoadMap(rows []activities.OverdueLoad, err error) (map[ids.UUID]int, error) {
	if err != nil {
		return nil, err
	}
	per := make(map[ids.UUID]int, len(rows))
	for _, row := range rows {
		per[row.OwnerID] = row.Overdue
	}
	return per, nil
}

// CountOpenForViewer answers how many tasks the lane's own narrowing matches,
// with no cap on it — the badge beside a bounded page.
//
// The same openTasksDueBy the page is built from, so the number and the cards
// cannot come to answer different questions. A second statement, which is what a
// true total beside a bounded page costs and what needs_you has always paid.
func (t attentionTasks) CountOpenForViewer(
	ctx context.Context, asOf, until time.Time, scope attention.TaskScope, owner ids.UUID,
) (int, error) {
	in, ok := openTasksDueBy(ctx, asOf, until, scope, owner)
	if !ok {
		return 0, nil
	}
	// The reminders the page lists ahead of the day are counted with it.
	in.OrRemindersDue = scope == attention.TasksMine
	return t.store.CountActivities(ctx, in)
}

// attentionPins reads the acting reader's own pinned rows.
//
// A pass-through in its own transaction: the rows are the module's and the
// identity they carry is the client's, so nothing is translated here beyond the
// vocabulary each side owns.
type attentionPins struct {
	pool  *pgxpool.Pool
	store *activities.Store
}

func (p attentionPins) PinnedRows(ctx context.Context) (map[attention.RowRef]bool, error) {
	var out map[attention.RowRef]bool
	err := database.WithWorkspaceTx(ctx, p.pool, func(tx pgx.Tx) error {
		pinned, err := p.store.PinnedRows(ctx, tx)
		if err != nil {
			return err
		}
		out = make(map[attention.RowRef]bool, len(pinned))
		for ref := range pinned {
			out[attention.RowRef{Source: ref.Source, RowID: ref.RowID}] = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
