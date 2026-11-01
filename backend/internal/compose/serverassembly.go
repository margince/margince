// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The assembly steps newServer runs. Each one binds ONE surface group
// together with the cross-module edges it needs — a module never imports a
// sibling (ADR-0054), so compose is where those edges are made. They live
// beside the Server inventory rather than inside it so the literal in
// server.go reads as what a process serves, not as how each set is built.
// serveroptions.go is the other half of the wiring: what a process ROLE
// layers on top of these defaults.

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/accountdraft"
	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/compose/company360"
	"github.com/margince/margince/backend/internal/compose/companybrief"
	"github.com/margince/margince/backend/internal/compose/companydossier"
	"github.com/margince/margince/backend/internal/compose/companyscan"
	"github.com/margince/margince/backend/internal/compose/homecache"
	"github.com/margince/margince/backend/internal/compose/magic"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/customfields"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/integrations"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// newContactsHandlers builds the contact/company/lead transport with the
// seams compose owns for it.
//
// The fieldcatalog seam: customfields' catalog read makes the
// workspace's active cf_* columns ride contact/company
// payloads (values only — the schema-change engine stays behind
// WithSchemaPool; ActiveColumns needs none of it).
// The match stager is injected here because approvals is a sibling of
// contacts and a module never imports one: compose is where that edge is
// made, as it is for every other cross-module dependency.
//
// The lead settings write through the installation settings store, and the
// qualify dialog's "also open a deal" rides the deals store through the
// contacts→deals edge (leadDealOpener) — both injected here for the same
// ADR-0054 reason as the stager.
func newContactsHandlers(pool *pgxpool.Pool) contactsHandlers {
	return withContactMergeCarriers(InstallationDB(pool), contacts.NewHandlers(InstallationDB(pool))).
		WithFieldCatalog(customfields.NewService(pool, nil)).
		WithMatchStager(linkedInMatchStager(pool)).
		WithVCardReviewStager(vcardCreateStager(pool)).
		WithSettings(StoredConfig(pool)).
		WithLocks(loadedLocks).
		WithSeatReadsLeads(seatReadsLeads(pool)).
		WithDealOpener(leadDealOpener{deals: deals.NewStore(InstallationDB(pool), DealsInstallation())}).
		WithShareRequests(contactShareRequests(pool)).
		WithLeadArrivals(newLeadWorkTasks(InstallationDB(pool), slog.Default())).
		WithInstallationRenamer(installationRenamer(pool)).
		WithConsumerMail(capture.MatcherTx)
}

// newActivitiesHandlers builds the timeline transport over the sibling
// modules its inbound and outbound edges need.
func newActivitiesHandlers(pool *pgxpool.Pool) activitiesHandlers {
	// ONE gate serving both seams. The composer's preview and the send's
	// default-deny door are two questions of the same authority, and two gates
	// here would be two answers about one message — the drift this whole
	// subsystem is shaped to prevent.
	gate := consentGateFor(pool)
	return activities.NewHandlers(InstallationDB(pool)).
		WithTaskRecommendation(taskRecommendationReader(pool)).
		WithLeadQualifyTasks(newLeadWorkTasks(InstallationDB(pool), slog.Default())).
		WithFieldCatalog(customfields.NewService(pool, nil)).
		WithRecordZone(installationZoneTx).
		WithSettings(StoredConfig(pool)).
		WithLocks(loadedLocks).
		WithConsent(gate).
		WithSendPreview(gate).
		// The SAME seam the check_availability tool reads, so the two doors
		// answer one question one way: whether a window was read off the host's
		// own diary or derived from this CRM's records.
		WithCalendarConnected(activities.CalendarConnected(calendarBackingResolver(pool))).
		// The public booking capture seams (feedback/14): contacts is the
		// idempotent-on-email contact path, consent records the
		// passthrough — both injected here, never sibling imports.
		WithPublicBooking(newContactsStore(InstallationDB(pool)), bookingConsentAdapter{
			store: consent.NewStore(InstallationDB(pool)),
		}).
		// The RFC 8058 unsubscribe linker (B-E11.32): consent mints the
		// preference token behind the List-Unsubscribe URL.
		WithUnsubscribe(preferenceLinkAdapter{store: consent.NewStore(InstallationDB(pool))}).
		// The disclosures a jurisdiction demands, put into the body that owes
		// them. See compose/disclosurefooter.go.
		WithDisclosures(disclosureAdapter{store: consent.NewStore(InstallationDB(pool)).
			// WITH THE COUNTRY READER, which decides which pack applies. A bare
			// store answers no country, so applicableRules finds no pack and
			// every message silently owes nothing — the failure this seam
			// exists to end, arriving through the wiring instead.
			WithInstallationCountry(consent.InstallationCountryFunc(identity.CountryOf))}).
		// The sender's own sign-off (core 0235). contacts owns the row because
		// it owns the contact the seat belongs to; activities appends it because
		// it owns the one send. The edge is injected here rather than imported,
		// like every other cross-module edge on this path.
		WithSignature(signatureReader{store: newContactsStore(InstallationDB(pool))}).
		// The name on the envelope. identity owns who the acting human is —
		// including the human an agent acts on behalf of — and that resolution
		// must be the same one the audit log records, so it is injected rather
		// than re-derived here.
		WithSenderName(identity.NewServiceFor(InstallationDB(pool))).
		// Which domains are our own, for the waiting queue's colleague rule.
		// capture owns workspace_email_domain and the rule for which entries
		// count as vouched-for, so the edge is injected rather than restated.
		WithOwnDomains(ownDomainReader{store: capture.NewOwnDomainStore(InstallationDB(pool))}).WithConsumerMail(capture.MatcherTx).
		// When a host is bookable. identity owns the setting because it is a
		// fact about a contact; this transport asks for it rather than holding
		// a pair of numbers for everybody (docs/explanation/scheduling.md).
		WithWorkingHours(workingHoursResolver(pool))
}

// Capture identities share the waiting scan's transaction snapshot.
type ownDomainReader struct{ store *capture.OwnDomainStore }

func (o ownDomainReader) Domains(ctx context.Context, tx pgx.Tx) ([]string, error) {
	return o.store.ColleagueDomainsTx(ctx, tx)
}

func (o ownDomainReader) MeetingParties(ctx context.Context, tx pgx.Tx) (owndomain.Parties, error) {
	return o.store.MeetingPartiesTx(ctx, tx)
}

func (o ownDomainReader) ReaderAddresses(
	ctx context.Context, tx pgx.Tx, reader ids.UUID,
) ([]string, error) {
	return o.store.ReaderAddressesTx(ctx, tx, reader)
}

// NewCollectionsStore is the ONE spelling of "the collections store with
// its catalogue": every site that needs a lists/tags/saved-views/export
// store resolving cf_* columns through this workspace's custom-field
// vocabulary calls this, never collections.NewStore directly — so a
// wiring gate on this one constructor covers every caller, rather than
// needing one gate per independently-built store.
func NewCollectionsStore(pool *pgxpool.Pool) *collections.Store {
	return collections.NewStore(InstallationDB(pool)).WithFieldCatalog(customfields.NewService(pool, nil)).
		WithLiveSteward(identity.LiveMemberSQL("u")).WithDealAmount(dealWorthTodaySQL).WithBaseCurrency(identity.BaseCurrencyOf).
		WithRuleUses(ruleUsesOf(pool))
}

// dealWorthTodaySQL is what the filter builder's deal amount compares: the
// forecast's base value, priced at today's rate for an open deal.
var dealWorthTodaySQL = deals.BaseValueSQL("CURRENT_DATE", installationBaseCurrencySQL, "t")

// installationBaseCurrencySQL reads the reporting currency inside the filter's
// own statement, which the segment engine compiles without a Go-side bind.
const installationBaseCurrencySQL = "(SELECT (bc.value #>> '{}')::text FROM setting bc WHERE bc.key = 'installation.base_currency')"

// newCollectionsHandlers builds the lists/tags/saved-views transport over
// NewCollectionsStore, so dynamic-list create validation and the members
// endpoint resolve a definition's vocabulary through collections.Store.
// SegmentEngine exactly as export does (wireExportSurface builds its store
// the same way) — a cf_* filter a saved list or a membership check names
// cannot be refused here while an export of the same list accepts it.
func newCollectionsHandlers(pool *pgxpool.Pool) collectionsHandlers {
	return collections.NewHandlers(NewCollectionsStore(pool))
}

// wireSurfaces binds the handler sets built after the literal, each over a
// dependency the literal had to build first.
func (s *Server) wireSurfaces(pool *pgxpool.Pool, log *slog.Logger) {
	s.dataTaskHandlers = dataTaskHandlers{pool: pool}
	s.wireStagedSurfaces(pool)
	s.wireAnalyticsSurface(pool)
	s.wireCaptureSettingsSurface(pool, log)
	s.wireExportSurface(pool, log)
	s.wireOnboardingSurface(pool)
	s.wireSystemOfRecordReads(pool)
	s.wireBulkSurface(pool)
	s.wireHandoverSurface()
}

// wireStagedSurfaces binds the two surfaces that read the staged queue. They
// share ONE approvals engine, so the day's card and the receipt's line are one
// queue rather than two readings of it; a second engine here is how the two
// would come to disagree.
func (s *Server) wireStagedSurfaces(pool *pgxpool.Pool) {
	staged := approvalsServiceWithEffects(pool)
	s.homeCache = homecache.New(pool, magic.MachineActors())
	s.attentionHandlers = newAttentionHandlers(pool, staged).WithHomeCache(s.homeCache)
	s.magicService = newMagicService(pool, staged, time.Now)
	s.magicHandlers = magic.NewHandlers(s.magicService).WithHomeCache(s.homeCache)
}

// wireCaptureSettingsSurface binds the workspace's own capture posture
// controls.
// wireAnalyticsSurface wires the four handler sets that read the numbers:
// the forecast and the three analytics surfaces built on the same store.
//
// After the literal like the rest of the assembly, and together because they
// are one seam — the share routes run in the FORECAST store's transaction,
// whose InTx gates on forecast:read, so the whole surface (issuing included)
// sits behind the grant that reads the thing being shared. Splitting them
// across the literal and here would put half of that seam out of sight of the
// other half.
// newForecastHandlers is the forecast surface over its seams, every call
// recorded through it keeping its baseline.
func newForecastHandlers(pool *pgxpool.Pool, now func() time.Time) forecasting.Handlers {
	return forecasting.NewHandlers(
		newForecastStoreFor(pool),
		ForecastAssemble, ForecastPeriodAt, ForecastWritableScope,
		ForecastConversionHistory, ForecastForwardMeasure, ForecastScopeHoldsDeals, now,
	).WithCallBaseline(ForecastCallBaseline).WithStageChances(ForecastStageChances).
		WithReaderRows(ForecastReaderContributions)
}

func (s *Server) wireAnalyticsSurface(pool *pgxpool.Pool) {
	reportingService := newReportingService(pool, time.Now)
	s.reportingHandlers = reporting.NewHandlers(reportingService, openingSelection())
	s.reportingExportHandlers = reportingExportHandlers{service: reportingService}
	s.reportingReaderPreviewHandlers = reportingReaderPreviewHandlers{
		reports: reportingService, roles: identity.NewServiceFor(InstallationDB(pool)),
	}
	s.forecastHandlers = newForecastHandlers(pool, func() time.Time { return time.Now().UTC() })
	// The floor comes from the constant rather than a setting for now: one
	// number, and moving it to installation settings is a migration plus a
	// reader, which is its own change.
	s.analyticsQueryHandlers = newAnalyticsQueryHandlers(
		InstallationDB(pool), analyticsquery.DefaultFloor, newAttentionNames(InstallationDB(pool)))
	s.reportMetrics = reportingService
	s.analyticsContextHandlers = newAnalyticsContextHandlers(
		InstallationDB(pool), func() time.Time { return time.Now().UTC() })
	s.sdrSummaryHandlers = newSdrSummaryHandlers(
		InstallationDB(pool), func() time.Time { return time.Now().UTC() }).withTargets(reportingService)
	s.analyticsShareHandlers = newAnalyticsShareHandlers(
		NewAnalyticsShareStore(func() time.Time { return time.Now().UTC() }),
		newForecastStoreFor(pool),
		func() time.Time { return time.Now().UTC() },
	)
}

func (s *Server) wireCaptureSettingsSurface(pool *pgxpool.Pool, log *slog.Logger) {
	// The workspace capture-settings surface (CAP-WIRE-7, ADR-0072):
	// read the auto-enrich posture (all roles), toggle it (admin/ops).
	s.captureSettingsHandlers = captureSettingsHandlers{store: capture.NewSettings(NewSettingsStore(pool), StoredConfig(pool))}
	// Whether contacts are looked up automatically for the details the provider
	// charges nothing for. Always wired, including where no provider is
	// connected: the posture is the installation's answer, not the
	// connection's, and an operator must be able to set it before connecting.
	s.integrationsSettingsHandlers = integrationsSettingsHandlers{store: integrations.NewSettings(NewSettingsStore(pool), StoredConfig(pool))}
	// The tier→model binding (ai-operational-spec §1.4): read it, replace it
	// without a restart. Always wired, including on an installation that has
	// bound nothing — an operator binding models for the first time reaches it
	// through the same surface as one re-pointing a lane.
	//
	// WithCatalogue wires the public OpenRouter model read unconditionally: it
	// needs no tenant credential, so there is no "no provider connected"
	// configuration to honor here.
	s.priceCatalogues = newAIPriceCatalogues()
	routing := ai.NewRoutingStore(NewSettingsStore(pool), StoredConfig(pool)).WithCatalogue(s.priceCatalogues.broker)
	s.aiRoutingHandlers = aiRoutingHandlers{store: routing}
	// The sync reads the picker's broker cache. s.vault is nil until WithKeyvault,
	// which rebuilds the sync over the vault it brings.
	s.voiceHandlers = s.WithPriceSync(newAIPriceSync(pool, s.vault, log, s.priceCatalogues))
	s.aiAdminHandlers = aiAdminHandlers{store: ai.NewAdminStore(InstallationDB(pool), NewSettingsStore(pool), StoredConfig(pool), budgetFullUsers, aiDeferredWork(pool))}
	s.ownDomainHandlers = ownDomainHandlers{
		store:    capture.NewOwnDomainStore(InstallationDB(pool)),
		evidence: deals.NewStore(InstallationDB(pool), DealsInstallation()), log: log,
	}
	// The installation's own identity and reporting basis (ADR-0090/A135):
	// name, reporting zone, base currency — the last of which locks once a
	// deal has converted against it (ADR-0085 §7).
	s.installationSettingsHandlers = installationSettingsHandlers{
		store:          NewInstallationSettingsFor(pool),
		pool:           pool,
		maxUploadBytes: s.uploadLimits.Attachment,
	}
	// The workspace's own consumer-mail list (CAP-PARAM-5): the surviving
	// domain control, and the only way an operator corrects a shipped
	// baseline that is wrong about one of their customers.
	s.consumerMailDomainHandlers = consumerMailDomainHandlers{store: capture.NewFreemailDomains(InstallationDB(pool))}
	// The seat COUNT, wired here rather than with the entitlement, because how
	// many seats an installation is USING is a fact about app_user rows and holds
	// whether or not a license was ever configured. A deployment that wires no
	// posture still answers the capacity surface.
	//
	// The entitlement is the half that needs the option: WithLicensePosture adds
	// the posture to this handler through withPosture rather than rebuilding it,
	// so the store is spelled once and neither half can drop the other.
	s.licenseHandlers = licenseHandlers{seats: identity.NewSeatUsage(InstallationDB(pool))}
}

// wireExportSurface binds the two export transports.
func (s *Server) wireExportSurface(pool *pgxpool.Pool, log *slog.Logger) {
	// First-class filtered export (B-E15.13): the writer reuses the ONE
	// predicate engine + the bundle writer's open-format rendering; the
	// collections store resolves a saved view / dynamic list source behind
	// its own visibility gate. WithFieldCatalog widens that same store's
	// vocabulary with this workspace's cf_* columns, so an export cannot
	// disagree with the list or the saved view it was built from — the same
	// seam newContactsHandlers wires for the record stores.
	// One store for both surfaces: the preview rides the same engine and the same
	// projection as the export, so a filter's count and sample cannot disagree
	// with an export of that filter.
	collectionsStore := NewCollectionsStore(pool)
	s.filteredExportHandlers = filteredExportHandlers{
		writer:      NewFilteredExportWriter(pool),
		collections: collectionsStore,
	}
	s.filterPreviewHandlers = filterPreviewHandlers{
		pool:        pool,
		collections: collectionsStore,
	}
	// The lane stays nil until WithFilterProposals binds one, which is the
	// no-model answer: a 409 saying a model is needed, never a 500.
	s.filterProposalHandlers = filterProposalHandlers{
		pool: pool, collections: collectionsStore, now: time.Now,
		labels: catalogLabels(customfields.NewService(pool, nil)),
	}
	s.exportBundleHandlers = newExportBundleHandlers(pool, log)
}

// wireOnboardingSurface binds the first-run group: the installation's own
// company, the site read that seeds it, and the onboarding state the two
// report progress through — all three gated by the same rollout.
func (s *Server) wireOnboardingSurface(pool *pgxpool.Pool) {
	// The installation's own company (the 0083 anchor). Its own store
	// instance, like every other contacts-backed shadow here: the company
	// form's write shape is contacts's, the transport is compose's.
	s.companyHandlers = companyHandlers{
		store: newContactsStore(InstallationDB(pool)).WithInstallationRenamer(installationRenamer(pool)).
			WithGeocodeEnqueue(boundGeocodeEnqueue),
		rollout: companyContextRolloutOnboarding,
	}
	s.siteReadHandlers = siteReadHandlers{companyContextRollout: companyContextRolloutOnboarding}
	s.onboardingStateHandlers = onboardingStateHandlers{
		state: identity.NewOnboardingStore(InstallationDB(pool)), company: newContactsStore(InstallationDB(pool)),
		proposal: &onboardingProposalEngine{
			state: identity.NewOnboardingStore(InstallationDB(pool)), contacts: newContactsStore(InstallationDB(pool)),
			rollout: companyContextRolloutOnboarding,
		},
	}
}

// wireSystemOfRecordReads builds the reads over this system of record — the
// company view and its grounded prose.
func (s *Server) wireSystemOfRecordReads(pool *pgxpool.Pool) {
	// The contacts store carries the SAME fieldcatalog seam contactsHandlers
	// gets: the 360 serves the company object, and without it the
	// company view would silently omit the cf_* columns GET
	// /companies/{id} returns for the same record.
	// The brief reads THROUGH the 360 service, so it inherits every gate the
	// page itself applies and can only describe what this caller may see.
	// The model lane is nil here: WithCompanyBrief binds the api role's
	// summarize lane, and without it the brief serves its deterministic
	// floor.
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
		store:      capture.NewSenderOverrideStore(InstallationDB(pool)),
		threadPass: verdictPass(pool, ConfidentialityVerdictArgs{}.Kind()),
	}
	s.captureOwnerIdentityHandlers = captureOwnerIdentityHandlers{store: capture.NewOwnerIdentityStore(InstallationDB(pool))}
	s.captureCounterpartyHoldHandlers = captureCounterpartyHoldHandlers{
		store:     capture.NewCounterpartyHoldStore(InstallationDB(pool)),
		recompute: activities.RecomputeAudienceTx,
		clearHold: activities.ClearCounterpartyHoldTx,
	}
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
	// The dossier reads the SAME contacts store the 360 and the brief read, so
	// the three cannot drift about what a company's facts are. No model lane is
	// wired yet: every assembly is the deterministic floor and says so.
	// Both lanes are nil here: WithCompanyDossier and WithGrowthFit bind the
	// api role's, and without them each surface serves its deterministic floor.
	// The two floors differ in kind — the dossier's still describes the company,
	// where growth fit's can only abstain — which is why they are separate
	// options rather than one.
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
	// AFTER the dossier service exists: the drafter takes it as a dependency,
	// and a nil *Service handed through the interface is not the nil INTERFACE
	// the drafter guards against — it would pass the guard and panic on the
	// first account draft.
	//
	// The account-started draft reads through the same 360 and writes nothing,
	// so it needs no pool of its own. Nil lane here for the same reason as the
	// brief's: WithAccountDraft binds the api role's, and without it the
	// endpoint answers from its deterministic floor rather than 501-ing.
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
}

// newConsentHandlers binds the consent surface to the four edges it cannot
// reach for itself.
//
// DSR fulfillment executes privacy's erase path, and the subject-access export
// assembles rows from every module that holds them — injected here so consent
// never imports a sibling.
//
// The guard endpoint previews the same verdict the send path takes, so it
// resolves the same jurisdiction windows. Without the country seam it would
// answer on the core defaults while a pack shortened the real ones, and tell a
// rep a send is allowed that the engine then refuses.
//
// The language seam is beside it because the two are the same shape: a setting
// identity owns, read on the caller's transaction, that consent cannot import
// its way to. Unwired, the confirm mail is written in English — which is what it
// was before the catalog existed, and is why this is not part of the
// confirmation lane's all-three-or-none rule.
func newConsentHandlers(pool *pgxpool.Pool) consent.Handlers {
	return consent.NewHandlers(InstallationDB(pool)).
		WithEraser(privacy.NewEraser(InstallationDB(pool)).WithCountry(identity.CountryForDecision)).
		WithSubjectAccessAssembler(newSubjectAccessAssembler(InstallationDB(pool))).
		WithInstallationName(consent.InstallationNameFunc(func(ctx context.Context) (string, error) {
			return identity.InstallationNameForPublicPage(ctx, pool)
		})).
		WithInstallationCountry(consent.InstallationCountryFunc(identity.CountryOf)).
		WithInstallationZone(identity.ZoneAppliedTx).
		WithMailLanguage(consent.MailLanguageFunc(identity.LanguageOf)).
		// The controller particulars form reads and writes through the settings
		// store; without it every read and write of the form is a server fault.
		WithSettings(StoredConfig(pool)).
		WithLocks(loadedLocks).
		// The edge that writes an accepted correction, through contacts' own
		// update path — so the subject's correction is governed by the same
		// gates as any other edit to that field.
		WithCorrectionApplier(correctionApplier{store: newContactsStore(InstallationDB(pool))}).
		// The privacy queues name the contacts and leads they point at through
		// the resolver every other surface names records with.
		WithRecordNames(newAttentionNames(InstallationDB(pool)))
}

func (o ownDomainReader) MailSeatIdentitiesSQL(ctx context.Context, tx pgx.Tx, bind func(any) int) (string, error) {
	return o.store.MailSeatIdentitiesSQL(ctx, tx, bind)
}
