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
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/briefs"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/deadline"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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

// attentionTasks reads open tasks through the activities store. A task is an
// activity of kind `task`, so this is the same read the task queue makes.
type attentionTasks struct{ store *activities.Store }

func (t attentionTasks) OpenForViewer(ctx context.Context, until time.Time, limit int) ([]attention.Task, error) {
	// The store answers "open and due by then" itself, so the limit bounds the
	// rows that QUALIFY. This used to read ten times the lane and narrow
	// afterwards, which put the bound on the wrong set: a pile of completed
	// tasks filled the scan, the overdue promise underneath never reached the
	// reader, and the day rendered clear while the work was still there.
	rows, _, err := t.store.ListActivities(ctx, activities.ListActivitiesInput{
		OpenAndDueBy: &until, Limit: &limit,
	})
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
// The test is the decision's own decided_by_system marker. It used to be
// decided_by IS NULL, inferring "nobody decided" from an empty column, and that
// read the wrong thing twice over: no writer produces approved-with-no-decider,
// and deleting an app_user empties decided_by on every approval that person
// decided — which would move their decisions into a lane headed "Done for you".
// Filtering on status alone would do the same thing to every reader's own
// approvals, which is the one claim this lane exists to make.
type attentionReceipts struct{ svc *approvals.Service }

func (r attentionReceipts) Recent(ctx context.Context, since time.Time, limit int) ([]attention.Receipt, error) {
	return recentReceipts(since, limit, func(scan int) ([]crmcontracts.Approval, error) {
		status := approvalStatusApproved
		bySystem := true
		rows, _, err := r.svc.ListWire(ctx, approvals.ListInput{
			Status: &status, DecidedBySystem: &bySystem, DecidedAfter: &since, Limit: scan,
		})
		return rows, err
	})
}

// recentReceipts turns the store's rows into the lane's cards.
//
// The read is bounded by the lane rather than widened past it: the store answers
// "approved, decided by the system, decided since" itself, so the limit applies
// to rows that qualify. The window belongs in SQL with the rest — the page is
// ordered by created_at while the window is about decided_at, so a window
// applied afterwards can discard a whole page and hide a decision made minutes
// ago beneath approvals staged more recently.
//
// The re-check below is not a second filter. It is what makes the deref of
// DecidedAt safe in this package, where the SQL guaranteeing it is elsewhere.
//
// The page reader is a parameter so a test can answer exactly the width it was
// asked for; nothing else varies it.
func recentReceipts(
	since time.Time, limit int, page func(scan int) ([]crmcontracts.Approval, error),
) ([]attention.Receipt, error) {
	rows, err := page(limit)
	if err != nil {
		return nil, err
	}
	return receiptsWithin(rows, since), nil
}

// receiptsWithin keeps the decided rows inside the lane's window.
func receiptsWithin(rows []crmcontracts.Approval, since time.Time) []attention.Receipt {
	out := make([]attention.Receipt, 0, len(rows))
	for _, row := range rows {
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

// newAttentionHandlers assembles the surface for the API role.
func newAttentionHandlers(pool *pgxpool.Pool, svc *approvals.Service) attention.Handlers {
	return attention.NewHandlers(newAttentionService(pool, svc, func() time.Time { return time.Now().UTC() }))
}

// newAttentionService binds every lane to the module that owns what it shows.
//
// Separate from the handler above so a test can assemble the day through the
// SAME wiring the route serves. A test that arranged these seams itself would
// keep passing while the shipped feed lost one — which is the failure the feed's
// stub-driven unit tests already have, and the reason its producers went so long
// without a test that reads them end to end.
func newAttentionService(pool *pgxpool.Pool, svc *approvals.Service, now attention.Clock) *attention.Service {
	db := InstallationDB(pool)
	return attention.NewService(
		attentionApprovals{svc: svc},
		attentionDuplicates{store: people.NewStore(db)},
		attentionTasks{store: activities.NewStore(db)},
		attentionReceipts{svc: svc},
		attentionBriefing{engine: briefs.NewBriefEngine(pool, people.NewStore(db)), now: now},
		attentionCommitments{store: people.NewStore(db)},
		attentionAtRisk{lister: quietDealLister(pool, deals.QuietThresholdDays)},
		attentionMeetings{store: activities.NewStore(db)},
		now,
	)
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
