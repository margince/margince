// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/margince/margince/backend/internal/compose"
	ps "github.com/margince/margince/backend/internal/compose/programsettings"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/database/dbsettings"
	"github.com/margince/margince/backend/internal/platform/deployconfig"
	"github.com/margince/margince/backend/internal/platform/httpserver"
	"github.com/margince/margince/backend/internal/shared/runtimeenv"
)

// apiConfig is the parsed boot configuration of the api process.
type apiConfig struct {
	dsn       string
	schemaDSN string
	addr      string
	redisAddr string
	// redisPassword is the bus credential, empty where the instance requires
	// none. The bus carries job payloads and therefore CRM data, so an
	// instance reachable by anything but this deployment has to require one.
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
	// It selects which license authorities are honoured, and decides nothing
	// destructive (shared/runtimeenv says why).
	posture        runtimeenv.Environment
	trustedProxies httpserver.TrustedProxies
	// settings are the loaded settings, for the reads compose makes itself.
	settings *config.Config
	// passwordLogin is auth.password.enabled: whether the email+password
	// method is offered and served.
	passwordLogin bool
	// mcpConnector is mcp.connector_enabled, the connector's deployment gate.
	mcpConnector bool
	// clientRegistration is mcp.client_registration: the ways an admin's
	// switches may turn on.
	clientRegistration identity.RegistrationMethods
}

// loadAPIConfig resolves the boot settings from args and environ and checks the
// ones that read two settings. Every fault this layer sees is reported in one
// error: one fault per boot made starting the binary by hand a guessing game.
// It hands compose the result before anything composes, so every stored
// setting a boot source sets is locked.
func loadAPIConfig(args, environ []string) (apiConfig, *config.Config, error) {
	reg, src, posture, err := apiSources(args, environ)
	if err != nil {
		return apiConfig{}, nil, err
	}
	loaded, err := config.Load(reg, src)
	if errors.Is(err, flag.ErrHelp) {
		config.PrintUsage(os.Stderr, reg)
	}
	if err != nil {
		return apiConfig{}, nil, bootError(err)
	}
	cfg, err := fromRegistry(loaded, posture)
	if err != nil {
		return apiConfig{}, nil, bootError(err)
	}
	compose.UseConfig(loaded)
	return cfg, loaded, nil
}

// apiSources are what the api loads, for a boot and a --check-config alike.
func apiSources(args, environ []string) (*config.Registry, config.Sources, runtimeenv.Environment, error) {
	reg, err := compose.ProgramRegistry(config.RoleAPI)
	if err != nil {
		return nil, config.Sources{}, "", err
	}
	posture := runtimeenv.Parse(config.EnvironValue(environ, runtimeenv.EnvVar))
	return reg, config.Sources{
		Args:           args,
		Environ:        environ,
		Known:          compose.AllSettingNames(),
		BootstrapRoots: deployconfig.Roots(),
		Faults:         func(c *config.Config) []error { return apiFaults(c, posture) },
	}, posture, nil
}

// checkAPIConfig runs --check-config when args ask for it: the boot's load,
// bootstrap roots included, and nothing that opens a connection.
func checkAPIConfig(args, environ []string, stdout io.Writer) (bool, error) {
	reg, src, _, err := apiSources(args, environ)
	if err != nil || !config.CheckRequested(reg, args) {
		return false, err
	}
	if err := config.Check(stdout, reg, src); err != nil {
		return true, bootError(err)
	}
	return true, nil
}

// fromRegistry copies the resolved settings into the struct the rest of
// cmd/api reads.
func fromRegistry(loaded *config.Config, posture runtimeenv.Environment) (apiConfig, error) {
	cfg := apiConfig{
		dsn: config.Val(loaded, dbsettings.DSN), schemaDSN: config.Val(loaded, ps.SchemaDSN),
		addr: config.Val(loaded, ps.Addr), redisAddr: config.Val(loaded, ps.Redis), redisPassword: config.Val(loaded, ps.RedisPassword),
		inlineRelay: config.Val(loaded, ps.InlineRelay), routingPath: config.Val(loaded, ps.AIRouting), fakeBrain: config.Val(loaded, ps.AIFake),
		logLevel: config.Val(loaded, ps.LogLevel), logFormat: config.Val(loaded, ps.LogFormat),
		publicBaseURL: config.Val(loaded, ps.PublicBaseURL), mcpAppsBaseURL: config.Val(loaded, ps.MCPAppsBaseURL), apiBaseURL: config.Val(loaded, ps.APIBaseURL),
		gmailClientID: config.Val(loaded, ps.GmailClientID), gmailClientSecret: config.Val(loaded, ps.GmailClientSecret),
		gmailPushToken: config.Val(loaded, ps.GmailPushToken), gmailPushAudience: config.Val(loaded, ps.GmailPushAudience),
		gmailPushSA: config.Val(loaded, ps.GmailPushServiceAccount), gmailJWKSURL: config.Val(loaded, ps.GmailJWKSURL),
		graphPushToken: config.Val(loaded, ps.GraphPushToken), graphClientID: config.Val(loaded, ps.GraphClientID),
		graphClientSecret: config.Val(loaded, ps.GraphClientSecret), graphTenant: config.Val(loaded, ps.GraphTenant),
		microsoftSignInTenant: config.Val(loaded, ps.MicrosoftSignInTenant), connectorStateKey: config.Val(loaded, ps.ConnectorStateKey),
		webhookKey: config.Val(loaded, ps.WebhookKey), metricsToken: config.Val(loaded, ps.MetricsToken), metricsAccess: config.Val(loaded, ps.MetricsAccess),
		vatCheckBaseURL: config.Val(loaded, ps.VATCheckBaseURL), geocodeBaseURL: config.Val(loaded, ps.GeocodeBaseURL),
		passwordLogin: config.Val(loaded, ps.PasswordLogin), settings: loaded,
		mcpConnector: config.Val(loaded, ps.MCPConnector),
		clientRegistration: identity.RegistrationMethods{
			Dynamic:          config.Val(loaded, ps.MCPDynamicRegistration),
			MetadataDocument: config.Val(loaded, ps.MCPMetadataDocument),
		},
		posture: posture,
	}
	trusted, err := trustedProxies(loaded)
	if err != nil {
		return apiConfig{}, err
	}
	cfg.trustedProxies = trusted
	return cfg, nil
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

// apiFaults holds the checks that read two settings, or a setting and the
// posture. Load runs them beside every other fault, so one boot names all.
func apiFaults(c *config.Config, posture runtimeenv.Environment) []error {
	var faults []error
	if err := dbsettings.Require(c); err != nil {
		faults = append(faults, err)
	}
	// An open endpoint given a token: the token would authenticate nothing,
	// while its operator believes the endpoint closed.
	if config.Val(c, ps.MetricsAccess) == ps.MetricsAccessOpen && config.Val(c, ps.MetricsToken) != "" {
		faults = append(faults, errors.New("--metrics-access=open serves /metrics to anyone, so the --metrics-token also set would "+
			"authenticate nothing: unset the token to open the endpoint, or drop --metrics-access=open to require it"))
	}
	if err := ps.FakeModelFault(c, posture); err != nil {
		faults = append(faults, err)
	}
	if _, err := trustedProxies(c); err != nil {
		faults = append(faults, err)
	}
	faults = append(faults, deployconfig.Faults(c.Files())...)
	return append(faults, compose.MailFaults(c)...)
}

// trustedProxies is checked among the load's faults, so --check-config sees it,
// and read again once the load holds.
func trustedProxies(c *config.Config) (httpserver.TrustedProxies, error) {
	trusted, err := httpserver.ParseTrustedProxies(config.Val(c, ps.TrustedProxies))
	if err != nil {
		return httpserver.TrustedProxies{}, fmt.Errorf("--trusted-proxies: %w", err)
	}
	return trusted, nil
}

// senderConfigured reports whether this deployment can put a message in
// somebody else's mailbox — by its own SMTP relay, or through a mailbox
// connector a rep has linked.
//
// It is the trigger for the public-origin rule: an installation that
// sends nothing may be configured however an operator likes, because no
// recipient ever sees a link built from it.
func senderConfigured(cfg apiConfig) bool {
	return config.Val(cfg.settings, compose.EmailEnabled) ||
		(cfg.gmailClientID != "" && cfg.gmailClientSecret != "") ||
		(cfg.graphClientID != "" && cfg.graphClientSecret != "")
}
