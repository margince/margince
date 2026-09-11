// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/accountdraft"
	"github.com/margince/margince/backend/internal/compose/company360"
	"github.com/margince/margince/backend/internal/compose/companybrief"
	"github.com/margince/margince/backend/internal/compose/companydossier"
	"github.com/margince/margince/backend/internal/compose/companyscan"
	"github.com/margince/margince/backend/internal/compose/contact360"
	"github.com/margince/margince/backend/internal/compose/contactbrief"
	"github.com/margince/margince/backend/internal/compose/contactdraft"
	"github.com/margince/margince/backend/internal/compose/contactresearch"
	"github.com/margince/margince/backend/internal/compose/dealstatus"
	"github.com/margince/margince/backend/internal/compose/leaddraft"
	"github.com/margince/margince/backend/internal/compose/meetingbrief"
	"github.com/margince/margince/backend/internal/compose/pipelinetrace"
	"github.com/margince/margince/backend/internal/compose/project360"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/aiactivity"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/commissions"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/contracts"
	"github.com/margince/margince/backend/internal/modules/customfields"
	"github.com/margince/margince/backend/internal/modules/dealrooms"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/finance"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/introductions"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/modules/overlay"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/modules/webhooks"
	"github.com/margince/margince/backend/internal/modules/weeklyplan"
	"github.com/margince/margince/backend/internal/shared/ports/contactdata"
)

// Aliases give the embedded handler sets distinct field names; each
// alias carries its module's full method set.
type (
	authHandlers            = identity.Handlers
	channelHandlers         = capture.ChannelHandlers
	traceHandlers           = capture.TraceHandlers
	pipelineTraceHandlers   = pipelinetrace.Handlers
	contactsHandlers        = contacts.Handlers
	dealsHandlers           = deals.Handlers
	projectsHandlers        = projects.Handlers
	contractsHandlers       = contracts.Handlers
	dealroomsHandlers       = dealrooms.Handlers
	commissionsHandlers     = commissions.Handlers
	activitiesHandlers      = activities.Handlers
	approvalsHandlers       = approvals.Handlers
	searchHandlers          = search.Handlers
	consentHandlers         = consent.Handlers
	collectionsHandlers     = collections.Handlers
	signalsHandlers         = signals.Handlers
	privacyHandlers         = privacy.Handlers
	automationHandlers      = automation.Handlers
	voiceHandlers           = ai.Handlers
	customfieldsHandlers    = customfields.Handlers
	overlayHandlers         = overlay.Handlers
	webhooksHandlers        = webhooks.Handlers
	company360Handlers      = company360.Handlers
	contact360Handlers      = contact360.Handlers
	project360Handlers      = project360.Handlers
	contactBriefHandlers    = contactbrief.Handlers
	contactResearchHandlers = contactresearch.Handlers
	meetingBriefHandlers    = meetingbrief.Handlers
	dealStatusHandlers      = dealstatus.Handlers
	companyBriefHandlers    = companybrief.Handlers
	companyDossierHandlers  = companydossier.Handlers
	companyScanHandlers     = companyscan.Handlers
	accountDraftHandlers    = accountdraft.Handlers
	contactDraftHandlers    = contactdraft.Handlers
	leadDraftHandlers       = leaddraft.Handlers
	financeHandlers         = finance.Handlers
	aiActivityHandlers      = aiactivity.Handlers
	noticesHandlers         = notices.Handlers
	weeklyPlanHandlers      = weeklyplan.Handlers
	forecastHandlers        = forecasting.Handlers
	assuranceHandlers       = assurance.Handlers
	introductionHandlers    = introductions.Handlers
)

// wireContact360 binds the contact record page — the company page's
// sibling: same one-transaction assembly, same omitted-and-named sections,
// same overlay refusal. Its own function so the composition root reads as a
// list of what is wired rather than how each piece is built.
func (s *Server) wireContact360(pool *pgxpool.Pool) {
	s.contact360Svc = contact360.NewService(pool, s.contactsStore, s.dealsStore, ProjectsStore(pool), consent.NewStore(InstallationDB(pool)), comms.NewStore(InstallationDB(pool), time.Now, activities.NewStore(InstallationDB(pool))), ai.NewFeedbackStore(InstallationDB(pool)), time.Now)
	s.contact360Handlers = contact360.NewHandlers(s.contact360Svc, s.sorDispatch.isOverlay)
	// The relationship brief is assembled from the SAME composite read the page
	// serves, so the two cannot disagree about what this caller may see. The
	// model lane is nil here: WithContactBrief binds the api role's, and a role
	// that wires none serves the deterministic floor and says so in
	// generated_by, rather than 501-ing on a workspace without a model.
	s.contactBriefHandlers = contactbrief.NewHandlers(
		contactbrief.NewService(pool, s.contact360Svc, nil, "", time.Now).
			WithEmailSummaries(emailRows(pool)),
		s.sorDispatch.isOverlay,
	)
	// The pre-meeting brief shares that composite read and adds the claim
	// reader for the rest of the room. It caches NOTHING (ADR-0097 D5): it is
	// opened minutes before a meeting, so a stored artifact would be the one
	// thing it must not be. Same deterministic floor and the same
	// generated_by honesty as the brief above.
	// The same membership seam the Worklist reads. Coaching asks "may this lead
	// speak into that rep's day"; the Worklist asks "may this lead open it".
	// One edge, one answer — two derivations would drift, and the pair that
	// drifts is exactly those two.
	s.meetingBriefSvc = meetingbrief.NewService(pool, s.contact360Svc, s.contactsStore, time.Now).
		WithEmailSummaries(emailRows(pool)).
		WithTeammates(newTeammatesSeam(pool))
	s.meetingBriefHandlers = meetingbrief.NewHandlers(s.meetingBriefSvc, s.sorDispatch.isOverlay)
	// The deal's status card reads the deal, its health, timeline, tasks and
	// Deal Room through their own gates. Built through the shared constructor
	// because the worklist reads the move this same service decides — see
	// newDealStatusService for why there is only one.
	s.dealStatusSvc = newDealStatusService(pool)
	s.dealStatusHandlers = dealstatus.NewHandlers(s.dealStatusSvc)
	// No provider is registered, which is the supported configuration rather
	// than a gap: the surface answers "not connected" and writes nothing
	// (ADR-0096 D4). Connecting one later is a provider implementation.
	s.contactResearchHandlers = contactresearch.NewHandlers(
		contactresearch.NewService(s.contactsStore, s.contact360Svc, contactdata.NewRegistry(nil), time.Now),
		s.sorDispatch.isOverlay,
	)
	// The contact-side draft reads through the same 360 and writes nothing, so
	// it needs no pool of its own. Nil lane here for the same reason as the
	// brief's: WithContactDraft binds the api role's, and without it the endpoint
	// answers from its deterministic floor rather than 501-ing.
	s.contactDraftHandlers = contactdraft.NewHandlers(
		contactdraft.NewService(s.contact360Svc, nil).
			WithEmailSummaries(emailRows(pool)).
			WithEnvelope(draftEnvelope(pool, s.log)), s.sorDispatch.isOverlay)
	// The lead-side draft, on the same terms: nil lane, so a deployment with no
	// model answers from contactdraft's deterministic floor rather than 501-ing,
	// and WithLeadDraft binds the api role's lane.
	s.leadDraftHandlers = leaddraft.NewHandlers(
		leaddraft.NewService(s.contactsStore, leadCorrespondence{activities.NewStore(InstallationDB(pool))}, nil).
			WithEnvelope(draftEnvelope(pool, s.log)), s.sorDispatch.isOverlay)
}

// wireProject360 assembles the project page from the module stores the
// handler sets already serve — the deals store with its field catalog, the
// shared contacts store, the contracts and activities stores — so the page and
// the per-record endpoints read the same columns under the same gates. It
// rides the same dispatch the company and contact pages do: a workspace on
// the incumbent mirror refuses all three the same way.
func (s *Server) wireProject360(pool *pgxpool.Pool) {
	svc := project360.NewService(
		pool,
		deals.NewStore(InstallationDB(pool), DealsInstallation()).WithFieldCatalog(customfields.NewService(pool, nil)),
		ProjectsStore(pool),
		s.contactsStore,
		contracts.NewStore(InstallationDB(pool), ContractFreezeRate(pool)),
		activities.NewStore(InstallationDB(pool)),
		time.Now,
	)
	s.project360Handlers = project360.NewHandlers(svc, s.sorDispatch.isOverlay)
}
