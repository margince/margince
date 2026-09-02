// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The deep read end-to-end: a
// human's start queues a durable crawl job and answers 202; the worker
// role crawls the organization's site under the bounded siteCrawler,
// folds the pages into a labeled corpus, and extracts it in ONE model
// call (chunked only for outsized sites) through the no-guess evidence
// gate — company fields, category facts, published people, and the
// site's legal-entity census. The gated findings LAND directly, in one
// transaction: profile fields fill-empty exactly like a quick scrape,
// category facts land in organization_fact. Nobody is asked to confirm a
// read they pressed the button for — the write is marked as model-derived
// and stays reversible instead. Published PEOPLE are the exception and
// still stage as leads. The dossier (people's site_read row) is the
// transparency surface the SPA polls: live phase and page counts while
// running, then what was read and what it found.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/webread"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

// SiteDeepReadArgs is one queued deep read. The args carry what the worker
// role needs to run without a request context: the tenant, the target, the
// dossier to advance, and the requesting human for the staged proposal's
// provenance until the claim yields the authoritative one.
//
// The seed URL is NOT among them. The worker crawls claim.SeedURL — the
// dossier row is the authority on what is read, exactly as it is on who asked
// — so a copy in the args would be an address sitting in a table with no
// workspace column and no RLS that no code path ever reads.
type SiteDeepReadArgs struct {
	Workspace      ids.UUID `json:"workspace_id"`
	OrganizationID ids.UUID `json:"organization_id"`
	SiteReadID     ids.UUID `json:"site_read_id"`
	RequestedBy    string   `json:"requested_by"`
	// MaxPages is this run's page ceiling, or 0 for the deployment's own. It
	// can only ever narrow: the worker clamps it against the configured cap, so
	// a payload cannot raise what an operator set.
	MaxPages int `json:"max_pages,omitempty"`
}

// Kind is the stable job identifier River persists in river_job.
func (SiteDeepReadArgs) Kind() string { return "site_deep_read" }

// WorkspaceID binds this deep read to its tenant (jobs.WorkspaceScoped).
// The field is Workspace because Go forbids a field and a method of the
// same name; the wire key stays workspace_id.
func (a SiteDeepReadArgs) WorkspaceID() ids.UUID { return a.Workspace }

// deepReadQueue isolates deep reads from the default queue: a crawl holds a
// worker for minutes (crawl wall + model calls), so a burst of them on the
// shared queue would starve the short maintenance jobs. Its own bounded pool
// (deepReadMaxWorkers) caps how much of the fleet crawling can occupy.
const (
	deepReadQueue      = "deep_read"
	deepReadMaxWorkers = 2
)

// siteDeepReadInsertOpts routes the job to its own queue and deduplicates by
// args: the dossier id is unique per read, so a re-submitted enqueue of the
// SAME read collapses while a fresh read (new dossier) always queues.
func siteDeepReadInsertOpts() *river.InsertOpts {
	return &river.InsertOpts{
		Queue:      deepReadQueue,
		UniqueOpts: river.UniqueOpts{ByArgs: true},
	}
}

// siteDeepReadWorker runs one queued deep read: claim the dossier, crawl,
// extract, stage, report. It is always registered on the worker role —
// with no model path it fails the read honestly instead of leaving it
// queued forever.
type siteDeepReadWorker struct {
	// pool opens the ONE transaction an act's proposals are staged in, so the
	// inbox never shows half of a read's findings as a whole question.
	pool    *pgxpool.Pool
	people  *people.Store
	crawler *siteCrawler
	extract evidenceExtractor
	// triageBrain answers the domain-triage classification. Its own lane, not
	// the extractor's: one cheap question of one page must not bill the profile
	// lane's premium-only ladder. Nil is a role that cannot classify, which
	// settles a domain from what the workspace already knows instead.
	triageBrain completer
	// fetch is the same guarded egress the crawl uses, for the ONE thing the
	// crawl does not fetch itself: the logo asset the seed page declared.
	fetch assetFetcher
	// blob holds the normalized logo bytes. Nil is a worker role with no
	// object store: it reads sites and resolves no logos, and every company
	// keeps its monogram.
	blob      blobstore.Store
	approvals *approvals.Service
	// authority is identity's live RBAC resolver. The worker acts as a system
	// principal, which sees every row; anything it decides ON BEHALF OF the
	// requesting human has to be decided within THAT human's scope instead —
	// see probeCtx.
	authority  authz.Resolver
	autoEnrich *capture.AutoEnrichStore
	// settings answers the auto-enrich flag at CLAIM time. The sweep checks it
	// too, but a job queued while the flag was on outlives that check: without
	// re-reading here, switching the feature off would keep costing crawls and
	// model calls until the queue drained.
	settings *capture.SettingsStore
	log      *slog.Logger
	caps     CrawlCaps
	now      func() time.Time
}

// newSiteDeepReadWorker assembles the worker-role deep read over one
// shared egress fetcher: the crawler walks pages through it and the
// extractor carries the same seam. brain may be nil — a picked-up read
// then finishes failed with an actionable log rather than sitting queued
// behind a worker that cannot extract.
func newSiteDeepReadWorker(pool *pgxpool.Pool, brain, factBrain, triageBrain completer, log *slog.Logger, caps CrawlCaps, blob blobstore.Store) *siteDeepReadWorker {
	fetcher := webread.New()
	caps = caps.withDefaults()
	return &siteDeepReadWorker{
		pool:        pool,
		people:      people.NewStore(InstallationDB(pool)),
		crawler:     newSiteCrawler(fetcher, caps),
		extract:     evidenceExtractor{fetch: fetcher, brain: brain, factBrain: factBrain},
		triageBrain: triageBrain,
		fetch:       fetcher,
		blob:        blob,
		approvals:   approvals.NewService(InstallationDB(pool)),
		authority:   identity.NewService(pool),
		autoEnrich:  capture.NewAutoEnrichStore(InstallationDB(pool)),
		settings:    capture.NewSettings(NewSettingsStore(pool)),
		log:         log,
		caps:        caps,
		now:         time.Now,
	}
}

// extractLaneBudget is the extraction's allowance in the job-timeout
// arithmetic: the page fan-out and the first profile call run concurrently,
// each a small fast call plus the validator's retry-and-escalate headroom.
//
// The allowance covers TWO profile calls, not one. A crawl that grows well
// past the profile trigger is asked again over the finished corpus
// (compose.reprofileOverWholeCrawl), and that call is serial — it reads the
// whole crawl, so it cannot start until the crawl has ended. Sizing this for
// one call would time out exactly the large sites the re-run exists for.
const extractLaneBudget = 150 * time.Second

// deepReadTimeout is the one declared timeout in the tree that cannot be a
// number in api/jobs.yaml: the crawl wall is an operator's, so the file
// declares {operator: DeepReadCaps} and the value is computed here and handed
// over at registration.
//
// It is the crawl wall, plus the parallel extraction budget, plus the logo
// lane's own bounded spend, plus a minute for the staging and dossier writes —
// floored at eight minutes so a tightened cap never squeezes the terminal
// writes. Every lane that can hold the job is counted here, or a slow one
// silently eats the allowance the terminal write depends on.
//
// It defaults the caps first, so a caller passing the zero CrawlCaps gets the
// budget the crawler will actually spend rather than one derived from a wall
// of zero.
func deepReadTimeout(caps CrawlCaps) time.Duration {
	budget := caps.withDefaults().Wall + extractLaneBudget + logoLaneBudget + time.Minute
	if floor := 8 * time.Minute; budget < floor {
		return floor
	}
	return budget
}

// reclaimAfter leaves a terminal-write grace beyond River's work timeout.
// A replacement worker may reclaim only after the prior worker has exceeded
// both its configured crawl budget and the time reserved to close the dossier.
func (w *siteDeepReadWorker) reclaimAfter() time.Duration {
	return deepReadTimeout(w.caps) + time.Minute
}

func (w *siteDeepReadWorker) run(ctx context.Context, args SiteDeepReadArgs) error {
	ctx = deepReadWorkerCtx(ctx, args)

	claim, err := w.people.BeginSiteRead(ctx, args.SiteReadID, w.reclaimAfter())
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			// The CAS miss: the read is no longer queued — a rival replica
			// claimed it, or a prior attempt already recorded its outcome.
			return nil
		}
		return fmt.Errorf("site deep read %s: begin: %w", args.SiteReadID, err)
	}
	// Everything past the claim runs as the requester the ROW names — the
	// dossier row is the authority on who asked, never the job payload — so the
	// provenance on what this read writes is the row's answer too.
	ctx = withClaimedRequester(ctx, claim.RequestedBy, args.SiteReadID)
	// Every model lane this read runs names the site, so the AI-activity rail
	// can say which website is being read rather than that one is.
	ctx = principal.WithWorkSubject(ctx, siteHostOf(claim.SeedURL))

	if settled, err := w.routeClaimedRead(ctx, args, claim); settled || err != nil {
		return err
	}

	if err := w.people.UpdateSiteReadProgress(ctx, args.SiteReadID, "crawling", nil); err != nil {
		w.log.WarnContext(ctx, "site read progress update failed", "read", args.SiteReadID.String(), "err", err)
	}
	// Crawl and extraction OVERLAP (crawlAndExtract): page calls launch
	// as pages commit, so the crawl's slow tail hides behind extraction.
	// The crawler owns the wall clock (caps.Wall); a seed page that
	// cannot be read at all is a failed read, not an empty one.
	progress, publishDraft := w.progressiveCallbacks(ctx, args.SiteReadID)
	crawler := w.crawler.withPageCeiling(w.pageCeiling(claim.RequestedBy, args.MaxPages))
	crawl, extraction, err := crawlAndExtract(ctx, crawler, w.extract, claim.SeedURL, progress, publishDraft)
	if err != nil {
		if deferred, deferErr := w.deferForBudget(ctx, args.SiteReadID, err); deferred {
			return deferErr
		}
		return w.fail(ctx, args.SiteReadID, fmt.Errorf("site deep read %s: %w", args.SiteReadID, err))
	}
	// From here on the read is about the site that ANSWERED. The seed is a
	// guess derived from the domain, and the fallback ladder may have reached
	// the company on www or over http; a proposal, a fact's evidence or a
	// staged lead that still named the original would cite a URL which served
	// nothing, and a human confirming it would be confirming a dead link.
	if crawl.SeedURL != "" {
		claim.SeedURL = crawl.SeedURL
	}
	// The logo lands as soon as the CRAWL succeeded, before anything the model
	// lane produced is judged: it is a 🟢 display asset (A55) that no human has
	// to approve, and it is read off the seed page's own markup, so a company
	// gets its face even on a read whose extraction later comes back empty or
	// dies outright.
	w.resolveLogo(ctx, args, claim, crawl)
	// Beside the logo and for the same reasons: a side fetch on the same
	// guarded egress, under its own deadline, whose failure is never the
	// read's. A company that publishes no newsroom is the ordinary case.
	w.readNewsroom(ctx, claim, crawl)
	// What the pages this crawl already fetched DECLARE about the software
	// serving them — read from the response headers, cookies, scripts and
	// markup each one arrived with, so no page is asked for twice
	// (sitetechnology.go).
	w.readSiteTechnology(ctx, claim, crawl)
	// The last side lane, and the only one that queues rather than fetches:
	// what the company publicly runs according to sources the company never
	// wrote — its DNS and its certificate history. Both are paced on their own
	// single-threaded queue, so a deep-read worker hands the question over
	// instead of waiting on it (technicalonsiteread.go).
	w.askWhatTheCompanyRuns(ctx, claim)

	return w.reportRead(ctx, args, claim, crawl, extraction)
}

// routeClaimedRead dispatches a claimed read to the lane that owns it, before
// any spend: the disabled-auto-enrich close, the domain-triage lane (which
// decides whether an organization should exist at all, so it cannot share the
// enrichment path, which assumes one to enrich), or the honest failure of a
// worker role with no model path. It reports whether it settled the read; not
// settled is the ordinary enrichment read, which the caller crawls itself.
func (w *siteDeepReadWorker) routeClaimedRead(ctx context.Context, args SiteDeepReadArgs, claim people.SiteReadClaim) (bool, error) {
	// Before ANY spend: an operator who turned auto-enrich off gets no further
	// crawling and no further model calls, including from work queued while it
	// was on. Only the automatic lane is gated — a human who asked for a read
	// is not governed by the automatic-enrichment setting.
	//
	// claim.RequestedBy, never args: the dossier row is the authority on who
	// asked for this read, and the job payload is metadata that travelled
	// beside it. Everything downstream that decides SPEND or AUTHORITY reads
	// the row — a payload disagreeing with it must not buy a wider budget or
	// skip a confirm-first proposal.
	if isSystemRead(claim.RequestedBy) {
		enabled, err := w.autoEnrichEnabled(ctx)
		if err != nil {
			// Recorded, not returned raw: the read is already claimed, so a
			// bare error would leave it running until the reclaim window
			// expires. Every other fault on this path records itself, and the
			// sweep re-enqueues the org on its next pass.
			return true, w.fail(ctx, args.SiteReadID,
				fmt.Errorf("site deep read %s: reading the auto-enrich setting: %w", args.SiteReadID, err))
		}
		if !enabled {
			// A triage read may not simply stop here. Its domain is a question
			// somebody's mail already asked, and abandoning it would leave that
			// question open forever — no organization for that domain, ever.
			// Answering it from what the workspace already knows is the honest
			// close, and it is what the operator's "don't crawl" actually means.
			if isDomainTriageRequest(claim.RequestedBy) {
				return true, w.triageWithoutLooking(ctx, args, claim)
			}
			return true, w.abandon(ctx, args.SiteReadID, "auto_enrich_disabled")
		}
	}
	// A role with no model path answers a triage the same way a disabled
	// setting does, rather than failing a question the sweep would then
	// re-ask forever.
	if isDomainTriageRequest(claim.RequestedBy) {
		if w.triageBrain == nil || w.extract.brain == nil {
			return true, w.triageWithoutLooking(ctx, args, claim)
		}
		return true, w.runTriage(ctx, args, claim)
	}
	if w.extract.brain == nil {
		return true, w.fail(ctx, args.SiteReadID,
			errors.New("site deep read: worker has no model path — configure --ai-routing (or --ai-fake) on the worker role"))
	}
	return false, nil
}

func (w *siteDeepReadWorker) progressiveCallbacks(ctx context.Context, readID ids.UUID) (func(string, []crawlPage), func(pageFactsResult)) {
	progress := func(phase string, pages []crawlPage) {
		if err := w.people.UpdateSiteReadProgress(ctx, readID, phase, siteReadPages(pages)); err != nil {
			w.log.WarnContext(ctx, "site read progress update failed", "read", readID.String(), "err", err)
		}
	}
	publishDraft := func(partial pageFactsResult) {
		found := siteReadPeople(partial.people)
		entities := siteReadLegalEntities(partial.entities)
		hash, err := siteReadProposalHash(nil, partial.facts, found, entities)
		if err != nil {
			w.log.WarnContext(ctx, "site read progressive draft hash failed", "read", readID.String(), "err", err)
			return
		}
		if err := w.people.UpdateSiteReadDraft(ctx, readID, partial.facts, found, entities, hash); err != nil {
			w.log.WarnContext(ctx, "site read progressive draft update failed", "read", readID.String(), "err", err)
		}
	}
	return progress, publishDraft
}

// siteReadLegalEntities projects the gated census onto the dossier shape.
// The abstention refuses to APPLY a legal identity it cannot attribute; it
// never had a reason to forget the identities it read, and the confirm
// step turns them into the choice only a human can make.
func siteReadLegalEntities(entities []corpusLegalEntity) []people.SiteReadLegalEntity {
	out := make([]people.SiteReadLegalEntity, 0, len(entities))
	for _, e := range entities {
		out = append(out, people.SiteReadLegalEntity{
			Name:              e.Name,
			RegisteredAddress: e.RegisteredAddress,
			RegisterNumber:    e.RegisterNumber,
			VatNumber:         e.VatNumber,
			EvidenceSnippet:   e.EvidenceSnippet,
			SourceURL:         e.SourceURL,
		})
	}
	return out
}

// finish records the crawl report on the dossier in one terminal write.
// terminalCtx derives the context for a terminal dossier write: the work
// context's VALUES (principal, workspace — WithWorkspaceTx reads the tenant
// GUC from them) with a fresh deadline of its own, NEVER the work context's
// deadline or cancellation. Closing the dossier must not be starved by the
// crawl+extract work it reports on — otherwise a read whose model calls
// exhausted the job budget is left running forever, squatting the org's one
// in-flight slot. Fifteen seconds bounds the single FinishSiteRead tx.
func terminalCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
}

func (w *siteDeepReadWorker) finish(ctx context.Context, readID ids.UUID, claim people.SiteReadClaim, status string, readPages []crawlPage, crawl siteCrawl, factCount int, proposalIDs []ids.UUID, fields []people.DeepReadField, facts []people.DeepReadFact, found []people.SiteReadPerson, entities []people.SiteReadLegalEntity, warnings []string, proposalHash string) error {
	in := people.FinishSiteReadInput{
		ClaimedAt:     &claim.ClaimedAt,
		Status:        status,
		Pages:         make([]people.SiteReadPage, 0, len(readPages)),
		Skipped:       make([]people.SiteReadSkip, 0, len(crawl.Skipped)),
		FactCount:     factCount,
		ProposalIDs:   proposalIDs,
		ProfileFields: fields,
		Facts:         facts,
		People:        found,
		LegalEntities: entities,
		Warnings:      warnings,
		ProposalHash:  proposalHash,
	}
	for _, p := range readPages {
		in.Pages = append(in.Pages, people.SiteReadPage{URL: p.URL, Kind: string(p.Kind)})
	}
	for _, s := range crawl.Skipped {
		in.Skipped = append(in.Skipped, people.SiteReadSkip{URL: s.URL, Reason: string(s.Reason)})
	}
	if crawl.Stopped != nil {
		reason := string(*crawl.Stopped)
		in.StoppedReason = &reason
	}
	tctx, cancel := terminalCtx(ctx)
	defer cancel()
	if err := w.people.FinishSiteRead(tctx, readID, in); err != nil {
		return fmt.Errorf("site deep read %s: finish: %w", readID, err)
	}
	return nil
}

// deepReadProposalKind is the staged proposal's wire identity — one
// spelling for the staging worker and the accept executor
// (deepreadaccept.go). Distinct from the quick scrape's "enrich": a deep
// read's acceptance also lands category facts.
const deepReadProposalKind = "deepread"
