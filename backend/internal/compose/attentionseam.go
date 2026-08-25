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
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gradionhq/margince/backend/internal/compose/attention"
	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/modules/activities"
	"github.com/gradionhq/margince/backend/internal/modules/approvals"
	"github.com/gradionhq/margince/backend/internal/modules/people"
	"github.com/gradionhq/margince/backend/internal/platform/database/storekit"
	"github.com/gradionhq/margince/backend/internal/shared/apperrors"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/deadline"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
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

// newAttentionHandlers assembles the surface for the API role.
func newAttentionHandlers(pool *pgxpool.Pool, svc *approvals.Service) attention.Handlers {
	db := InstallationDB(pool)
	return attention.NewHandlers(attention.NewService(
		attentionApprovals{svc: svc},
		attentionDuplicates{store: people.NewStore(db)},
		attentionTasks{store: activities.NewStore(db)},
		attentionReceipts{svc: svc},
		func() time.Time { return time.Now().UTC() },
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
