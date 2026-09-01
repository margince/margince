// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Command worker is the background process role (ADR-0054, amended §2):
// the standalone outbox relay for split deployments — cmd/api runs the
// same relay inline by default (--inline-relay), so small installs never
// need this binary — plus the Surface-B runner scheduler when a brain is
// declared: catalog seeding, due-job execution, and the
// approval-decided resume subscriber.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
	// Embedded tzdata: workspace timezones must resolve on scratch
	// containers that ship no zoneinfo.
	_ "time/tzdata"

	// The composed extension set (ADR-0069): the generated module under
	// build/composition/ in a composed build, the committed vanilla stub
	// in a bare one — same import path either way.
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/margince/margince/composition"
	"github.com/redis/go-redis/v9"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/deployconfig"
	"github.com/margince/margince/backend/internal/platform/events"
	"github.com/margince/margince/backend/internal/platform/httpserver"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/platform/netguard"
	"github.com/margince/margince/backend/internal/shared/buildinfo"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/runtimeenv"
	"github.com/margince/margince/backend/pkg/extension"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "worker:", err)
		os.Exit(1)
	}
}

// run is the worker's boot sequence — the debug subcommands, flags,
// extensions, deployment file, logger, pool, bus, the event lanes and the job
// runner, then the relay it blocks on — in the order the process depends on;
// boot.go and jobrunner.go hold the phases.
func run(ctx context.Context, args []string, stdout io.Writer) error {
	if handled, err := runDebugSubcommand(ctx, args, stdout); handled {
		return err
	}

	boot, err := configureWorker(args, stdout)
	if err != nil {
		return err
	}
	cfg, deployCfg, logger := boot.cfg, boot.deploy, boot.log
	config.WarnUndeclared(logger, cfg.unknownVars)

	pool, err := database.NewPool(ctx, cfg.dsn)
	if err != nil {
		return err
	}
	// Registered before the lanes' join below, so LIFO closes the pool after it.
	defer pool.Close()
	// Before any lane runs: a pool connecting as a role row-level security
	// does not bind serves every tenant's rows to every job, and nothing later
	// in this boot would say so.
	if err := compose.AssertRuntimeRole(ctx, pool); err != nil {
		return err
	}

	// Before this role does ANY work: a worker from a different release than the
	// one this installation records is half of a torn tag pull, and it stops
	// rather than run the outbox relay, the retention evaluator and the agent
	// runner against a schema and a contract that are not its own
	// (compose/releaseversion.go carries why). Ahead of the composition record
	// below, because a role that must not run must not write either.
	if err := compose.AssertInstallationRelease(ctx, pool, logger, buildinfo.ReleaseVersion); err != nil {
		return err
	}
	// And again on a tick, because the boot check answers once.
	ctx, stopForReleaseSkew, releaseSkewErr := watchReleaseSkew(ctx, pool, logger)
	defer stopForReleaseSkew()

	// Before this role does any work, so an operator mistake never leaves a
	// worker running on a license the api refuses to boot on. The RUNNING
	// posture is the api's to watch: it is the role that serves it, and two
	// roles re-resolving one calendar answer would report the same lapse twice.
	//
	// After the pool rather than before it, which it used to be: an installation
	// that has sealed its token holds it in the key vault, and the vault is a
	// table. That makes this a WRITE — the first boot to resolve a declared
	// token seals it and stamps an audit row — so it also has to sit after the
	// release assertion above, which exists to stop a role that must not run
	// from writing. AssertRuntimeRole comes earlier still, so a pool connecting
	// as the wrong role fails as that rather than as a license nobody can read.
	// THE key vault for this process, resolved once and handed to every lane
	// that needs one. Three lanes used to resolve their own and each read the
	// answer in its own way; one value cannot drift from itself.
	//
	// Here because this is the first thing that needs it, and no earlier: the
	// question it may have to answer — does this installation hold sealed
	// secrets it can no longer open — is asked of a table, so it belongs after
	// the pool and after the release assertion, for the same reasons the license
	// does. A nil vault IS an unconfigured deployment; keyvault.ForRole carries
	// why nothing downstream is handed a flag beside it.
	vault, err := keyvault.ForRole(ctx, "worker", pool, config.FromOS)
	if err != nil {
		return err
	}
	if err := admitBoot(ctx, logger, pool, vault, cfg, deployCfg); err != nil {
		return err
	}

	// What this binary composed (ADR-0069 §5); pre-bootstrap the inventory half
	// skips — the api records the first observation once it has bootstrapped
	// the installation. The worker composes the same units the api does and
	// runs the send path, so it registers the same channel vocabulary and
	// answers "can a reply leave this installation" from the same set; the
	// write is an idempotent upsert, so whichever role boots second changes
	// nothing.
	if err := compose.RecordComposition(ctx, pool, logger, boot.extensions); err != nil {
		return err
	}

	rdb, err := events.NewClient(ctx, cfg.redisAddr)
	if err != nil {
		return err
	}
	// Same ordering obligation as pool.Close above: the lanes read this client.
	defer closeBus(rdb, logger)

	// Before the lanes and the runner: a listener started after them reports
	// nothing during exactly the window a slow boot needs explaining. /readyz
	// answers "still starting" until started.complete below, so coming up
	// early never makes this replica look ready before it can work.
	started := &bootGate{}
	observe, err := startObserveListener(ctx, cfg, pool, rdb, started, logger)
	if err != nil {
		return err
	}
	defer observe.Stop()

	//nolint:contextcheck // boot-time wiring: the model path outlives any request context (cmd/api resolves the same path under the same waiver)
	modelPath, boundModels, err := selectModelPath(ctx, workerModelPathSpec(cfg, deployCfg), pool, logger)
	if err != nil {
		return err
	}

	// The api serves the write, this role only ever reads — so without this it
	// would keep serving whatever binding it resolved at boot while the api
	// served the new one, which is the two-roles-disagree failure moving
	// routing into the database was meant to end (compose/routingwatcher).
	go compose.NewRoutingWatcher(pool, &modelPath, config.FromOS, logger).Run(ctx)

	// Deferred BEFORE the error is checked: a failure here still leaves earlier
	// lanes running on the bus and the pool whose closes are deferred above, and
	// LIFO is what puts this join ahead of them.
	lanes, err := startEventLanes(ctx, cfg, pool, rdb, vault, modelPath, logger, stdout)
	defer lanes.join()
	if err != nil {
		return err
	}

	// The operator relay, resolved in THIS role. Unattended work runs here, so
	// the weekly retrospective's mail has no request to arrive on.
	weeklyMail := weeklyMailConfig(ctx, cfg, deployCfg, pool, vault, logger)
	_, _ = fmt.Fprintln(stdout, weeklyMailBanner(weeklyMail))

	stopJobs, err := startJobRunner(ctx, pool, rdb, vault, compose.OverlayBudgetConfig(deployCfg.EffectiveOverlayBudget()),
		logger, cfg, modelPath, boundModels, lanes, weeklyMail, stdout)
	if err != nil {
		return err
	}
	defer stopJobs()
	// AFTER the job runner, because that is where this role's unconditional
	// BindExtensionRuntime happens and a delivery needs it: a unit's handler
	// reaches the installation through the per-call Runtime, which refuses
	// while nothing is bound. Started earlier, a retained entry redelivered in
	// that window would fail on the wiring and then wait out the subscriber's
	// whole reclaim interval before anyone tried again — on a worker with no
	// model configured, which never reaches the runner lane's own binding.
	// The context is the LANES', which is this function's own ctx with their
	// cancel around it (startEventLanes) — carried on the value rather than
	// re-derived here, so this lane ends when its siblings do instead of
	// outliving them under a second shutdown.
	startExtensionSubscriptionLanes(lanes.ctx, pool, rdb, lanes.background, logger, stdout) //nolint:contextcheck // the lanes' ctx IS derived from this one; see above.
	// Every phase a replica needs to do work has returned; /readyz may say so.
	started.complete()
	// Deferred AFTER complete, so LIFO runs it FIRST: readiness goes false at
	// the top of the shutdown, before the runner and the lanes are put down.
	// The listener outlives both — it is stopped last — so a draining replica
	// keeps answering, and what it answers is "stop sending me work".
	defer started.draining()

	relayUntilSignal(ctx, cfg, pool, rdb, logger, stdout)
	return releaseSkewRefusal(releaseSkewErr)
}

// releaseSkewRefusal is why the relay returned: a signal, or the release guard.
//
// The relay wakes on either, and only one of them is a fault. A signal leaves
// the channel empty and the role exits zero; a confirmed release change answers
// the refusal, so the process exits non-zero into the crash loop the boot guard
// already produces.
func releaseSkewRefusal(refused <-chan error) error {
	select {
	case err := <-refused:
		return err
	default:
		return nil
	}
}

// registerComposedExtensions registers the composed extension set before
// anything else runs; a failing registration aborts the boot (ADR-0069 EXT-P4).
//
// It returns the SAME snapshot it registered, because run hands that value on
// to the boot inventory: taking a second snapshot there would let the two
// observe different declarations, and the inventory's whole job is to record
// what this process is actually running.
func registerComposedExtensions() ([]extension.Extension, error) {
	extensions := composition.Extensions()
	if err := compose.RegisterExtensions(extensions, composition.Verbs(), composition.Jobs()); err != nil {
		return nil, err
	}
	return extensions, nil
}

// workerBoot is everything decided before this process touches a network: the
// parsed flags, the deployment file, the composed extension set, and the
// logger the phases after it report through.
type workerBoot struct {
	cfg        workerConfig
	deploy     deployconfig.Config
	extensions []extension.Extension
	log        *slog.Logger
}

// configureWorker runs the phases that can fail on configuration alone, in the
// order they depend on each other: flags, then extensions (a failing
// registration aborts the boot before anything is opened), then the deployment
// file, then the logger — and the capture posture last, because it carries the
// logger into the Sink's post-commit steps, where a fault is reported rather
// than returned.
//
// Grouped because they share one property the phases after them do not: none
// of them opens a connection, so a deployment misconfigured in any of these
// ways fails before a pool, a bus client or a listener exists to clean up.
func configureWorker(args []string, stdout io.Writer) (workerBoot, error) {
	cfg, err := parseWorkerFlags(args)
	if err != nil {
		return workerBoot{}, err
	}
	extensions, err := registerComposedExtensions()
	if err != nil {
		return workerBoot{}, err
	}
	deployCfg, err := loadDeployment(&cfg)
	if err != nil {
		return workerBoot{}, err
	}
	log, err := newWorkerLogger(cfg, stdout)
	if err != nil {
		return workerBoot{}, err
	}
	cfg.captureConfig = compose.CaptureConfigFromDeploy(deployCfg.Capture, log)
	return workerBoot{cfg: cfg, deploy: deployCfg, extensions: extensions, log: log}, nil
}

// newWorkerLogger builds this role's correlation-aware logger from the
// operator's --log-level and --log-format, and installs it as the process
// default. Every process role shares the one level/format vocabulary, and a
// typo in either is a boot error rather than a silent fallback to a level
// nobody asked for.
//
// THE DEFAULT MATTERS MOST IN THIS ROLE. Every job runs here, and a job's
// failure reaches an operator through jobs.faultFor, which logs through the
// package-level slog functions — so before this role installed its handler,
// the one line a postponed tick leaves anywhere went to the stdlib default
// while everything else went to the operator's configured sink.
func newWorkerLogger(cfg workerConfig, stdout io.Writer) (*slog.Logger, error) {
	return httpserver.InstallProcessLogger(stdout, cfg.logLevel, cfg.logFormat)
}

// runResumeSubscriber consumes cg:overnight-agent: approval decisions
// wake parked runs.
//
// Its reclaim window has to clear the whole run, not the default: this
// handler resumes a multi-step agent loop that may take the full
// RunWallClock, and reclaiming a merely-slow consumer would hand the same
// decision to a peer replica while the first is still running it. The run
// row's own claim already refuses the second resume; keeping the window
// above the handler's honest runtime means the bus stops producing the
// duplicate in the first place.
func runResumeSubscriber(ctx context.Context, rdb *redis.Client, svc *compose.RunnerService, log *slog.Logger) {
	runSubscriber(ctx, rdb, "cg:overnight-agent", svc.HandleEvent, log, compose.RunWallClock+time.Minute)
}

// runSubscriber consumes one events.md consumer group, Dedupe-wrapped
// because the bus is at-least-once (events.md §3). minIdle overrides the
// reclaim window for a group whose handler runs longer than the default;
// zero keeps it.
func runSubscriber(ctx context.Context, rdb *redis.Client, groupName string, handler events.Handler, log *slog.Logger, minIdle time.Duration) {
	for _, g := range kevents.Groups() {
		if g.Name == groupName {
			runGroupSubscriber(ctx, rdb, g, handler, log, minIdle)
			return
		}
	}
	// A name no catalog group answers to is a typo in this role's wiring. Said
	// out loud rather than run: the zero Group subscribes to no streams, so the
	// lane would come up, log nothing, and deliver nothing forever.
	log.Error("worker: no such consumer group, so this lane delivers nothing", "group", groupName)
}

// runGroupSubscriber consumes one consumer group, whether the catalog declared
// it or a composed extension subscription did (compose.ComposedSubscription
// builds its own, over the streams its declared types route to). Everything
// after the group is identical for both, which is the point: a unit's listener
// is an ordinary consumer, not a second delivery mechanism.
func runGroupSubscriber(ctx context.Context, rdb *redis.Client, group kevents.Group, handler events.Handler, log *slog.Logger, minIdle time.Duration) {
	sub := events.NewSubscriber(rdb, group, events.Dedupe(rdb, group.Name, handler), log).WithMinIdle(minIdle)
	if err := sub.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("subscriber "+group.Name, "err", err)
	}
}

// ensureLicense resolves this worker's entitlement, reading the token out of
// the boot's key vault when the installation has sealed it there.
//
// A vault that is configured but malformed already failed the boot before this
// is reached, which is the posture every role takes: an installation whose root
// key no longer opens its own secrets has a problem that outlives the license
// question.
// admitBoot holds every precondition this worker must satisfy before it
// starts doing work: it is entitled to run, and anything it sends will
// carry links a recipient can open.
func admitBoot(
	ctx context.Context, logger *slog.Logger, pool *pgxpool.Pool,
	vault keyvault.Vault, cfg workerConfig, deployCfg deployconfig.Config,
) error {
	if err := ensureLicense(ctx, logger, pool, vault, deployCfg, cfg.posture); err != nil {
		return err
	}
	return requireUsablePublicOrigin(cfg, deployCfg)
}

// requireUsablePublicOrigin refuses to start a sending worker whose
// outgoing links a recipient could not open.
//
// The worker sends too — the scheduled and agent lanes run here — so it
// answers to the same rule as the api. A role that skipped it would send
// exactly the messages the api refuses.
//
// What this predicate CANNOT see, and it is worth naming rather than
// implying: an installation may store its Google app in the database
// through Settings instead of composing it from the environment
// (compose.newCaptureRegistryWithGoogle), and that app is not readable
// here — boot runs before, and independently of, any workspace read. Such
// a worker starts without this check. It does not then send a broken
// link: the send-time guard in activities refuses every tokenized message
// on the same rule. What is lost is the early failure, not the
// protection.
func requireUsablePublicOrigin(cfg workerConfig, deployCfg deployconfig.Config) error {
	if !deployCfg.Email.Enabled && !cfg.gmailAppWired() {
		return nil
	}
	if err := netguard.RequirePublicOrigin("--public-base-url", cfg.publicBaseURL, cfg.posture); err != nil {
		return fmt.Errorf("worker: %w", err)
	}
	return nil
}

func ensureLicense(ctx context.Context, logger *slog.Logger, pool *pgxpool.Pool, vault keyvault.Vault, deployCfg deployconfig.Config, posture runtimeenv.Environment) error {
	_, err := compose.EnsureLicense(ctx, logger, pool, vault, deployCfg, posture, config.FromOS)
	return err
}

// watchReleaseSkew starts the periodic half of the release guard and hands run
// what it needs to act on it: a context the watcher can put the role down
// through, the cancel to release on the ordinary path, and the channel the exit
// reads.
//
// The boot check answers once. Roll only the api and this role keeps the relay,
// the retention evaluator and the agent runner pointed at a schema and an event
// contract that are not its own, with nothing logged and nothing crash-looping
// — the state the guard exists to prevent, reached by a partial deploy rather
// than a torn pull.
//
// The cancel is what turns the watcher's answer into an exit: the lanes and the
// relay all run on the context returned here, so cancelling it puts the role
// down the way a signal does, and run returns the refusal afterwards so the
// process exits NON-ZERO into the crash loop the boot guard already produces.
//
// The refusal travels by CHANNEL rather than by a variable the goroutine
// writes: the send happens before the cancel that lets the relay return, so
// run's read is ordered after it by the channel rather than by an argument
// about when the relay wakes.
//
// See compose/releasewatch.go for why a single differing read is not enough and
// why a read failure is not a difference.
func watchReleaseSkew(
	ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger,
) (context.Context, context.CancelFunc, <-chan error) {
	ctx, stop := context.WithCancel(ctx)
	skew := compose.WatchInstallationRelease(
		ctx, pool, logger, buildinfo.ReleaseVersion, compose.ReleaseRecheckInterval)
	refused := make(chan error, 1)
	go func() {
		err, stopping := <-skew
		if !stopping || err == nil {
			return
		}
		refused <- err
		logger.Error("release guard: stopping this role", "err", err)
		stop()
	}()
	return ctx, stop, refused
}
