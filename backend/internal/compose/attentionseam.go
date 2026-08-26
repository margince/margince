// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The day's surface, wired to the modules that own what it shows.
//
// attention is a compose subpackage and approvals, people and activities are
// modules, so every edge between them is bound here like any other cross-module
// edge. What crosses is four READS. No verb does: a card's approve, complete or
// merge goes to the endpoint that already owns it, so this surface can never
// become a second place where a decision's rules live.

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gradionhq/margince/backend/internal/compose/attention"
	"github.com/gradionhq/margince/backend/internal/compose/briefs"
	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/modules/activities"
	"github.com/gradionhq/margince/backend/internal/modules/agents"
	"github.com/gradionhq/margince/backend/internal/modules/approvals"
	"github.com/gradionhq/margince/backend/internal/modules/deals"
	"github.com/gradionhq/margince/backend/internal/modules/people"
	"github.com/gradionhq/margince/backend/internal/platform/database/storekit"
	"github.com/gradionhq/margince/backend/internal/shared/apperrors"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/deadline"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/principal"
)

// attentionApprovals reads the staged queue through the engine every approval
// surface reads, so the feed shows what the inbox shows.
type attentionApprovals struct{ svc *approvals.Service }

func (a attentionApprovals) ListWire(ctx context.Context, in attention.ApprovalQuery) ([]crmcontracts.Approval, error) {
	status := in.Status
	rows, _, err := a.svc.ListWire(ctx, approvals.ListInput{Status: &status, Limit: in.Limit})
	return rows, err
}

// CountPending is how many staged proposals this caller could decide.
//
// Counted by reading a page rather than with a COUNT, because decidability is a
// per-row probe and no SQL count can apply it. approvals.PendingScanCap bounds
// that read and its own comment states the contract this inherits: a full
// result means "this many or more". The lane it feeds is bounded far below the
// cap, so the number stops being exact only once it is already large enough to
// mean the same thing to a reader.
func (a attentionApprovals) CountPending(ctx context.Context) (int, error) {
	status := "pending"
	rows, _, err := a.svc.ListWire(ctx, approvals.ListInput{
		Status: &status,
		Limit:  approvals.PendingScanCap,
	})
	if err != nil {
		return 0, err
	}
	return len(rows), nil
}

// attentionDuplicates reads the dedupe queue through the people store, which
// applies the both-sides-visible rule to the page and the count alike.
type attentionDuplicates struct{ store *people.Store }

func (d attentionDuplicates) OpenCandidates(ctx context.Context, limit int) ([]attention.DuplicatePair, error) {
	rows, _, err := d.store.ListDedupeCandidates(ctx, people.DedupeQueueInput{Limit: limit})
	if err != nil {
		return nil, err
	}
	pairs := make([]attention.DuplicatePair, 0, len(rows))
	for _, row := range rows {
		pairs = append(pairs, attention.DuplicatePair{
			ID:         row.ID,
			EntityType: row.EntityType,
			Confidence: row.Confidence,
			LeftID:     row.LeftID,
			RightID:    row.RightID,
			Evidence:   comparisons(ctx, row.ID, row.Evidence),
		})
	}
	return pairs, nil
}

// dedupeEvidenceRow is the detection-time snapshot as the queue stores it.
type dedupeEvidenceRow struct {
	Field      string  `json:"field"`
	LeftValue  *string `json:"left_value"`
	RightValue *string `json:"right_value"`
	Signal     string  `json:"signal"`
}

// comparisons decodes the stored snapshot.
//
// A snapshot that will not decode yields NO evidence rather than an error: the
// pair is still a real decision, and the two records beside each other are the
// larger part of the answer. Losing the field table degrades the card; refusing
// the whole lane over one malformed row would hide every other decision behind
// it.
func comparisons(ctx context.Context, candidate ids.UUID, raw json.RawMessage) []attention.FieldComparison {
	if len(raw) == 0 {
		return nil
	}
	var rows []dedupeEvidenceRow
	if err := json.Unmarshal(raw, &rows); err != nil {
		// Degrade for the reader, but say so. A snapshot that will not parse
		// means a detector wrote something nothing can read, and this is the
		// only place that would ever notice: an empty comparison and a corrupt
		// one look identical on screen, and forever.
		slog.WarnContext(ctx, "attention: dedupe evidence snapshot will not parse",
			"candidate_id", candidate.String(), "error", err)
		return nil
	}
	out := make([]attention.FieldComparison, 0, len(rows))
	for _, row := range rows {
		out = append(out, attention.FieldComparison{
			Field:  row.Field,
			Left:   row.LeftValue,
			Right:  row.RightValue,
			Signal: row.Signal,
		})
	}
	return out
}

// Describe names one side of a pair, under the reader's own scope.
//
// Each branch is that record's ordinary get, so a reader who may not see the
// record gets the same refusal here as anywhere else. The pair's own row is not
// permission to read what it points at.
func (d attentionDuplicates) Describe(
	ctx context.Context, entityType string, id ids.UUID,
) (attention.RecordFace, error) {
	switch entityType {
	case flipObjectPerson:
		row, err := d.store.GetPerson(ctx, ids.From[ids.PersonKind](id), storekit.LiveOnly)
		if err != nil {
			return attention.RecordFace{}, err
		}
		return personFace(row), nil
	case flipObjectOrganization:
		row, err := d.store.GetOrganization(ctx, ids.From[ids.OrganizationKind](id), storekit.LiveOnly)
		if err != nil {
			return attention.RecordFace{}, err
		}
		return organizationFace(row), nil
	case flipObjectLead:
		row, err := d.store.GetLead(ctx, ids.From[ids.LeadKind](id), storekit.LiveOnly)
		if err != nil {
			return attention.RecordFace{}, err
		}
		return leadFace(row), nil
	default:
		return attention.RecordFace{}, apperrors.ErrNotFound
	}
}

func (d attentionDuplicates) CountOpen(ctx context.Context) (int, error) {
	return d.store.CountOpenDedupeCandidates(ctx)
}

// taskScanFactor is how much wider than the lane the task read goes.
//
// The store returns tasks by recency regardless of whether they are done or
// due, and this seam filters afterwards, so the scan has to carry enough rows
// for the filter to still find today's work under a pile of finished ones. Ten
// times the lane is a guess with a floor rather than a measurement: what it
// must not be is EQUAL to the lane, which is what silently dropped overdue
// work.
const taskScanFactor = 10

// attentionTasks reads open tasks through the activities store. A task is an
// activity of kind `task`, so this is the same read the task queue makes.
type attentionTasks struct{ store *activities.Store }

func (t attentionTasks) OpenForViewer(ctx context.Context, until time.Time, limit int) ([]attention.Task, error) {
	kind := activityKindTask
	// Read WIDER than the lane shows, because the store cannot filter on
	// "open and due by now" and this does the filtering afterwards. Asking for
	// exactly the lane's size let a dozen completed or future-dated tasks fill
	// the page and push a genuinely overdue one off it — the day would read
	// clear while the task queue still showed the promise it had missed.
	scan := limit * taskScanFactor
	rows, _, err := t.store.ListActivities(ctx, activities.ListActivitiesInput{Kind: &kind, Limit: &scan})
	if err != nil {
		return nil, err
	}
	open := make([]attention.Task, 0, len(rows))
	for _, row := range rows {
		if row.IsDone != nil && *row.IsDone {
			continue
		}
		// A task with no due date is still work somebody agreed to, but it is
		// not work for TODAY, and a queue that promised today's list would be
		// lying if it carried the undated backlog too. `until` is the end of
		// the day, so anything not yet past it is still ahead of the reader.
		if row.DueAt == nil || !deadline.Passed(row.DueAt, until) {
			continue
		}
		due := *row.DueAt
		open = append(open, attention.Task{
			ID:      ids.UUID(row.Id),
			Subject: subjectOfActivity(row),
			DueAt:   &due,
		})
	}
	return open, nil
}

// subjectOfActivity is the line a task shows. An activity always carries one
// for a task — the create path refuses a blank subject — so the fallback is
// for a row that predates that rule rather than a routine case.
func subjectOfActivity(row crmcontracts.Activity) string {
	if row.Subject != nil && *row.Subject != "" {
		return *row.Subject
	}
	return "(untitled task)"
}

// approvalStatusApproved is the decided status a receipt is read from. Spelled
// once here because the receipt lane asks for it by name and a typo would
// quietly return an empty lane rather than an error.
const approvalStatusApproved = "approved"

// attentionReceipts reads what ran without asking.
//
// The test is decided_by IS NULL, which is the convention the expiry sweep
// states in its own words: "decided_by stays NULL and the actor is the system:
// nobody decided". Filtering on status alone would put the reader's OWN
// approvals in a lane headed "Done for you" — telling somebody the system
// handled a thing they handled themselves, which is the one claim this lane
// exists to make and the easiest one to get wrong.
type attentionReceipts struct{ svc *approvals.Service }

func (r attentionReceipts) Recent(ctx context.Context, since time.Time, limit int) ([]attention.Receipt, error) {
	return recentReceipts(since, limit, func(scan int) ([]crmcontracts.Approval, error) {
		status := approvalStatusApproved
		rows, _, err := r.svc.ListWire(ctx, approvals.ListInput{Status: &status, Limit: scan})
		return rows, err
	})
}

// recentReceipts pages the approved queue and keeps what ran without asking.
//
// The read is WIDER than the lane shows, and that is the whole of this
// function's reason to exist: the engine can only page approvals by status,
// while `decided_by IS NULL` — the test for "nobody was asked" — is applied
// afterwards. A read the size of the lane is filled by the reader's OWN recent
// approvals and filters to nothing, so the lane reports a quiet night while
// dozens of things ran. The same shape as the task lane's scan factor, and for
// the same reason.
//
// The page reader is a parameter so a test can answer exactly the width it was
// asked for; nothing else varies it.
func recentReceipts(
	since time.Time, limit int, page func(scan int) ([]crmcontracts.Approval, error),
) ([]attention.Receipt, error) {
	rows, err := page(approvals.PendingScanCap)
	if err != nil {
		return nil, err
	}
	receipts := receiptsWithin(rows, since)
	if len(receipts) > limit {
		receipts = receipts[:limit]
	}
	return receipts, nil
}

// receiptsWithin keeps the decided rows this lane may claim: inside the window,
// and decided by nobody.
func receiptsWithin(rows []crmcontracts.Approval, since time.Time) []attention.Receipt {
	out := make([]attention.Receipt, 0, len(rows))
	for _, row := range rows {
		// A human's own decision is not something that ran for them.
		if row.DecidedBy != nil {
			continue
		}
		// Inside the window, not before it: `since` is the receipt lane's own
		// horizon, and the same authority answers "is this behind that" here as
		// answers it for a task's due date.
		if row.DecidedAt == nil || deadline.Passed(row.DecidedAt, since) {
			continue
		}
		summary := ""
		if row.Summary != nil {
			summary = *row.Summary
		}
		out = append(out, attention.Receipt{
			ID:         ids.UUID(row.Id),
			Kind:       row.Kind,
			Summary:    summary,
			OccurredAt: *row.DecidedAt,
		})
	}
	return out
}

// attentionBriefing binds the briefing lane to the same engine entry point Home
// and the agent tool read, so all three read one queue rather than three
// readings of it.
type attentionBriefing struct {
	engine *briefs.BriefEngine
	now    attention.Clock
}

// Queue serves the acting rep's unanswered briefing entries for today.
//
// No run for today reads as an EMPTY lane, not a refusal. LatestRun answers
// ErrNotFound both when the night has not produced one and when a rep is new,
// and neither is a permission problem — reporting them as a withheld lane
// would tell the rep something was hidden from her when nothing was.
//
// Answered entries are dropped here rather than in the feed, because what the
// states mean belongs to the brief. The engine already resolves an expired
// snooze on this read, so an item whose set-aside has run out comes back
// actionable without anything here knowing that rule either.
func (a attentionBriefing) Queue(ctx context.Context) ([]attention.BriefEntry, error) {
	run, err := a.engine.LatestRun(ctx, a.now())
	if errors.Is(err, apperrors.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	entries := make([]attention.BriefEntry, 0, len(run.Items))
	for _, item := range run.Items {
		if !briefs.Unanswered(item) {
			continue
		}
		entries = append(entries, attention.BriefEntry{
			ID: item.ID, DealID: item.DealID, Rank: item.Rank,
		})
	}
	return entries, nil
}

// attentionCommitments reads the acting rep's own promises through the people
// store.
//
// A claim carries no assignee, so ownership rides the person it was made to:
// the rep who holds the relationship is the one who made the promise in their
// own captured conversation. A principal with no human behind it has no
// promises of its own to keep, which is a refusal rather than an empty lane —
// the feed omits and NAMES the lane instead of reporting a clear day.
type attentionCommitments struct{ store *people.Store }

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
			QuietDays:         idleDaysOf(deal, now),
			CloseOverdue:      deal.CloseOverdue,
			ExpectedCloseDate: deal.ExpectedCloseDate,
		})
	}
	return risky, nil
}

// idleDaysOf is how long the deal has been quiet, counted from the same base
// the idle rule itself measures from: the last activity, or the deal's creation
// when nothing has ever touched it.
func idleDaysOf(deal agents.SlippingDeal, now time.Time) int {
	since := deal.CreatedAt
	if deal.LastActivityAt != nil {
		since = *deal.LastActivityAt
	}
	days := int(now.Sub(since).Hours() / 24)
	if days < 0 {
		return 0
	}
	return days
}

// attentionMeetings reads today's remaining meetings through the activities
// store — the same gated list every other activity surface reads.
//
// SCAN AND FILTER, like the task lane above and for the same reason: the store
// cannot express "kind=meeting between two instants", so this reads wider and
// narrows here. The same scan factor applies, because a day with a pile of
// finished meetings would otherwise push the one still ahead off the page.
type attentionMeetings struct{ store *activities.Store }

func (m attentionMeetings) Today(
	ctx context.Context, from, until time.Time, limit int,
) ([]attention.Meeting, error) {
	kind := string(crmcontracts.ActivityKindMeeting)
	scan := limit * taskScanFactor
	rows, _, err := m.store.ListActivities(ctx, activities.ListActivitiesInput{Kind: &kind, Limit: &scan})
	if err != nil {
		return nil, err
	}
	ahead := make([]attention.Meeting, 0, len(rows))
	for _, row := range rows {
		if !meetingStillWorthPreparing(row, from, until) {
			continue
		}
		ahead = append(ahead, attention.Meeting{
			ID: ids.UUID(row.Id), Subject: subjectOfMeeting(row), StartsAt: row.OccurredAt,
		})
	}
	// Soonest first: the lane is a countdown, and the store returns activities
	// newest-first, which is the opposite order for a day still ahead.
	sort.SliceStable(ahead, func(i, j int) bool { return ahead[i].StartsAt.Before(ahead[j].StartsAt) })
	if len(ahead) > limit {
		ahead = ahead[:limit]
	}
	return ahead, nil
}

// meetingStillWorthPreparing keeps the meetings a rep can still do something
// about: booked (not held, not cancelled, not a no-show) and starting between
// now and the end of the day.
//
// A meeting with no status is treated as booked. Capture writes calendar events
// without one, and dropping them would empty this lane on exactly the
// installations whose calendars are connected.
func meetingStillWorthPreparing(row crmcontracts.Activity, from, until time.Time) bool {
	if row.MeetingStatus != nil && *row.MeetingStatus != crmcontracts.ActivityMeetingStatusBooked {
		return false
	}
	return !row.OccurredAt.Before(from) && row.OccurredAt.Before(until)
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

// newAttentionHandlers assembles the surface for the API role.
func newAttentionHandlers(pool *pgxpool.Pool, svc *approvals.Service) attention.Handlers {
	db := InstallationDB(pool)
	now := func() time.Time { return time.Now().UTC() }
	return attention.NewHandlers(attention.NewService(
		attentionApprovals{svc: svc},
		attentionDuplicates{store: people.NewStore(db)},
		attentionTasks{store: activities.NewStore(db)},
		attentionReceipts{svc: svc},
		attentionBriefing{engine: briefs.NewBriefEngine(pool, people.NewStore(db)), now: now},
		attentionCommitments{store: people.NewStore(db)},
		attentionAtRisk{lister: quietDealLister(pool, deals.QuietThresholdDays)},
		attentionMeetings{store: activities.NewStore(db)},
		now,
	))
}

// The three faces a merge decision compares.
//
// Each answers the same two questions in that record's own terms: which one is
// this, and which side carries more. `detail` is the field a reader actually
// uses to tell two near-identical records apart — a company's domain, a
// person's address — never an id.

func organizationFace(row crmcontracts.Organization) attention.RecordFace {
	face := attention.RecordFace{
		Label:        row.DisplayName,
		CreatedAt:    &row.CreatedAt,
		RelatedCount: row.ContactCount,
	}
	if row.Domains != nil && len(*row.Domains) > 0 {
		face.Detail = (*row.Domains)[0].Domain
	}
	return face
}

func personFace(row crmcontracts.Person) attention.RecordFace {
	face := attention.RecordFace{Label: row.FullName, CreatedAt: &row.CreatedAt}
	if row.Emails != nil && len(*row.Emails) > 0 {
		face.Detail = string((*row.Emails)[0].Email)
	}
	return face
}

func leadFace(row crmcontracts.Lead) attention.RecordFace {
	face := attention.RecordFace{CreatedAt: &row.CreatedAt}
	if row.FullName != nil {
		face.Label = *row.FullName
	}
	switch {
	case row.Email != nil:
		face.Detail = string(*row.Email)
	case row.CompanyName != nil:
		face.Detail = *row.CompanyName
	}
	return face
}
