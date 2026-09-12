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
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/customfields"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/finance"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/integrations"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/config"
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
	return contacts.NewHandlers(InstallationDB(pool)).
		WithFieldCatalog(customfields.NewService(pool, nil)).
		WithMatchStager(linkedInMatchStager(pool)).
		WithVCardReviewStager(vcardCreateStager(pool)).
		WithSettings(NewSettingsStore(pool)).
		WithSeatReadsLeads(seatReadsLeads(pool)).
		WithDealOpener(leadDealOpener{deals: deals.NewStore(InstallationDB(pool), DealsInstallation())}).
		// A merge carries the retiring subject's stops, or it refuses. consent
		// owns communication_suppression; contacts owns the merge; neither
		// imports the other, so the edge is injected here.
		WithStopCarrier(consent.NewStore(InstallationDB(pool)))
}

// newFinanceHandlers builds the invoicing transport over the two edges it
// cannot reach for itself: the installation's base currency, and who at the
// account receives an invoice.
//
// The billing-contact edge is an injection rather than an import for the
// ordinary ADR-0054 reason — contacts owns relationship, finance owns the
// invoice — and it matters more than usual here: the same read serves the
// company page's own billing section, so a second one would let the finance
// card and the account panel disagree about who the recipient is.
func newFinanceHandlers(pool *pgxpool.Pool) finance.Handlers {
	return finance.NewHandlers(InstallationDB(pool), identity.BaseCurrencyOf).
		WithBillingContacts(financeBillingContacts{contacts: contacts.NewStore(InstallationDB(pool))})
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
		WithConsent(gate).
		WithSendPreview(gate).
		// The public booking capture seams (feedback/14): contacts is the
		// idempotent-on-email contact path, consent records the
		// passthrough — both injected here, never sibling imports.
		WithPublicBooking(contacts.NewStore(InstallationDB(pool)), bookingConsentAdapter{store: consent.NewStore(InstallationDB(pool))}).
		// The RFC 8058 unsubscribe linker (B-E11.32): consent mints the
		// preference token behind the List-Unsubscribe URL.
		WithUnsubscribe(preferenceLinkAdapter{store: consent.NewStore(InstallationDB(pool))}).
		// The sender's own sign-off (core 0235). contacts owns the row because
		// it owns the contact the seat belongs to; activities appends it because
		// it owns the one send. The edge is injected here rather than imported,
		// like every other cross-module edge on this path.
		WithSignature(contacts.NewStore(InstallationDB(pool))).
		// The name on the envelope. identity owns who the acting human is —
		// including the human an agent acts on behalf of — and that resolution
		// must be the same one the audit log records, so it is injected rather
		// than re-derived here.
		WithSenderName(identity.NewServiceFor(InstallationDB(pool))).
		// Which domains are our own, for the waiting queue's colleague rule.
		// capture owns workspace_email_domain and the rule for which entries
		// count as vouched-for, so the edge is injected rather than restated.
		WithOwnDomains(ownDomainReader{store: capture.NewOwnDomainStore(InstallationDB(pool))}).
		// When a host is bookable. identity owns the setting because it is a
		// fact about a contact; this transport asks for it rather than holding
		// a pair of numbers for everybody (docs/explanation/scheduling.md).
		WithWorkingHours(workingHoursResolver(pool))
}

// ownDomainReader adapts capture's own-domain store to the seam the waiting
// queue takes. It borrows the caller's transaction so one read's strict and
// relaxed counts see a single snapshot of the domains.
type ownDomainReader struct{ store *capture.OwnDomainStore }

func (o ownDomainReader) Domains(ctx context.Context, tx pgx.Tx) ([]string, error) {
	return o.store.ColleagueDomainsTx(ctx, tx)
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
	return collections.NewStore(InstallationDB(pool)).WithFieldCatalog(customfields.NewService(pool, nil))
}

// newCollectionsHandlers builds the lists/tags/saved-views transport over
// NewCollectionsStore, so dynamic-list create validation and the members
// endpoint resolve a definition's vocabulary through collections.Store.
// SegmentEngine exactly as export does (wireExportSurface builds its store
// the same way) — a cf_* filter a saved list or a membership check names
// cannot be refused here while an export of the same list accepts it.
func newCollectionsHandlers(pool *pgxpool.Pool) collectionsHandlers {
	return collections.NewHandlers(NewCollectionsStore(pool))
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
func (s *Server) wireAnalyticsSurface(pool *pgxpool.Pool) {
	s.forecastHandlers = forecasting.NewHandlers(
		forecasting.NewStore(InstallationDB(pool)),
		ForecastDeals, ForecastPeriodAt, ForecastWritableScope,
		ForecastConversionHistory, ForecastForwardMeasure,
		func() time.Time { return time.Now().UTC() },
	)
	// The floor comes from the constant rather than a setting for now: one
	// number, and moving it to installation settings is a migration plus a
	// reader, which is its own change.
	s.analyticsQueryHandlers = newAnalyticsQueryHandlers(
		InstallationDB(pool), analyticsquery.DefaultFloor)
	s.analyticsContextHandlers = newAnalyticsContextHandlers(
		InstallationDB(pool), func() time.Time { return time.Now().UTC() })
	s.analyticsShareHandlers = newAnalyticsShareHandlers(
		NewAnalyticsShareStore(func() time.Time { return time.Now().UTC() }),
		forecasting.NewStore(InstallationDB(pool)),
		func() time.Time { return time.Now().UTC() },
	)
}

func (s *Server) wireCaptureSettingsSurface(pool *pgxpool.Pool) {
	// The workspace capture-settings surface (CAP-WIRE-7, ADR-0072):
	// read the auto-enrich posture (all roles), toggle it (admin/ops).
	s.captureSettingsHandlers = captureSettingsHandlers{store: capture.NewSettings(NewSettingsStore(pool))}
	// Whether contacts are looked up automatically for the details the provider
	// charges nothing for. Always wired, including where no provider is
	// connected: the posture is the installation's answer, not the
	// connection's, and an operator must be able to set it before connecting.
	s.integrationsSettingsHandlers = integrationsSettingsHandlers{store: integrations.NewSettings(NewSettingsStore(pool))}
	// The tier→model binding (ai-operational-spec §1.4): read it, replace it
	// without a restart. Always wired, including on an installation that has
	// bound nothing — an operator binding models for the first time reaches it
	// through the same surface as one re-pointing a lane.
	//
	// WithCatalogue wires the public OpenRouter model read unconditionally: it
	// needs no tenant credential, so there is no "no provider connected"
	// configuration to honor here.
	s.aiRoutingHandlers = aiRoutingHandlers{
		store: ai.NewRoutingStore(NewSettingsStore(pool), config.FromOS).
			WithCatalogue(ai.NewModelCatalogue(systemClock{})),
	}
	s.ownDomainHandlers = ownDomainHandlers{store: capture.NewOwnDomainStore(InstallationDB(pool))}
	// The installation's own identity and reporting basis (ADR-0090/A135):
	// name, reporting zone, base currency — the last of which locks once a
	// deal has converted against it (ADR-0085 §7).
	s.installationSettingsHandlers = installationSettingsHandlers{
		store:          identity.NewInstallationSettings(InstallationDB(pool), NewSettingsStore(pool)),
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
	s.overlayExportHandlers = newOverlayExportHandlers(pool, log)
}

// wireOnboardingSurface binds the first-run group: the installation's own
// company, the site read that seeds it, and the onboarding state the two
// report progress through — all three gated by the same rollout.
func (s *Server) wireOnboardingSurface(pool *pgxpool.Pool) {
	// The installation's own company (the 0083 anchor). Its own store
	// instance, like every other contacts-backed shadow here: the company
	// form's write shape is contacts's, the transport is compose's.
	s.companyHandlers = companyHandlers{store: contacts.NewStore(InstallationDB(pool)), rollout: companyContextRolloutOnboarding}
	s.siteReadHandlers = siteReadHandlers{companyContextRollout: companyContextRolloutOnboarding}
	s.onboardingStateHandlers = onboardingStateHandlers{
		state: identity.NewOnboardingStore(InstallationDB(pool)), company: contacts.NewStore(InstallationDB(pool)),
		proposal: &onboardingProposalEngine{
			state: identity.NewOnboardingStore(InstallationDB(pool)), contacts: contacts.NewStore(InstallationDB(pool)),
			rollout: companyContextRolloutOnboarding,
		},
	}
}

// wireSystemOfRecordReads builds the per-workspace native/overlay dispatch
// and the reads that ride it — the company view and its grounded prose.
func (s *Server) wireSystemOfRecordReads(pool *pgxpool.Pool) {
	// The overlay read dispatch is built with a nil live-incumbent resolver
	// here (force-fresh degrades to the mirror). WithKeyvault injects the
	// vault-backed resolver once the vault is known — the vault arrives via
	// an option applied AFTER newServer returns, and the dispatch/provider/
	// freshness reader are pointers shared across that return, so a
	// boot-time SetOverlayIncumbentResolver reaches the same instance this
	// field serves reads through.
	s.sorDispatch = NewDispatcher(NewProvider(pool), NewOverlayProvider(pool, s.overlayMeter, nil), pool)
	// The company view (company360) is assembled from THIS system of record;
	// it asks the same dispatch every other overlay-aware read asks, so a
	// workspace running on the incumbent mirror gets one honest refusal
	// instead of a page that quietly omits most of itself. Wired after the
	// dispatch because it needs it.
	// The contacts store carries the SAME fieldcatalog seam contactsHandlers
	// gets: the 360 serves the company object, and without it the
	// company view would silently omit the cf_* columns GET
	// /companies/{id} returns for the same record.
	// The brief reads THROUGH the 360 service, so it inherits every gate the
	// page itself applies and can only describe what this caller may see.
	// The model lane is nil here: WithAccountBrief binds the api role's
	// summarize lane, and without it the brief serves its deterministic
	// floor.
	s.contactsStore = contacts.NewStore(InstallationDB(pool)).WithFieldCatalog(customfields.NewService(pool, nil))
	s.blockedDomainHandlers = blockedDomainHandlers{contacts: s.contactsStore}
	s.captureExclusionHandlers = captureExclusionHandlers{store: capture.NewExclusionStore(InstallationDB(pool))}
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
	// The importer maps only core columns (see importTargets for why custom
	// fields are not among them), so it needs no field catalog of its own.
	s.importHandlers = importHandlers{db: InstallationDB(pool), uploadLimit: s.uploadLimits.CSVImport}
	s.company360Svc = company360.NewService(pool, s.contactsStore, s.dealsStore, ProjectsStore(pool), approvals.NewService(InstallationDB(pool)), time.Now)
	s.companyBriefSvc = companybrief.NewService(pool, s.company360Svc, s.contactsStore, nil, "", time.Now).
		WithEmailSummaries(emailRows(pool))
	s.companyBriefHandlers = companybrief.NewHandlers(s.companyBriefSvc, s.sorDispatch.isOverlay)
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
		s.companyDossierSvc, s.companyGrowthFitSvc, s.sorDispatch.isOverlay,
	)
	// The account scan over the same composite read and the same dismissals.
	// No lane and no job runner here: an ensure on this role settles the
	// rules' floor in-request, and WithAccountScan binds the api role's.
	s.companyScanSvc = companyscan.NewService(pool, s.company360Svc, s.company360Svc, nil, nil, nil, time.Now, s.log).
		WithEmailSummaries(emailRows(pool))
	s.companyScanHandlers = companyscan.NewHandlers(s.companyScanSvc, s.sorDispatch.isOverlay)
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
	s.accountDraftHandlers = accountdraft.NewHandlers(
		accountdraft.NewService(s.company360Svc, nil).
			WithEnvelope(draftEnvelope(pool, s.log)).
			WithEmailSummaries(emailRows(pool)).
			WithDossier(s.companyDossierSvc), s.sorDispatch.isOverlay,
	)
	s.company360Handlers = company360.NewHandlers(
		s.company360Svc,
		s.sorDispatch.isOverlay,
	)
	// The contact page is the company page's sibling and rides the same
	// dispatch, so it is wired here rather than beside the handler sets: a
	// workspace on the incumbent mirror refuses both the same way.
	s.wireContact360(pool)
	// After sorDispatch exists: the reversal reads the SAME dispatcher every
	// other write on this server does.
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
		// THE SAME APPROVALS ENGINE the inbox decides through, so a card raised
		// here is answered there rather than sitting in a second queue nobody
		// reads.
		WithReviewRouter(reviewRouter{approvals: approvalsServiceWithEffects(pool)}).
		WithEraser(privacy.NewEraser(InstallationDB(pool))).
		WithSubjectAccessAssembler(newSubjectAccessAssembler(InstallationDB(pool))).
		WithInstallationName(consent.InstallationNameFunc(func(ctx context.Context) (string, error) {
			return identity.InstallationNameForPublicPage(ctx, pool)
		})).
		WithInstallationCountry(consent.InstallationCountryFunc(identity.CountryOf)).
		WithMailLanguage(consent.MailLanguageFunc(identity.LanguageOf))
}
