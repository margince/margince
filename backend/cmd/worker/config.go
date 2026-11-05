// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Boot configuration for the worker process role: the resolved settings and
// the checks that read two of them. Kept out of main.go so that file stays the
// process lifecycle (wire, run, drain) rather than the config vocabulary.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	ps "github.com/margince/margince/backend/internal/compose/programsettings"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/database/dbsettings"
	"github.com/margince/margince/backend/internal/platform/deployconfig"
	"github.com/margince/margince/backend/internal/platform/httpserver"
	"github.com/margince/margince/backend/internal/shared/runtimeenv"
)

// workerConfig is the parsed boot configuration of the worker process.
type workerConfig struct {
	dataTasksPace time.Duration
	dsn           string
	publicBaseURL string
	listsEnabled  bool
	captureConfig compose.CaptureConfig
	// allowDataReset is operations.allow_data_reset: whether this installation
	// armed the destructive reset at all. The worker's only stake is the cache
	// flush it subscribes to, which exists solely to serve that reset — so an
	// installation that never armed it holds no subscriber either.
	allowDataReset  bool
	ratesFx         string
	ratesCurrencies []string
	// csvImportCap is uploads.csv_import_mb in bytes: an import commit reads
	// its stored upload under the limits the api accepted it with.
	csvImportCap int64
	redisAddr    string
	// redisPassword is the bus credential, empty where the instance requires
	// none. The bus carries job payloads and therefore CRM data, so an
	// instance reachable by anything but this deployment has to require one.
	redisPassword     string
	routingPath       string
	fakeBrain         bool
	gmailClientID     string
	gmailClientSecret string
	graphClientID     string
	graphClientSecret string
	graphTenant       string
	gmailPubsubTopic  string
	graphNotifyURL    string
	webhookKey        string
	geocodeBaseURL    string
	vatCheckBaseURL   string
	certLogBaseURL    string
	// jobDrainWindow is how long a job already running at shutdown keeps its
	// work context; see programsettings.DefaultJobDrainWindow for the budget it is part of.
	jobDrainWindow time.Duration
	logLevel       string
	logFormat      string
	observeAddr    string
	observePprof   bool
	// posture is what MARGINCE_ENV says this deployment is, read ONCE here
	// (OPS-CFG-2). It selects which license authorities are honoured, and
	// decides nothing destructive (shared/runtimeenv says why).
	posture runtimeenv.Environment
	// settings are the loaded settings, for the reads compose makes itself.
	settings *config.Config
}

// loadWorkerConfig resolves the boot settings from args and environ, and
// reports every fault of the configuration in one error. Like the api's, it
// hands compose the result before anything composes.
func loadWorkerConfig(args, environ []string) (workerConfig, *config.Config, error) {
	reg, src, posture, err := workerSources(args, environ)
	if err != nil {
		return workerConfig{}, nil, err
	}
	loaded, err := config.Load(reg, src)
	if errors.Is(err, flag.ErrHelp) {
		config.PrintUsage(os.Stderr, reg)
	}
	if err != nil {
		return workerConfig{}, nil, fmt.Errorf("worker: %w", err)
	}
	compose.UseConfig(loaded)
	return fromRegistry(loaded, posture), loaded, nil
}

// workerSources are what the worker loads, for a boot and a --check-config
// alike.
func workerSources(args, environ []string) (*config.Registry, config.Sources, runtimeenv.Environment, error) {
	reg, err := compose.ProgramRegistry(config.RoleWorker)
	if err != nil {
		return nil, config.Sources{}, "", err
	}
	posture := runtimeenv.Parse(config.EnvironValue(environ, runtimeenv.EnvVar))
	return reg, config.Sources{
		Args:           args,
		Environ:        environ,
		Known:          compose.AllSettingNames(),
		BootstrapRoots: deployconfig.Roots(),
		Faults:         func(c *config.Config) []error { return workerFaults(c, posture) },
	}, posture, nil
}

// checkWorkerConfig runs --check-config when args ask for it: the boot's
// load, bootstrap roots included, and nothing that opens a connection.
func checkWorkerConfig(args, environ []string, stdout io.Writer) (bool, error) {
	reg, src, _, err := workerSources(args, environ)
	if err != nil || !config.CheckRequested(reg, args) {
		return false, err
	}
	if err := config.Check(stdout, reg, src); err != nil {
		return true, fmt.Errorf("worker: %w", err)
	}
	return true, nil
}

// fromRegistry copies the resolved settings into the struct the rest of
// cmd/worker reads.
func fromRegistry(loaded *config.Config, posture runtimeenv.Environment) workerConfig {
	return workerConfig{
		dsn:           config.Val(loaded, dbsettings.DSN),
		publicBaseURL: config.Val(loaded, ps.PublicBaseURL),
		redisAddr:     config.Val(loaded, ps.Redis), redisPassword: config.Val(loaded, ps.RedisPassword),
		routingPath: config.Val(loaded, ps.AIRouting), fakeBrain: config.Val(loaded, ps.AIFake),
		gmailClientID: config.Val(loaded, ps.GmailClientID), gmailClientSecret: config.Val(loaded, ps.GmailClientSecret),
		graphClientID: config.Val(loaded, ps.GraphClientID), graphClientSecret: config.Val(loaded, ps.GraphClientSecret),
		graphTenant: config.Val(loaded, ps.GraphTenant), gmailPubsubTopic: config.Val(loaded, ps.GmailPubsubTopic),
		graphNotifyURL: config.Val(loaded, ps.GraphNotificationURL), webhookKey: config.Val(loaded, ps.WebhookKey),
		geocodeBaseURL: config.Val(loaded, ps.GeocodeBaseURL), vatCheckBaseURL: config.Val(loaded, ps.VATCheckBaseURL),
		certLogBaseURL: config.Val(loaded, ps.CertLogBaseURL), jobDrainWindow: config.Val(loaded, ps.JobDrainWindow), dataTasksPace: config.Val(loaded, ps.DataTasksPace),
		logLevel: config.Val(loaded, ps.LogLevel), logFormat: config.Val(loaded, ps.LogFormat),
		observeAddr: config.Val(loaded, ps.ObserveAddr), observePprof: config.Val(loaded, ps.ObservePprof),
		listsEnabled:    config.Val(loaded, ps.ListsEnabled),
		csvImportCap:    compose.UploadLimitsFrom(loaded).CSVImport,
		allowDataReset:  config.Val(loaded, ps.AllowDataReset),
		ratesFx:         config.Val(loaded, ps.RatesFxSource),
		ratesCurrencies: config.Val(loaded, ps.RatesFxCurrencies),
		posture:         posture, settings: loaded,
	}
}

// workerFaults holds the checks that read two settings, or a setting and the
// posture. Load joins them to its own faults.
func workerFaults(c *config.Config, posture runtimeenv.Environment) []error {
	var faults []error
	for _, err := range []error{dbsettings.Require(c), ps.ObserveFault(c), ps.FakeModelFault(c, posture)} {
		if err != nil {
			faults = append(faults, err)
		}
	}
	faults = append(faults, deployconfig.Faults(c.Files())...)
	return append(faults, compose.MailFaults(c)...)
}

// defaultFxBootstrapCurrencies is the candidate set the FX refresh proposes on
// an empty sheet when the operator configured none — the three foreign
// currencies the base-EUR UI and the demo seed already use. Overridable via
// rates.fx_currencies; the FX refresh cannot bootstrap an empty sheet without a
// set, so an unset config takes this default rather than leaving the admin
// button dead on a fresh install.
var defaultFxBootstrapCurrencies = []string{"USD", "GBP", "CHF"}

// fxBootstrapCurrencies takes the operator's configured candidate set, or the
// default when none is configured.
func fxBootstrapCurrencies(configured []string) []string {
	if len(configured) == 0 {
		// Copy, never hand out the package-level slice — a consumer that
		// mutated it would rewrite the default process-wide.
		return append([]string(nil), defaultFxBootstrapCurrencies...)
	}
	return configured
}

// sendPath is the worker's outbound-send configuration. The Surface-B agent
// runner is this role's one sending lane — its governed tool surface carries
// send_email — and it stages through the same delivery machinery the api does:
// a lane that could accept a send but never queue one is a silent hole, and
// this role is the one that transmits.
//
// The deterministic automation lanes take none of it. A send_email action
// stages an approval rather than transmitting, and the automation module's
// Comms seam declares DraftEmail alone, so there is no send to configure for
// the workflow engine or the clock-trigger scanner.
//
// No mailbox pre-flight: that check is advisory and needs the connect
// registry, which only the api role builds. The transmit-time authority gate
// still refuses a grant that cannot send, so its absence costs a clearer
// message, never a message that should not have gone.
func sendPath(cfg workerConfig, delivery compose.DeliveryMachinery) compose.SendPath {
	return compose.SendPath{PublicBaseURL: cfg.publicBaseURL, Delivery: delivery, Environment: cfg.posture}
}

// gmailAppWired reports whether the deployment configured the Google OAuth
// app: without both halves the gmail connector is never registered, so the
// poll, the push-watch job, and the send lane are all absent by omission.
func (c workerConfig) gmailAppWired() bool {
	return c.gmailClientID != "" && c.gmailClientSecret != ""
}

// newWorkerLogger installs the operator's logger as the process default. A
// job's failure is logged by jobs.faultFor through the package-level slog
// functions, so without the default it misses the configured sink.
func newWorkerLogger(cfg workerConfig, stdout io.Writer) (*slog.Logger, error) {
	return httpserver.InstallProcessLogger(stdout, cfg.logLevel, cfg.logFormat)
}
