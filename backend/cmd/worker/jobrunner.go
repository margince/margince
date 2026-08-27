// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The worker role's River wiring: the credential custodian and sweep registry
// the scheduled jobs share, the job configuration itself, and the bounded drain
// that stops the runner.
package main

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/certlog"
	"github.com/margince/margince/backend/internal/platform/dnsread"
	"github.com/margince/margince/backend/internal/platform/geocode"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/platform/overlaybudget"
	"github.com/margince/margince/backend/internal/platform/webread"
)

// gmailWatchConfig builds the Gmail push-watch maintenance config: the
// watch job runs only where a Pub/Sub topic is configured AND the Gmail
// app is wired (gmailWired); otherwise capture stays on the poll and the
// topic is left empty.
func gmailWatchConfig(cfg workerConfig, gmailWired bool) compose.GmailWatchConfig {
	w := compose.GmailWatchConfig{
		Interval:    cfg.gmailWatchInterval,
		RenewWithin: cfg.gmailWatchRenew,
	}
	if gmailWired {
		w.Topic = cfg.gmailPubsubTopic
	}
	return w
}

// startJobRunner boots the River periodic jobs: River
// gives leader election (one run cluster-wide, so worker replicas never
// double-sweep the close-date and reconcile passes), retries, and graceful
// drain — what the bare tickers lacked. The domain logic (Sweep/Reconcile)
// is unchanged; only the scheduler is River now. The returned stop function
// drains in-flight jobs on shutdown.
func startJobRunner(ctx context.Context, pool *pgxpool.Pool, rdb *redis.Client, vault keyvault.Vault, overlayBudget overlaybudget.Config, logger *slog.Logger, cfg workerConfig, modelPath compose.ModelPath, boundModels map[string]map[string]bool, lanes workerLanes, weeklyMail compose.WeeklyMailConfig, stdout io.Writer) (func(), error) {
	// The sweep registry is always live — the standing IMAP connector needs
	// no deployment config; gmail joins it when the OAuth app is configured.
	// The vault holds every connection's sealed credential (the standing
	// flavors resolve through it), so it is wired here regardless. The SAME
	// vault is the credential custodian of the two pollers this role runs — the
	// overlay reconcile (the only one that can resolve a connected workspace's
	// sealed HubSpot token, overlay.DueOverlayConnections' CredentialRef) and
	// the Telegram getUpdates poll (a bot's sealed token) — and it is the
	// boot's, not a second resolution of it; when none is configured it is nil,
	// so an unconfigured deployment never fails worker boot over pollers it has
	// no connected workspace to run anyway.
	// THIS is the role that pulls mailboxes, so the object store a captured
	// file is written to has to reach the config the sync registry and the job
	// lanes are built from. Wired in the api role alone, every inbound
	// attachment would be dropped in production while every test that composes
	// an api server passed.
	cfg.captureConfig.Blob = lanes.blob
	captureReg := compose.CaptureSyncRegistry(pool, vault, compose.GmailConfig{
		ClientID:     cfg.gmailClientID,
		ClientSecret: cfg.gmailClientSecret,
	}, compose.GraphConfig{
		ClientID:     cfg.graphClientID,
		ClientSecret: cfg.graphClientSecret,
		Tenant:       cfg.graphTenant,
	}, cfg.captureConfig, logger).WithSyncInterval(cfg.gmailSyncInterval)
	watchCfg := gmailWatchConfig(cfg, cfg.gmailAppWired())

	// The extension tier's per-call Runtime, bound before the runner exists
	// rather than beside the Surface-B lane that also binds it. This is the
	// binding the job lane depends on: a composed extension job is a River kind
	// worked HERE, on every worker, including one with no model configured —
	// where startRunnerLane returns at its AgentLoop guard without ever
	// binding. An unbound process is not a crash (every capability answers
	// errExtensionRuntimeUnwired, a clean refusal) but a tick that can only
	// ever refuse is not a working job.
	//
	// Unconditional, and that is the point: a guard here would reintroduce
	// exactly the shape that left the job lane unbound. `vault` is the boot's,
	// already nil where none is configured, and it is the same value
	// startRunnerLane passes — so the two bindings are idempotent.
	compose.BindExtensionRuntime(pool, vault)
	// The capture pipeline a unit's ingress lands through, bound in the same
	// breath and on the same terms. THIS role is the one that matters: a record
	// is ingested by unattended work only — a job tick or a subscription
	// delivery — and both of those run here. A worker that bound the runtime
	// and not this would serve every other capability and answer
	// errIngressUnwired to the one capability a connector unit exists for.
	//
	// It takes cfg.captureConfig, which is the same deployment config the sweep
	// registry above is built from, blob store included: a unit's captured
	// message and a mailbox's then reach one set of suppression lists and one
	// attachment store rather than two that drift.
	compose.BindExtensionCapture(pool, cfg.captureConfig)

	runner, err := newJobRunner(pool, logger, cfg, captureReg, watchCfg, vault, lanes, rdb, overlayBudget, modelPath, boundModels, weeklyMail)
	if err != nil {
		return nil, err
	}
	if err := runner.Start(ctx); err != nil {
		return nil, err
	}
	_, _ = fmt.Fprintln(stdout, jobRunnerBanner(cfg, watchCfg, modelPath, vault, lanes.runner))
	return func() { stopJobRunner(ctx, runner, logger) }, nil
}

// jobLane is the job runner as SHUTDOWN sees it: two ways to stop, one softer
// than the other. Named as an interface so the escalation between them can be
// driven by a test — a real drain overrun needs a job that will not finish,
// which is the one thing a test cannot arrange without waiting for it.
type jobLane interface {
	Stop(ctx context.Context) error
	StopAndCancel(ctx context.Context) error
}

// jobDrainWindow bounds the graceful drain: a job caught mid-flight by shutdown
// gets this long to finish on its own terms.
const jobDrainWindow = 30 * time.Second

// jobCancelWindow bounds the wait AFTER the work contexts are cancelled. Short,
// because nothing is being given time to finish here — only to notice it was
// cancelled and return.
const jobCancelWindow = 5 * time.Second

// stopJobRunner ends the job lane before this process closes what the jobs
// write through.
//
// The drain is bounded, and River's Stop RETURNS on that deadline rather than
// enforcing it — in-flight job goroutines keep running. Shutdown does not wait
// for them: run() closes the bus and then the pool as its deferred calls
// unwind, so an overrun used to leave a job writing an overlay budget meter
// into a closed Redis client, or reading through a closed pool. The failure
// lands in whatever the job logs, at shutdown, where it reads as a symptom of
// stopping rather than of a job that was never stopped.
//
// So an overrun escalates rather than proceeding. Cancelling the work contexts
// and waiting again is what actually ends those goroutines; River marks a job
// cancelled this way for retry, so the cost is a job that runs again, not one
// that is lost.
//
// If even that overruns, the process closes its connections under live job
// goroutines — the same shape as before, but named at Error with what will
// fail, rather than left to be inferred from a downstream write's complaint.
func stopJobRunner(ctx context.Context, lane jobLane, logger *slog.Logger) {
	// The run context is already cancelled at shutdown, so give the
	// drain its own bounded window.
	stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), jobDrainWindow)
	defer cancel()
	drained := lane.Stop(stopCtx)
	if drained == nil {
		return
	}
	logger.Warn("the job drain did not finish inside its window; cancelling the jobs still in flight so they cannot outlive the bus and the pool this process is about to close",
		"window", jobDrainWindow, "err", drained)
	cancelCtx, cancelHard := context.WithTimeout(context.WithoutCancel(ctx), jobCancelWindow)
	defer cancelHard()
	if err := lane.StopAndCancel(cancelCtx); err != nil {
		logger.Error("job goroutines are STILL running as this process closes the bus and the pool; whatever they write next fails against a closed client, and that failure will look like a shutdown problem rather than this one",
			"window", jobCancelWindow, "err", err)
	}
}

// newJobRunner declares every scheduled lane this role runs, and for each the
// deployment condition that turns it on — or the omission that honestly leaves
// it off. One declaration, so no lane can be enabled by one boot phase and
// starved by another.
func newJobRunner(pool *pgxpool.Pool, logger *slog.Logger, cfg workerConfig, captureReg *capture.Registry, watchCfg compose.GmailWatchConfig, vault keyvault.Vault, lanes workerLanes, rdb *redis.Client, overlayBudget overlaybudget.Config, modelPath compose.ModelPath, boundModels map[string]map[string]bool, weeklyMail compose.WeeklyMailConfig) (*jobs.Runner, error) {
	// Firing a scheduled message stages its delivery and enqueues the dispatch
	// job, through the SAME machinery an immediate send uses. Insert-only, like
	// the api's: this role works what it inserts, and a stager built on the
	// runner being assembled here would need that runner to already exist.
	sendInserter, err := jobs.NewInserter(pool, logger)
	if err != nil {
		return nil, err
	}
	return compose.NewJobRunner(pool, logger, compose.JobRunnerConfig{
		SendDelivery: compose.NewDeliveryStager(pool, sendInserter),
		// The send lane reads attachment bytes from the same object store
		// capture writes them to; without it a message carrying files fails at
		// the read rather than going out without them.
		SendBlob: lanes.blob,
		// The geocoder, when this deployment has one. Empty leaves it nil, the
		// worker records that it cannot resolve, and radius queries stay
		// unavailable — which is honest for an installation that geocodes
		// nothing, and better than answering from an empty table.
		Geocoder:  geocoderFor(cfg.geocodeBaseURL),
		Geocoding: compose.GeocodingConfig{BackfillInterval: cfg.geocodeBackfill},
		// The technical lookup, when the operator turned it on. Nil leaves the
		// sweep unregistered and the button answering 501 — declared absent
		// rather than a lane that queues into a process that will not read.
		TechnicalEnricher:   technicalEnricherFor(cfg, pool),
		TechnicalEnrichment: compose.TechnicalEnrichmentConfig{BackfillInterval: cfg.technicalBackfill},
		// The registry that resolves a staged delivery's mailbox: the SAME
		// sweep registry the capture polls use, so the connector set that
		// syncs a mailbox is the one that transmits from it.
		SendRegistry:      captureReg,
		SendPacing:        compose.SendPacing{Limit: cfg.sendRateLimit, Window: cfg.sendRateWindow, MaxAge: cfg.sendMaxAge},
		CloseDateInterval: cfg.closeDateInterval,
		ReconcileInterval: cfg.reconcileInterval,
		TimeScanInterval:  cfg.timeScanInterval,
		// The GDPR retention fan-out's cadence: --retention-interval is the
		// schedule source, now read by River rather than by a ticker.
		PrivacyRetention: compose.PrivacyRetentionConfig{Interval: cfg.retentionInterval},
		// A nil deliverer (no --webhook-key) registers neither half.
		WebhookRetry: compose.WebhookRetryConfig{Interval: cfg.webhookRetryInterval, Deliverer: lanes.deliverer},
		// A nil service (no declared model) registers neither half.
		AgentScheduler: compose.AgentSchedulerConfig{Interval: cfg.runnerInterval, Service: lanes.runner},
		// Provider-run execution. Both halves are required: an adapter to
		// call and a vault to unseal its credential with. Either absent
		// registers nothing, which is the honest posture for a deployment
		// with no provider configured — nothing can reach a vendor at all
		// (PI-AC-9). lanes.providers is nil unless MARGINCE_PROVIDER_SURFE
		// named a mode.
		ProviderRuns:  compose.ProviderRunsConfig{Registry: lanes.providers, Vault: vault},
		GmailRegistry: captureReg,
		GmailWatch:    watchCfg,
		// The Telegram ingest worker builds its Sink from this — the same
		// suppression-list config every other capture path shares.
		CaptureConfig: cfg.captureConfig,
		// Telegram ingress pulls, so the WORKER role owns it end to end: the
		// dispatcher and the poll jobs both need the vault that holds each bot's
		// sealed token. Without a configured vault there is no token to unseal
		// and the poller stays off by omission, the same posture the overlay
		// poller takes one field below.
		ChannelVault: vault,
		// The classify + enrich passes run only where a model is
		// configured; without one both are absent by omission.
		ClassifyBrain:      modelPath.CaptureClassify,
		VerdictBrain:       modelPath.CaptureCounterpartyVerdict,
		EnrichBrain:        modelPath.Enrich,
		SignalExtractBrain: modelPath.SignalExtract,
		WeeklyReviewBrain:  modelPath.WeeklyReview,
		// The retrospective's outbound channel, resolved in main from the same
		// deployment file cmd/api reads. Zero mails nothing.
		WeeklyMail:             weeklyMail,
		TranscriptProposeBrain: modelPath.TranscriptPropose,
		DocumentExtractBrain:   modelPath.DocumentExtract,
		OverlayVault:           vault,
		OverlayInterval:        cfg.overlayInterval,
		OverlayBackfillLimit:   cfg.overlayBackfillLimit,
		// The poller's OVB meter records against the SAME Redis the relay
		// uses (rdb) so the worker's poller spend and the api's force-fresh
		// spend land on one shared per-workspace-per-incumbent count. Built
		// here in cmd (the raw-Redis dependency stays out of compose).
		OverlayMeter: overlaybudget.New(rdb, overlayBudget),
		// The deep-read worker registers regardless: without a model path
		// (nil SiteExtract) it fails a picked-up read honestly rather than
		// leaving it queued behind a job no one can work.
		DeepReadBrain:       modelPath.SiteExtract,
		DeepReadFactBrain:   modelPath.SiteFactExtract,
		DeepReadTriageBrain: modelPath.SiteTriage,
		// Same posture for the voice build: the worker registers with or
		// without a model, failing picked-up builds actionably when brainless.
		VoiceBrain: modelPath.VoiceBuild,
		// The rate-refresh producers register regardless; without a source
		// (empty FX url / no pricing sources) or a model (nil RateExtract)
		// they no-op honestly. FX and model-cost both extract from a fetched
		// page via the shared RateExtract lane.
		RateExtractBrain:      modelPath.RateExtract,
		FxSourceURL:           cmp.Or(cfg.ratesFx, "https://api.frankfurter.dev/v1/latest"),
		FxBootstrapCurrencies: fxBootstrapCurrencies(cfg.ratesCurrencies),
		FxExtractBrain:        modelPath.RateExtract,
		ModelPricingSources:   compose.PricingSourcesFromMap(cfg.ratesModelPricing),
		BoundModelIDs:         boundModels,
		DeepReadCaps:          compose.CrawlCaps{MaxPages: cfg.deepReadMaxPages, MaxBytes: cfg.deepReadMaxBytes, Wall: cfg.deepReadWall},
		// The same object store retention purges from: a deep read resolves
		// the company's logo out of the site it just crawled and stores the
		// normalized bytes here. Nil (no blobstore configured) leaves every
		// company on its monogram — the read itself is unaffected.
		Blobstore: lanes.blob,
		// The embed-reindex worker registers regardless: without an embed
		// lane (nil Embedder) a picked-up job fails clearly rather than
		// sitting queued forever behind a job no one can work — the same
		// posture as DeepReadBrain above.
		Embedder: modelPath.Embedder,
	})
}

// geocoderFor builds the provider a base URL names, or nil for none.
//
// "public" is spelled out rather than defaulted, so choosing OpenStreetMap's
// free service is a decision somebody made rather than what happens when a
// variable is unset. Its terms hold a recurring client to four requests a
// minute; an installation with real volume points this at its own instance.
//
// technicalEnricherFor builds the three-lane reader, or nil when this
// deployment reads none of it.
//
// All three lanes are wired together or not at all: a partial enricher would
// complete some lanes and never the others, and a lane that never completes
// leaves its facts frozen at whatever the last full run saw — which reads on
// the record exactly like a company that has not changed.
//
//nolint:ireturn // the PORT is the return type: nil means this deployment geocodes nothing, which a concrete type cannot express.
func technicalEnricherFor(cfg workerConfig, pool *pgxpool.Pool) *compose.TechnicalEnricher {
	if cfg.certLogBaseURL == "" {
		return nil
	}
	baseURL := cfg.certLogBaseURL
	if baseURL == baseURLPublic {
		baseURL = certlog.PublicBaseURL
	}
	return compose.NewTechnicalEnricher(
		dnsread.New(dnsread.NewPacer(dnsReadInterval)),
		certlog.NewCrtSh(baseURL, nil),
		webread.New(),
		people.NewStore(compose.InstallationDB(pool)),
		nil,
	)
}

// dnsReadInterval paces this installation's resolver queries.
//
// Public resolvers are far more tolerant than a certificate log, so this is a
// throughput floor rather than a policy one: fast enough that a company's
// handful of lookups is not the slow part, slow enough that a fleet sweep is
// not a burst anybody notices.
const dnsReadInterval = 200 * time.Millisecond

// baseURLPublic is the word an operator writes to mean "the free public
// service", for whichever lane the flag belongs to.
const baseURLPublic = "public"

func geocoderFor(baseURL string) geocode.Client {
	switch baseURL {
	case "":
		return nil
	case baseURLPublic:
		return geocode.NewNominatim(geocode.PublicBaseURL, nil)
	default:
		return geocode.NewNominatim(baseURL, nil)
	}
}
