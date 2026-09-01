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
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/briefs"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/aiactivity"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/modules/overlay"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/overlaybudget"
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

// subjectOfActivity is the line a task shows. An activity always carries one
// for a task — the create path refuses a blank subject — so the fallback is
// for a row that predates that rule rather than a routine case.
func subjectOfActivity(row crmcontracts.Activity) string {
	if row.Subject != nil && *row.Subject != "" {
		return *row.Subject
	}
	return "(untitled task)"
}

// linkPriority orders the records a task may be filed under, most specific
// first. A task raised for a lead is ABOUT that lead even when the same row
// also names the company it came from, so the lead is what the reader wants to
// open. A rank absent here is a record kind this surface does not route to.
var linkPriority = map[crmcontracts.ActivityLinkEntityType]int{
	flipObjectLead: 1,
	flipObjectDeal: 2,
	crmcontracts.ActivityLinkEntityTypeProject: 3,
	flipObjectPerson:       4,
	flipObjectOrganization: 5,
}

// primaryLink picks the one record a task row points at.
//
// An activity may be filed under several, and the lane shows one row with one
// destination. Choosing by a stated priority rather than by the order the store
// happened to return them is what keeps the destination stable: the same task
// must not lead to the company on one read and the lead on the next.
//
// The links this reads are already row-scope filtered by the activities read,
// so a record the reader may not see never becomes a destination here.
func primaryLink(row crmcontracts.Activity) (string, ids.UUID) {
	if row.Links == nil {
		return "", ids.UUID{}
	}
	best := 0
	var bestType string
	var bestID ids.UUID
	for _, link := range *row.Links {
		rank, routable := linkPriority[link.EntityType]
		if !routable {
			continue
		}
		if best == 0 || rank < best {
			best = rank
			bestType = string(link.EntityType)
			bestID = ids.UUID(link.EntityId)
		}
	}
	return bestType, bestID
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
		receipt := attention.Receipt{
			ID:         ids.UUID(row.Id),
			Kind:       row.Kind,
			Summary:    summary,
			OccurredAt: *row.DecidedAt,
		}
		// Both or neither: a type with no id names nothing, and an id with no
		// type says where to look without saying at what.
		if row.TargetEntityType != nil && row.TargetEntityId != nil {
			receipt.TargetType = *row.TargetEntityType
			receipt.TargetID = ids.UUID(*row.TargetEntityId)
		}
		out = append(out, receipt)
	}
	return out
}

// attentionFailedEffects reads the decisions this rep approved whose released
// work then failed — the mark decide.go leaves on the approved row. The
// service binds the acting user itself (FailedForDecider), so this lane can
// only ever carry the reader's own decisions back to them.
type attentionFailedEffects struct{ svc *approvals.Service }

func (f attentionFailedEffects) Failed(ctx context.Context, limit int) ([]attention.FailedEffect, error) {
	rows, _, err := f.svc.ListWire(ctx, approvals.ListInput{FailedForDecider: true, Limit: limit})
	if err != nil {
		return nil, err
	}
	out := make([]attention.FailedEffect, 0, len(rows))
	for _, row := range rows {
		if row.EffectFailedAt == nil || row.EffectFailure == nil {
			// The SQL filter guarantees the pair; the re-check is what makes
			// the derefs safe in this package, where that SQL is elsewhere.
			continue
		}
		failed := attention.FailedEffect{
			ID:       ids.UUID(row.Id),
			Kind:     row.Kind,
			Sentence: *row.EffectFailure,
			FailedAt: *row.EffectFailedAt,
		}
		// Both or neither: a type with no id names nothing, and an id with no
		// type says where to look without saying at what.
		if row.TargetEntityType != nil && row.TargetEntityId != nil {
			failed.TargetType = *row.TargetEntityType
			failed.TargetID = ids.UUID(*row.TargetEntityId)
		}
		out = append(out, failed)
	}
	return out, nil
}

// attentionBriefing binds the briefing lane to the same engine entry point Home
// and the agent tool read, so all three read one queue rather than three
// readings of it.
type attentionBriefing struct {
	engine *briefs.BriefEngine
	now    attention.Clock
}

// Queue serves the acting rep's unanswered briefing entries for today, and
// whether a run exists at all.
//
// No run for today reads as an EMPTY lane with ran=false, not a refusal.
// LatestRun answers ErrNotFound both when the night has not produced one and
// when a rep is new, and neither is a permission problem — reporting them as
// a withheld lane would tell the rep something was hidden from her when
// nothing was. ran is what lets the feed tell that emptiness from a morning
// the rep finished: a found run counts as ran even with zero unanswered
// entries.
//
// Answered entries are dropped here rather than in the feed, because what the
// states mean belongs to the brief. The engine already resolves an expired
// snooze on this read, so an item whose set-aside has run out comes back
// actionable without anything here knowing that rule either.
func (a attentionBriefing) Queue(ctx context.Context) ([]attention.BriefEntry, bool, error) {
	run, err := a.engine.LatestRun(ctx, a.now())
	if errors.Is(err, apperrors.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
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
	return entries, true, nil
}

// newAttentionHandlers assembles the surface for the API role. meter is the
// Server's shared OVB meter (rebindable; overlay.go), which the sync-health
// lane's budget concern reads.
func newAttentionHandlers(pool *pgxpool.Pool, svc *approvals.Service, meter *overlaybudget.Meter) attention.Handlers {
	return attention.NewHandlers(newAttentionService(pool, svc, meter, func() time.Time { return time.Now().UTC() }))
}

// newAttentionService binds every lane to the module that owns what it shows.
//
// Separate from the handler above so a test can assemble the day through the
// SAME wiring the route serves. A test that arranged these seams itself would
// keep passing while the shipped feed lost one — which is the failure the feed's
// stub-driven unit tests already have, and the reason its producers went so long
// without a test that reads them end to end.
func newAttentionService(pool *pgxpool.Pool, svc *approvals.Service, meter *overlaybudget.Meter, now attention.Clock) *attention.Service {
	db := InstallationDB(pool)
	return attention.NewService(
		attentionApprovals{svc: svc},
		attentionDuplicates{store: people.NewStore(db)},
		attentionTasks{store: activities.NewStore(db)},
		attentionReceipts{svc: svc},
		attentionBriefing{engine: briefs.NewBriefEngine(pool, people.NewStore(db)), now: now},
		// Commitments is deliberately UNBOUND: the lane's production writer —
		// the extraction task that reads promises out of captured
		// conversations — does not exist yet (issue #849), so no real
		// installation can put a row behind it, and a lane fed only by demo
		// seeds would show every real customer an empty promise list dressed
		// as a feature. A nil binding renders the lane ABSENT (the contract's
		// honest "this feed does not do commitments"), and rebinding is the
		// one-line attentionCommitments{store: people.NewStore(db)} when #849
		// lands. The seam type stays compiled against the interface (the
		// assertion beside it in attentionlanesseam.go), and the store read
		// behind it keeps its own integration test — what is NOT tested is
		// the seam wiring itself, because nothing wires it.
		nil,
		attentionAtRisk{lister: quietDealLister(pool, deals.QuietThresholdDays)},
		attentionDecay{pool: pool, store: people.NewStore(db), now: now},
		attentionMeetings{store: activities.NewStore(db)},
		attentionFailedEffects{svc: svc},
		// The compliance clock: the open DSR cases, due-soonest first, served
		// exactly as far as consent's own DSR-admin gate reaches — the store
		// refuses everyone else and the lane renders that as withheld.
		attentionDSRs{store: consent.NewStore(db)},
		// The sync's own health, read through the module that owns the
		// mirror. Built without a vault on purpose: the health read never
		// touches a credential, and binding it here (rather than inside the
		// vault-gated overlay wiring) keeps the lane alive on every role
		// that serves the feed. A workspace not in overlay mode answers
		// ErrModeNotOverlay and the lane stays absent.
		attentionSyncHealth{svc: overlayReadService(db, nil, overlay.NewMirrorStore(db, unresolvedOwnerEmails{}), meter)},
		// The reader's own mailbox connections, through the capture module's
		// registry over the same rows the settings screen lists. Built bare —
		// no sink, no authority, no vault — so the lane lives on every role
		// that serves the feed; HealthConcerns' own doc states the reach this
		// construction depends on.
		attentionCaptureHealth{registry: capture.NewRegistry(db, nil, nil, nil)},
		// The reader's own troubled AI runs, from the same projection the
		// activity rail reads.
		attentionAIWork{store: aiactivity.NewStore(db)},
		// The reader's own sends that never arrived, from the bounce stamp
		// comms records on the row.
		attentionBounces{store: comms.NewStore(db, time.Now, activities.NewStore(db))},
		// The rules that stopped doing their work, through the automation
		// store's own gate.
		attentionAutomations{store: automation.NewAutomationStore(db)},
		// The reader's own unread notices — the durable informational line.
		attentionNotices{store: notices.NewStore(db)},
		// The label resolver: every card that names a record gets that
		// record's display name under the reader's own grants, one gated get
		// per distinct subject (attentionnames.go).
		attentionNames{
			people:     people.NewStore(db),
			deals:      deals.NewStore(db, DealsInstallation()),
			activities: activities.NewStore(db),
			projects:   projects.NewStore(db),
		},
		now,
	).WithWaiting(attentionWaiting{store: activities.NewStore(db), now: now}).
		// The reader's own sends that never left, from the stamp the
		// dispatcher's park records on the row.
		WithUndelivered(attentionUndelivered{store: comms.NewStore(db, time.Now, activities.NewStore(db))}).
		WithMachineSender(capture.IsMachineAddress).
		// The figures behind a deal a row names but does not carry — the
		// overnight brief's rows, which rank ids and keep their evidence
		// behind the brief's own endpoint.
		WithDealFacts(attentionDealFacts{store: deals.NewStore(db, DealsInstallation())})
}
