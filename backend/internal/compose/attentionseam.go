// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The day's surface, wired to the modules that own what it shows.
//
// attention is a compose subpackage and approvals, contacts and activities are
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
	"github.com/margince/margince/backend/internal/compose/dealstatus"
	"github.com/margince/margince/backend/internal/compose/magic"
	"github.com/margince/margince/backend/internal/compose/worklistsnap"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/aiactivity"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/introductions"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// attentionApprovals reads the staged queue through the engine every approval
// surface reads, so the feed shows what the inbox shows.
type attentionApprovals struct{ svc *approvals.Service }

func (a attentionApprovals) ListWire(ctx context.Context, in attention.ApprovalQuery) ([]crmcontracts.Approval, error) {
	rows, _, err := a.svc.ListWire(ctx, approvalListInput(in))
	return rows, err
}

func approvalListInput(in attention.ApprovalQuery) approvals.ListInput {
	return approvals.ListInput{Status: &in.Status, Limit: in.Limit, Actionable: true, RequestedForReader: in.RequestedForReader, RequestedForOwners: in.RequestedForOwners, OnlyUnassigned: in.OnlyUnassigned}
}

// CountPending is the engine's exact count, under the scope the lane reads.
func (a attentionApprovals) CountPending(ctx context.Context, in attention.ApprovalQuery) (int, error) {
	count, err := a.svc.CountPending(ctx, approvalListInput(in))
	return count.Count, err
}

// ApprovalsAfter continues the lane's own read past its depth, through the
// inbox keyset. A later page is decided by the same read as the first.
func (a attentionApprovals) ApprovalsAfter(
	ctx context.Context, in attention.ApprovalQuery, after string,
) ([]attention.Positioned[crmcontracts.Approval], bool, error) {
	query := approvalListInput(in)
	query.Cursor = after
	rows, page, err := a.svc.ListWire(ctx, query)
	if err != nil {
		return nil, false, err
	}
	staged := make([]attention.Positioned[crmcontracts.Approval], len(rows))
	for i, row := range rows {
		position, err := approvals.PositionAfter(row)
		if err != nil {
			return nil, false, err
		}
		staged[i] = attention.Positioned[crmcontracts.Approval]{Row: row, After: position}
	}
	return staged, page.HasMore, nil
}

func (a attentionApprovals) PendingInScope(ctx context.Context, in attention.ApprovalQuery, named []ids.ApprovalID) (map[ids.ApprovalID]bool, error) {
	return a.svc.PendingInScope(ctx, approvalListInput(in), named)
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
	entityLead: 1,
	entityDeal: 2,
	crmcontracts.ActivityLinkEntityTypeProject: 3,
	entityContact: 4,
	entityCompany: 5,
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

// attentionFailedEffects reads the decisions this rep approved whose released
// work then failed — the mark decide.go leaves on the approved row. The
// service binds the acting user itself (FailedForDecider), so this lane can
// only ever carry the reader's own decisions back to them.
type attentionFailedEffects struct{ svc *approvals.Service }

func (f attentionFailedEffects) Failed(ctx context.Context, limit int) ([]attention.FailedEffect, error) {
	decisions, err := f.svc.OwnFailedDecisions(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]attention.FailedEffect, 0, len(decisions))
	for _, decision := range decisions {
		row := decision.Approval
		if row.EffectFailedAt == nil || row.EffectFailure == nil {
			// The SQL filter guarantees the pair; the re-check is what makes
			// the derefs safe in this package, where that SQL is elsewhere.
			continue
		}
		failed := attention.FailedEffect{
			ID:        ids.UUID(row.Id),
			Kind:      row.Kind,
			Sentence:  *row.EffectFailure,
			Summary:   row.Summary,
			FailedAt:  *row.EffectFailedAt,
			Retryable: decision.Retryable,
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

// newAttentionHandlers assembles the surface for the API role.
func newAttentionHandlers(pool *pgxpool.Pool, svc *approvals.Service) attention.Handlers {
	return attention.NewHandlers(newAttentionService(pool, svc, func() time.Time { return time.Now().UTC() }))
}

// NewAttentionService is the production attention binding, for a test outside
// this package.
func NewAttentionService(pool *pgxpool.Pool, svc *approvals.Service, now attention.Clock) *attention.Service {
	return newAttentionService(pool, svc, now)
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
	cards := newDealStatusService(pool, now)
	waiting := attentionWaiting{
		store: activities.NewStore(db).WithOwnDomains(
			ownDomainReader{store: capture.NewOwnDomainStore(db)}),
		deals:    deals.NewStore(db, DealsInstallation()),
		contacts: newContactsStore(db),
	}
	atRisk := attentionAtRisk{
		lister: quietDealScanWithClock(pool, deals.QuietThresholdDays, now),
		tail:   quietDealTail(pool, deals.QuietThresholdDays),
		among:  quietDealsAmong(pool, deals.QuietThresholdDays), pool: pool, now: now,
	}
	return attention.NewService(
		attentionApprovals{svc: svc},
		attentionDuplicates{store: newContactsStore(db)},
		attentionTasks{store: activities.NewStore(db), claims: newContactsStore(db), work: newInquiryWork(db), db: db},
		attentionReceipts{tasks: activities.NewStore(db).WithRecordZone(installationZoneTx), svc: svc, deals: deals.NewStore(db, DealsInstallation())},
		attentionBriefing{
			engine:  briefs.NewBriefEngine(pool, newContactsStore(db)),
			figures: attentionDealFacts{store: deals.NewStore(db, DealsInstallation())},
		},
		nil, // Assigned promises are tasks; taskless claims name no personal owner.
		atRisk,
		attentionDecay{pool: pool, store: newContactsStore(db), now: now},
		attentionMeetings{store: activities.NewStore(db)},
		attentionFailedEffects{svc: svc},
		attentionDSRs{store: consent.NewStore(db)},
		attentionCaptureHealth{registry: captureHealthRegistry(db)},
		attentionAIWork{store: aiactivity.NewStore(db)},
		attentionBounces{store: comms.NewStore(db, time.Now, activities.NewStore(db))},
		attentionAutomations{store: automation.NewAutomationStore(db)},
		attentionNotices{store: notices.NewStore(db), users: identity.NewService(db.Pool())},
		newAttentionNames(db),
		now,
		attention.WithZone(attentionZone(pool)),
		attention.WithMeetingsAwaitingOutcome(
			attentionMeetingsAwaitingOutcome{store: activities.NewStore(db)}),
		attention.WithNoticeCases(attentionNoticeCases{store: consent.NewStore(db)}),
		attention.WithContactCases(attentionContactCases{store: newContactsStore(db)}),
	).WithWaiting(waiting).
		WithWaitingBacklog(waiting).
		WithRiskBacklog(atRisk).WithApprovalBacklog(attentionApprovals{svc: svc}).
		WithAwaiting(attentionAwaiting{
			store: activities.NewStore(db).WithOwnDomains(
				ownDomainReader{store: capture.NewOwnDomainStore(db)}),
		}).
		WithPins(attentionPins{pool: pool, store: activities.NewStore(db)}).
		WithDone(attentionPins{pool: pool, store: activities.NewStore(db).WithClock(now)}).
		WithWalks(worklistsnap.New(pool, now)).
		WithTaskBacklog(attentionTasks{store: activities.NewStore(db), work: newInquiryWork(db)}).
		WithHandledWindow(func(ctx context.Context, asOf time.Time) (time.Time, error) {
			return magic.ResolveWindow(ctx, magicBriefCutoff{engine: briefs.NewBriefEngine(pool, nil), now: now}, nil, asOf)
		}).
		WithReceiptDetails(attentionReceiptDetails{magic: newMagicService(pool, svc, now), names: newAttentionNames(db)}).
		WithSnapshots(attentionSnapshots{pool: pool}).
		WithIntroductions(attentionIntroductions{
			store: introductions.NewStore(db, time.Now),
		}).
		WithUndelivered(attentionUndelivered{store: comms.NewStore(db, time.Now, activities.NewStore(db))}).
		WithMachineSender(capture.IsMachineAddress).
		WithMeetingHorizon(attentionMeetingHorizon{
			registry: captureHealthRegistry(db), store: activities.NewStore(db),
		}).
		WithDomainQuestions(attentionDomainQuestions{store: newContactsStore(db)}).
		WithBookingHealth(newAttentionBookingHealth(pool, db)).
		WithDealSuggestions(attentionDealSuggestions{store: deals.NewStore(db, DealsInstallation())}).
		WithTagSuggestions(attentionTagSuggestions{store: collections.NewStore(db)}).
		WithDealFacts(attentionDealFacts{store: deals.NewStore(db, DealsInstallation())}).
		WithContactTouch(attentionContactTouch{pool: pool}).
		WithCompanyTouch(attentionCompanyTouch{pool: pool, now: now}).
		WithContactEmployers(newContactsStore(db)).
		WithMeetingOrganizers(activities.NewStore(db)).
		// Cached cards keep queue reads from starting model work.
		WithDealMoves(attentionDealMoves{cards: cards}).
		WithDealStandings(attentionDealStandings{cards: cards}).
		WithBaseMoney(AttentionBaseMoney{Pool: pool}).
		WithWeeklyPlans(attentionWeeklyPlan{store: weeklyPlanStore(pool), pool: pool}).WithNamedTeams(newTeammatesSeam(pool)).WithTeammates(newTeammatesSeam(pool)).WithFormerTeammates(newTeammatesSeam(pool)).
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

// attentionDealStandings carries the deal card's written verdict to the queue.
//
// An adapter rather than a direct binding, because the two sides name the same
// thing with different types: dealstatus answers its own CachedCard, and the
// queue's seam is declared over a type it owns. A shared type would be a
// sibling-module import in one direction or the other, which is the edge every
// seam in this file exists to avoid.
// attentionDomainQuestions binds the reader's own undecided domains to the
// contacts store that owns the triage ledger.
type attentionDomainQuestions struct{ store *contacts.Store }

// OpenDomainQuestions answers the acting human's own open questions.
//
// The port takes no owner argument on purpose: the store reads the acting
// human's id itself, which is what lets the queue row claim the reader as its
// owner without a second field restating it.
func (a attentionDomainQuestions) OpenDomainQuestions(ctx context.Context) ([]attention.DomainQuestion, error) {
	// The read binds only the owner, so one generic plan serves every reader.
	var rows []contacts.DomainQuestion
	err := database.ReuseGenericPlans(ctx, func(ctx context.Context) error {
		var err error
		rows, err = a.store.OpenDomainQuestionsForOwner(ctx)
		return err
	})
	if err != nil {
		return nil, err
	}
	out := make([]attention.DomainQuestion, 0, len(rows))
	for _, row := range rows {
		out = append(out, attention.DomainQuestion{
			Domain:     row.Domain,
			Reason:     row.Reason,
			ReasonCode: row.ReasonCode,
			AskedAt:    row.AskedAt,
			EvidenceAt: row.EvidenceAt,
			Note:       row.Note,
			Context:    row.Context,
		})
	}
	return out, nil
}

type attentionDealStandings struct {
	cards *dealstatus.Service
}

// CachedStandings answers the already-written standing for each of these deals.
func (a attentionDealStandings) CachedStandings(
	ctx context.Context, dealIDs []ids.UUID,
) (map[ids.UUID]attention.DealStanding, error) {
	cards, err := a.cards.CachedCards(ctx, dealIDs)
	// A standing is an ENRICHMENT of a queue row, not the row itself, so a
	// caller who may not read deals loses the standing and keeps the page —
	// the same answer attentionNames.Labels gives for a type it may not read.
	//
	// The two grants come apart: a role holding activity but not deal is a
	// grant an administrator can write, and the activity lane puts a task
	// linked to a deal on that member's queue. Propagating the refusal would
	// turn their perfectly legitimate worklist into a 403.
	//
	// Any other error still propagates: a database that will not answer must
	// not read as a queue whose rows simply have no standing.
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return map[ids.UUID]attention.DealStanding{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := make(map[ids.UUID]attention.DealStanding, len(cards))
	for id, card := range cards {
		takenAt := card.GeneratedAt
		out[id] = attention.DealStanding{
			Standing:     card.Standing,
			DecisiveLine: card.DecisiveLine,
			AsOf:         &takenAt,
		}
	}
	return out, nil
}

func (a attentionDomainQuestions) OwnsDomainQuestion(ctx context.Context, domain string) (bool, error) {
	return a.store.OwnsDomainQuestion(ctx, domain)
}
