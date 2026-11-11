// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/accountdraft"
	"github.com/margince/margince/backend/internal/compose/company360"
	"github.com/margince/margince/backend/internal/compose/companybrief"
	"github.com/margince/margince/backend/internal/compose/companydossier"
	"github.com/margince/margince/backend/internal/compose/companyscan"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/customfields"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// wireSystemOfRecordReads builds the reads over this system of record: the
// company view and its grounded prose.
func (s *Server) wireSystemOfRecordReads(pool *pgxpool.Pool) {
	// The contacts store carries the fieldcatalog seam contactsHandlers gets.
	// Without it the company view would omit the cf_* columns that
	// GET /companies/{id} returns for the same record.
	// The brief reads through the 360 service, so it sees only what the page shows.
	// Its nil model lane is bound by WithCompanyBrief; unbound, it serves its floor.
	s.contactsStore = newContactsStore(InstallationDB(pool)).WithFieldCatalog(customfields.NewService(pool, nil))
	s.blockedDomainHandlers = blockedDomainHandlers{contacts: s.contactsStore}
	s.captureExclusionHandlers = captureExclusionHandlers{store: capture.NewExclusionStore(InstallationDB(pool))}
	// Both stores, because answering a domain question is two different acts:
	// keeping it settles the triage ledger, discarding it writes the caller's
	// own capture exclusion.
	s.domainQuestionHandlers = domainQuestionHandlers{
		contacts:   s.contactsStore,
		exclusions: capture.NewExclusionStore(InstallationDB(pool)),
	}
	s.threadAudience = NewThreadAudienceSetter(pool)
	s.captureSenderHandlers = captureSenderHandlers{
		db:         InstallationDB(pool),
		store:      newSenderOverrideStore(pool),
		threadPass: verdictPass(pool, ConfidentialityVerdictArgs{}.Kind()),
	}
	s.captureOwnerIdentityHandlers = captureOwnerIdentityHandlers{store: capture.NewOwnerIdentityStore(InstallationDB(pool))}
	suggestionSink := sync.OnceValue(func() *capture.Sink { return newCaptureSink(pool, s.captureConfig) })
	s.conversationSuggestionHandlers = conversationSuggestionHandlers{read: func(ctx context.Context, anchor datasource.EntityRef) (capture.ConversationSuggestions, error) {
		return suggestionSink().ConversationSuggestions(ctx, anchor)
	}}
	s.captureCounterpartyHoldHandlers = captureCounterpartyHoldHandlers{
		store:     capture.NewCounterpartyHoldStore(InstallationDB(pool)),
		recompute: activities.RecomputeAudienceTx,
		clearHold: activities.ClearCounterpartyHoldTx,
	}
	s.marginceChangeHandlers = newMarginceChangeHandlers(pool)
	s.claimHandlers = claimHandlers{contacts: s.contactsStore, deals: deals.NewStore(InstallationDB(pool), DealsInstallation())}
	s.ownershipRequestHandlers = ownershipRequestHandlers{transfers: newOwnershipTransfers(pool)}
	s.dealCommitmentHandlers = newDealCommitmentHandlers(pool, time.Now)
	s.promiseTakeOverHandlers = newPromiseTakeOverHandlers(pool)
	// The importer maps only core columns (see importTargets for why custom
	// fields are not among them), so it needs no field catalog of its own.
	s.importHandlers = importHandlers{db: InstallationDB(pool), uploadLimit: s.uploadLimits.CSVImport}
	s.company360Svc = company360.NewService(pool, s.contactsStore, s.dealsStore, ProjectsStore(pool), NewApprovalsService(InstallationDB(pool)), time.Now).
		WithActivityFields(activities.NewStore(InstallationDB(pool)).WithFieldCatalog(customfields.NewService(pool, nil))).
		WithSignalWords(signalSummaryWords)
	s.companyBriefSvc = companybrief.NewService(pool, s.company360Svc, s.contactsStore, nil, "", time.Now).
		WithEmailSummaries(emailRows(pool))
	s.companyBriefHandlers = companybrief.NewHandlers(s.companyBriefSvc)
	// The dossier reads the contacts store the 360 and the brief read, so the
	// three agree on a company's facts. WithCompanyDossier and WithGrowthFit
	// bind the api role's lanes; unbound, each serves its deterministic floor.
	// The floors differ in kind (the dossier's describes, growth fit's abstains),
	// so they are separate options.
	s.companyDossierSvc = companydossier.NewService(pool, s.contactsStore, nil, "", time.Now).
		WithEmailSummaries(emailRows(pool))
	s.companyGrowthFitSvc = companydossier.NewGrowthFitService(
		pool, s.contactsStore, offeringConfirmed(s.contactsStore), nil, "", time.Now,
	).WithEmailSummaries(emailRows(pool))
	s.companyDossierHandlers = companydossier.NewHandlers(
		s.companyDossierSvc, s.companyGrowthFitSvc,
	)
	// The account scan over the same composite read and the same dismissals.
	// No lane and no job runner here: an ensure on this role settles the
	// rules' floor in-request, and WithAccountScan binds the api role's.
	s.companyScanSvc = companyscan.NewService(pool, s.company360Svc, s.company360Svc, nil, nil, nil, time.Now, s.log).
		WithEmailSummaries(emailRows(pool))
	s.companyScanHandlers = companyscan.NewHandlers(s.companyScanSvc)
	s.company360Svc.RecogniseScanFindings(s.companyScanSvc)
	// Built after the dossier service, which the drafter takes as a dependency.
	// A nil *Service passed as the interface is not a nil interface.
	// It would pass the drafter's guard and panic on the first account draft.
	// The draft reads through the same 360 and writes nothing, so it needs no pool.
	// WithAccountDraft binds the api role's lane; unbound, it answers from its floor.
	s.firstDrafts.account = accountdraft.NewService(s.company360Svc, nil).
		WithEnvelope(draftEnvelope(pool, s.log)).
		WithEmailSummaries(emailRows(pool)).
		WithDossier(s.companyDossierSvc)
	s.accountDraftHandlers = accountdraft.NewHandlers(s.firstDrafts.account)
	s.company360Handlers = company360.NewHandlers(s.company360Svc)
	// The contact page is the company page's sibling, so it is wired here
	// rather than beside the handler sets.
	s.wireContact360(pool)
	s.wireReversal(pool)
	s.wireProject360(pool)
	s.wireSalesReview(pool)
}
