// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Resolving the installation's OAuth app at the moment it is used, for both
// vendors.
//
// A connector used to hold a client built at boot from the process environment,
// which is why an app set through Settings could not be connected: the registry
// registered a vendor's connectors only where the environment carried the pair,
// so a stored app reached a transport with no connector behind it and the
// surface answered its declared 501. Resolving per call is what lets the same
// connector serve either source, and lets a changed app take effect without a
// restart.
//
// Only the token half lives here. Building a consent URL cannot report an error
// — AuthCodeURL takes no context — so it stays with the transport, which
// resolves the app itself and builds a concrete client from it.

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/gcal"
	"github.com/margince/margince/backend/internal/modules/capture/gmail"
	"github.com/margince/margince/backend/internal/modules/capture/googleconn"
	"github.com/margince/margince/backend/internal/modules/capture/graph"
	"github.com/margince/margince/backend/internal/modules/capture/graphcal"
	"github.com/margince/margince/backend/internal/modules/capture/oauthflow"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// appResolver reports the installation's STORED app for one vendor: the app,
// and whether there was one to find. Named because several constructions and
// two Server fields have to spell the same shape, and an unnamed one drifts.
type appResolver func(ctx context.Context) (capture.ConnectorApp, bool, error)

// errNoGoogleApp and errNoMicrosoftApp are what a connector reports when neither
// source can supply the installation's app. Each wraps the shared auth-rejected
// sentinel so the registry parks the connection instead of retrying a call that
// cannot start: no amount of backoff produces an app nobody has configured.
var (
	errNoGoogleApp = fmt.Errorf(
		"capture: this installation has no Google OAuth app configured; "+
			"set the client id and secret under Settings, Capture: %w",
		connector.ErrAuthRejected,
	)
	errNoMicrosoftApp = fmt.Errorf(
		"capture: this installation has no Microsoft OAuth app configured; "+
			"set the Entra client id and secret under Settings, Capture: %w",
		connector.ErrAuthRejected,
	)
)

// resolveApp picks the client ONE call will use: the stored app where the
// installation has one, the deployment's otherwise.
//
// A resolution error is returned rather than falling back: a sealed secret that
// will not open means the vault's root key moved, and quietly connecting with an
// older environment copy would hide that behind a flow that half works.
//
// Generic over the vendor's own client type. The two vendors own separate OAuth
// interfaces and cannot share one, but this rule — stored wins, environment is
// the fallback, a fault is neither — is one rule, and a second copy of it is the
// copy that would keep falling back on the day the first stopped.
func resolveApp[T comparable](
	ctx context.Context, resolve appResolver, env T, build func(capture.ConnectorApp) T, absent error,
) (T, error) {
	var zero T
	if resolve != nil {
		app, stored, err := resolve(ctx)
		if err != nil {
			return zero, err
		}
		if stored {
			return build(app), nil
		}
	}
	if env != zero {
		return env, nil
	}
	return zero, absent
}

// googleAppAuthorizer performs the token half of the Google handshake against
// the app the installation is using AT THE MOMENT OF THE CALL.
type googleAppAuthorizer struct {
	// resolve reports the stored app; `stored` false means this installation has
	// none and the environment's app, where there is one, is what to use.
	resolve appResolver
	// env is the app the deployment composed, or nil where it composed none.
	env googleconn.Authorizer
	// build turns a resolved app into a client for one provider — the one thing
	// that differs between mail and calendar, which own separate OAuth types and
	// request separate scopes.
	build func(capture.ConnectorApp) googleconn.Authorizer
}

//nolint:ireturn // returns the Authorizer seam by design — the caller holds an interface so tests substitute a stub
func (a googleAppAuthorizer) client(ctx context.Context) (googleconn.Authorizer, error) {
	return resolveApp(ctx, a.resolve, a.env, a.build, errNoGoogleApp)
}

func (a googleAppAuthorizer) Exchange(ctx context.Context, code, redirectURI string) (oauthflow.TokenGrant, error) {
	c, err := a.client(ctx)
	if err != nil {
		return oauthflow.TokenGrant{}, err
	}
	return c.Exchange(ctx, code, redirectURI)
}

func (a googleAppAuthorizer) AccessToken(ctx context.Context, refreshToken string) (string, error) {
	c, err := a.client(ctx)
	if err != nil {
		return "", err
	}
	return c.AccessToken(ctx, refreshToken)
}

// graphAppAuthorizer is the Microsoft twin. It carries a third method the Google
// pair does not — Refresh — because Microsoft rotates the refresh token on every
// redemption and asks for the grant's own scopes, and that call has to reach the
// same resolved app as the other two.
type graphAppAuthorizer struct {
	resolve appResolver
	env     graph.Authorizer
	build   func(capture.ConnectorApp) graph.Authorizer
}

//nolint:ireturn // returns the Authorizer seam by design — the caller holds an interface so tests substitute a stub
func (a graphAppAuthorizer) client(ctx context.Context) (graph.Authorizer, error) {
	return resolveApp(ctx, a.resolve, a.env, a.build, errNoMicrosoftApp)
}

func (a graphAppAuthorizer) Exchange(ctx context.Context, code, redirectURI string) (oauthflow.TokenGrant, error) {
	c, err := a.client(ctx)
	if err != nil {
		return oauthflow.TokenGrant{}, err
	}
	return c.Exchange(ctx, code, redirectURI)
}

func (a graphAppAuthorizer) AccessToken(ctx context.Context, refreshToken string) (string, error) {
	c, err := a.client(ctx)
	if err != nil {
		return "", err
	}
	return c.AccessToken(ctx, refreshToken)
}

func (a graphAppAuthorizer) Refresh(
	ctx context.Context, refreshToken string, granted []string,
) (oauthflow.TokenRefresh, error) {
	c, err := a.client(ctx)
	if err != nil {
		return oauthflow.TokenRefresh{}, err
	}
	return c.Refresh(ctx, refreshToken, granted)
}

// newGmailAuthorizer builds the mail connector's per-call authorizer over the
// given stored-app resolver, with the environment's app as the fallback.
func newGmailAuthorizer(resolve appResolver, c GmailConfig) googleAppAuthorizer {
	var env googleconn.Authorizer
	if c.canSync() {
		env = newGmailOAuth(c)
	}
	return googleAppAuthorizer{
		resolve: resolve,
		env:     env,
		build: func(app capture.ConnectorApp) googleconn.Authorizer {
			return gmail.NewOAuth(gmail.OAuthConfig{
				ClientID: app.ClientID, ClientSecret: app.ClientSecretRef, Scopes: gmailScopes,
			})
		},
	}
}

// newGcalAuthorizer is the calendar half of the SAME app. It requests the
// calendar scope alone, which is why it builds its own client rather than
// sharing the mail one: a shared credential would accrete the other's grant.
func newGcalAuthorizer(resolve appResolver, c GmailConfig) googleAppAuthorizer {
	var env googleconn.Authorizer
	if c.canSync() {
		env = newGcalOAuth(c)
	}
	return googleAppAuthorizer{
		resolve: resolve,
		env:     env,
		build: func(app capture.ConnectorApp) googleconn.Authorizer {
			return gcal.NewOAuth(gcal.OAuthConfig{ClientID: app.ClientID, ClientSecret: app.ClientSecretRef})
		},
	}
}

// newGraphAuthorizer is the Outlook MAIL half of the installation's Entra app.
//
// The stored app carries its own tenant, which the environment's does not
// inherit: an operator who pinned their registration to one directory pinned the
// app, and reading the deployment's --graph-tenant over it would authorize a
// directory they deliberately excluded.
func newGraphAuthorizer(resolve appResolver, g GraphConfig) graphAppAuthorizer {
	var env graph.Authorizer
	if g.canSync() {
		env = newGraphOAuth(g)
	}
	return graphAppAuthorizer{
		resolve: resolve,
		env:     env,
		build: func(app capture.ConnectorApp) graph.Authorizer {
			return graph.NewOAuth(graph.OAuthConfig{
				ClientID: app.ClientID, ClientSecret: app.ClientSecretRef,
				Tenant: app.Tenant, Scopes: graphScopes,
			})
		},
	}
}

// newGraphCalAuthorizer is the Outlook CALENDAR half of the same registration —
// its own consent asking the calendar permission alone, for the same reason the
// Google pair splits.
func newGraphCalAuthorizer(resolve appResolver, g GraphConfig) graphAppAuthorizer {
	var env graph.Authorizer
	if g.canSync() {
		env = newGraphCalOAuth(g)
	}
	return graphAppAuthorizer{
		resolve: resolve,
		env:     env,
		build: func(app capture.ConnectorApp) graph.Authorizer {
			return graphcal.NewOAuth(graphcal.OAuthConfig{
				ClientID: app.ClientID, ClientSecret: app.ClientSecretRef, Tenant: app.Tenant,
			})
		},
	}
}

// newConnectorAppResolvers reads the installation's stored apps, one resolver
// per vendor.
//
// One constructor for both process roles. The api resolves an app to run a
// person's consent flow and the worker to refresh a token on the sync poll, and
// those two asking the same question in two hand-written spellings is how they
// would come to disagree about which project this installation is.
func newConnectorAppResolvers(
	pool *pgxpool.Pool, vault keyvault.Vault, log *slog.Logger,
) (google, microsoft appResolver) {
	if pool == nil || vault == nil {
		return nil, nil
	}
	store := capture.NewConnectorAppStore(NewSettingsStore(pool), vault, log)
	return appCredentialsFrom(store, capture.AppProviderGoogle),
		appCredentialsFrom(store, capture.AppProviderMicrosoft)
}

// appCredentialsFrom adapts the store to the resolver shape. Split from the
// constructor so a test can drive the workspace rule against a store it built
// itself.
//
// On a system principal: the caller is usually a rep connecting their own
// mailbox, who holds no capture grant, and on the worker there is no human
// caller at all. The installation's own configuration is not the rep's to be
// entitled to — it is what the server uses to do what they ARE entitled to.
// Same reasoning the model binding is resolved under, and it keeps the object
// gate rather than declaring the entry ungated.
func appCredentialsFrom(store *capture.ConnectorAppStore, p capture.AppProvider) appResolver {
	return func(ctx context.Context) (capture.ConnectorApp, bool, error) {
		ws, ok := principal.WorkspaceID(ctx)
		if !ok {
			// No workspace bound is a caller that has not authenticated; the
			// transport's own gate judges that, and answering "no app" here
			// would let it read as an unconfigured installation.
			return capture.ConnectorApp{}, false, nil
		}
		return store.Credentials(bootCtx(ctx, ws, connectorAppReadActor), p)
	}
}
