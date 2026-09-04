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
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/briefs"
	"github.com/margince/margince/backend/internal/compose/worklistsnap"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/aiactivity"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/introductions"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/modules/overlay"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/overlaybudget"
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
		attentionBriefing{
			engine: briefs.NewBriefEngine(pool, people.NewStore(db)),
			// The same reader WithDealFacts binds below, so the lane keeps an
			// entry exactly when the figures pass can state its deal.
			figures: attentionDealFacts{store: deals.NewStore(db, DealsInstallation())},
			now:     now,
		},
		// Commitments is bound now that claims have a writer. It was nil while
		// nothing could put a row behind the lane: a lane fed only by demo
		// seeds would have shown every real customer an empty promise list
		// dressed as a feature, and absent is the honest rendering of "this
		// feed does not do commitments".
		//
		// That is no longer true. POST /people/{id}/claims writes through
		// people.RecordConversationClaim, and the transcript reader files what
		// a reader accepts from a meeting. A promise a rep made is then real
		// data the queue was still refusing to show.
		attentionCommitments{store: people.NewStore(db)},
		attentionAtRisk{lister: quietDealScan(pool, deals.QuietThresholdDays)},
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
		newAttentionNames(db),
		now,
		// The installation's own midnight is where "today" ends for the
		// due-dated lanes. Unbound it would be UTC's, which is nobody's day
		// outside one timezone.
		attention.WithZone(attentionZone(pool)),
	).WithWaiting(attentionWaiting{
		store: activities.NewStore(db).WithOwnDomains(
			ownDomainReader{store: capture.NewOwnDomainStore(db)}),
		now: now,
	}).
		// The reader's own override. The ranking has carried a pin level since
		// it was written and nothing could set it, so the one control that says
		// "I know, and I want this first anyway" did not exist.
		WithPins(attentionPins{pool: pool, store: activities.NewStore(db)}).
		// One reader's walk, held still while they page it. Without it the
		// cursor is an offset into a ranking rebuilt on every read, so the
		// count above the queue climbs as work arrives behind somebody who is
		// still paging and a row crossing the page boundary is served twice or
		// not at all.
		WithWalks(worklistsnap.New(pool, now)).
		// The asks waiting on this colleague to answer. Until this lane existed
		// a colleague learned they had been asked only by opening that
		// contact's Network tab, so an ask nobody went looking for expired
		// unanswered — which to the requester reads exactly like a refusal.
		WithIntroductions(attentionIntroductions{
			store: introductions.NewStore(db, time.Now),
		}).
		// The reader's own sends that never left, from the stamp the
		// dispatcher's park records on the row.
		WithUndelivered(attentionUndelivered{store: comms.NewStore(db, time.Now, activities.NewStore(db))}).
		WithMachineSender(capture.IsMachineAddress).
		// The figures behind a deal a row names but does not carry — the
		// overnight brief's rows, which rank ids and keep their evidence
		// behind the brief's own endpoint.
		WithDealFacts(attentionDealFacts{store: deals.NewStore(db, DealsInstallation())}).
		// The step a deal row suggests, decided ONCE by the deal's own status
		// card and read here. The queue does not reason about next steps: it
		// reads what that card already worked out, so the row and the deal page
		// name the same move rather than arriving at two answers.
		//
		// Reads the card's cache and never assembles one — a page holds thirty
		// rows and assembling costs a timeline, seats and possibly a model call
		// each. A deal nobody has opened simply carries no step.
		WithDealMoves(newDealStatusService(pool)).
		// The base-currency conversion the ranked queue's money comparisons
		// run in — the same engine every other money surface prices with.
		WithBaseMoney(AttentionBaseMoney{Pool: pool}).
		// Whether a team-scoped reader may open a named person's queue. Bound
		// unconditionally: unbound, that reader is refused, so a seam that
		// dropped this would present as a Team Lead unable to open their own
		// rep's day rather than as one able to open a stranger's.
		WithTeammates(newTeammatesSeam(pool)).
		// The inbound leads still owed a first reply. The store answers the
		// ordering and the state; this lane only ranks them against the rest of
		// the day.
		WithLeadResponses(attentionLeadResponses{
			store:     people.NewStore(db),
			teammates: newTeammatesSeam(pool),
		}).
		// How many promises each teammate has already missed, for the team
		// board. Counted rather than listed, because the task lane above stops
		// at a dozen and a board built from it would call every loaded rep
		// equally loaded.
		WithOverdueLoad(attentionOverdue{store: activities.NewStore(db)})
}

// attentionZone binds the feed's day boundary to the installation's timezone,
// through the SAME read the slipping-deal sweep and the morning brief already
// make — one spelling of "which day is it here", so two surfaces cannot come to
// disagree about when today ended.
//
// A transaction per call and no cache: the setting is one indexed row, the feed
// asks once per request, and a cached zone would go on deciding "today" for
// however long the process lives after an operator moved the installation.
func attentionZone(pool *pgxpool.Pool) attention.Zone {
	return func(ctx context.Context) (*time.Location, error) {
		return installationZone(ctx, pool)
	}
}
