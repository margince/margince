// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/deployconfig"
	"github.com/margince/margince/backend/internal/platform/httpserver"
	"github.com/margince/margince/backend/internal/shared/runtimeenv"
)

// apiConfig is the parsed boot configuration of the api process.
type apiConfig struct {
	dsn        string
	configPath string
	schemaDSN  string
	addr       string
	redisAddr  string
	// redisPassword is the bus credential, empty where the instance requires
	// none. The bus carries job payloads and therefore CRM data, so an
	// instance reachable by anything but this deployment has to require one —
	// the desktop bundle's loopback bus does, and generates it per
	// installation.
	redisPassword         string
	inlineRelay           bool
	routingPath           string
	fakeBrain             bool
	logLevel              string
	logFormat             string
	publicBaseURL         string
	mcpAppsBaseURL        string
	apiBaseURL            string
	gmailClientID         string
	gmailClientSecret     string
	gmailPushToken        string
	gmailPushAudience     string
	gmailPushSA           string
	gmailJWKSURL          string
	graphPushToken        string
	graphClientID         string
	graphClientSecret     string
	graphTenant           string
	microsoftSignInTenant string
	connectorStateKey     string
	webhookKey            string
	metricsToken          string
	metricsAccess         string
	vatCheckBaseURL       string
	geocodeBaseURL        string
	// posture is what MARGINCE_ENV says this deployment is, read ONCE here
	// (OPS-CFG-2) rather than at each of the three places that used to ask.
	// It selects the configuration overlay and which license authorities are
	// honoured; it decides nothing destructive — see shared/runtimeenv.
	posture runtimeenv.Environment
	// unknownVars are the MARGINCE_* variables set that this role does not
	// read, reported once the logger exists so they reach the same stream.
	unknownVars    []string
	trustedProxies httpserver.TrustedProxies
}

// loadAPIConfig resolves the boot settings from args and environ and checks the
// ones that read two settings. Every fault this layer sees is reported in one
// error: one fault per boot made starting the binary by hand a guessing game.
func loadAPIConfig(args, environ []string) (apiConfig, *config.Config, error) {
	reg, err := config.NewRegistry(APISettings())
	if err != nil {
		return apiConfig{}, nil, err
	}
	legacy, err := config.NewItemRegistry(legacyAPIItems()...)
	if err != nil {
		return apiConfig{}, nil, err
	}
	legacyNames := map[string]bool{}
	for _, item := range legacy.Items() {
		legacyNames[item.Name] = true
	}
	loaded, err := config.Load(reg, config.Sources{
		Args:           args,
		Environ:        environ,
		KnownEnv:       func(name string) bool { return legacyNames[name] },
		BootstrapRoots: deployconfig.Sections(),
	})
	if errors.Is(err, flag.ErrHelp) {
		config.PrintUsage(os.Stderr, reg)
	}
	if err != nil {
		return apiConfig{}, nil, bootError(err)
	}
	cfg, err := fromRegistry(loaded, environ)
	if err != nil {
		return apiConfig{}, nil, bootError(err)
	}
	return cfg, loaded, nil
}

// fromRegistry copies the resolved settings into the struct the rest of
// cmd/api reads, then runs the checks that need two settings.
func fromRegistry(loaded *config.Config, environ []string) (apiConfig, error) {
	cfg := apiConfig{
		dsn: config.Val(loaded, apiDSN), configPath: loaded.Files().Base, schemaDSN: config.Val(loaded, apiSchemaDSN),
		addr: config.Val(loaded, apiAddr), redisAddr: config.Val(loaded, apiRedis), redisPassword: config.Val(loaded, apiRedisPassword),
		inlineRelay: config.Val(loaded, apiInlineRelay), routingPath: config.Val(loaded, apiAIRouting), fakeBrain: config.Val(loaded, apiAIFake),
		logLevel: config.Val(loaded, apiLogLevel), logFormat: config.Val(loaded, apiLogFormat),
		publicBaseURL: config.Val(loaded, apiPublicBaseURL), mcpAppsBaseURL: config.Val(loaded, apiMCPAppsBaseURL), apiBaseURL: config.Val(loaded, apiAPIBaseURL),
		gmailClientID: config.Val(loaded, apiGmailClientID), gmailClientSecret: config.Val(loaded, apiGmailClientSecret),
		gmailPushToken: config.Val(loaded, apiGmailPushToken), gmailPushAudience: config.Val(loaded, apiGmailPushAudience),
		gmailPushSA: config.Val(loaded, apiGmailPushSA), gmailJWKSURL: config.Val(loaded, apiGmailJWKSURL),
		graphPushToken: config.Val(loaded, apiGraphPushToken), graphClientID: config.Val(loaded, apiGraphClientID),
		graphClientSecret: config.Val(loaded, apiGraphClientSecret), graphTenant: config.Val(loaded, apiGraphTenant),
		microsoftSignInTenant: config.Val(loaded, apiMicrosoftSignInTenant), connectorStateKey: config.Val(loaded, apiConnectorStateKey),
		webhookKey: config.Val(loaded, apiWebhookKey), metricsToken: config.Val(loaded, apiMetricsToken), metricsAccess: config.Val(loaded, apiMetricsAccess),
		vatCheckBaseURL: config.Val(loaded, apiVATCheckBaseURL), geocodeBaseURL: config.Val(loaded, apiGeocodeBaseURL),
		posture:     runtimeenv.Parse(environValue(environ, runtimeenv.EnvVar)),
		unknownVars: loaded.Warnings(),
	}
	trusted, err := httpserver.ParseTrustedProxies(config.Val(loaded, apiTrustedProxies))
	if err != nil {
		return apiConfig{}, fmt.Errorf("--trusted-proxies: %w", err)
	}
	cfg.trustedProxies = trusted
	return cfg, errors.Join(apiFaults(cfg)...)
}

// environValue is the last NAME=value for name, as os.Getenv reads a repeat.
func environValue(environ []string, name string) string {
	var value string
	for _, entry := range environ {
		if k, v, ok := strings.Cut(entry, "="); ok && k == name {
			value = v
		}
	}
	return value
}

// bootError renders one fault as a sentence and several as a list.
func bootError(err error) error {
	faults := []error{err}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		faults = joined.Unwrap()
	}
	if len(faults) == 1 {
		return fmt.Errorf("api: %w", faults[0])
	}
	lines := make([]string, len(faults))
	for i, fault := range faults {
		lines[i] = fault.Error()
	}
	return errors.New("api: the configuration is incomplete:\n  - " + strings.Join(lines, "\n  - "))
}

// The two postures --metrics-access names.
const (
	metricsAccessToken = "token"
	metricsAccessOpen  = "open"
)

// apiFaults refuses an open /metrics that was also given a token. That token
// would authenticate nothing, while its operator believes the endpoint closed.
func apiFaults(cfg apiConfig) []error {
	if cfg.metricsAccess == metricsAccessOpen && cfg.metricsToken != "" {
		return []error{errors.New("--metrics-access=open serves /metrics to anyone, so the --metrics-token also set would " +
			"authenticate nothing: unset the token to open the endpoint, or drop --metrics-access=open to require it")}
	}
	return nil
}

// senderConfigured reports whether this deployment can put a message in
// somebody else's mailbox — by its own SMTP relay, or through a mailbox
// connector a rep has linked.
//
// It is the trigger for the public-origin rule: an installation that
// sends nothing may be configured however an operator likes, because no
// recipient ever sees a link built from it.
func senderConfigured(cfg apiConfig, deployCfg deployconfig.Config) bool {
	return deployCfg.Email.Enabled ||
		(cfg.gmailClientID != "" && cfg.gmailClientSecret != "") ||
		(cfg.graphClientID != "" && cfg.graphClientSecret != "")
}
