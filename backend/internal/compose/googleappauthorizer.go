// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gradionhq/margince/backend/internal/modules/capture"
	"github.com/gradionhq/margince/backend/internal/modules/capture/gcal"
	"github.com/gradionhq/margince/backend/internal/modules/capture/gmail"
	"github.com/gradionhq/margince/backend/internal/modules/capture/googleconn"
	"github.com/gradionhq/margince/backend/internal/modules/capture/oauthflow"
	"github.com/gradionhq/margince/backend/internal/platform/keyvault"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/principal"
	"github.com/gradionhq/margince/backend/internal/shared/ports/connector"
)

// googleAppResolver reports the installation's STORED Google app: the pair, and
// whether there was one to find. Named because three constructions and one
// Server field all have to spell the same shape, and an unnamed one drifts.
type googleAppResolver func(ctx context.Context) (clientID, clientSecret string, stored bool, err error)

// errNoGoogleApp is what a connector reports when neither source can supply the
// installation's Google app. It wraps the shared auth-rejected sentinel so the
// registry parks the connection instead of retrying a call that cannot start:
// no amount of backoff produces an app nobody has configured.
var errNoGoogleApp = fmt.Errorf(
	"capture: this installation has no Google OAuth app configured; "+
		"set the client id and secret under Settings, Capture: %w",
	connector.ErrAuthRejected,
)

// googleAppAuthorizer performs the token half of the Google handshake against
// the app the installation is using AT THE MOMENT OF THE CALL.
//
// A connector used to hold a client built at boot from the process environment,
// which is why an app set through Settings could not be connected: the registry
// registered the Google connectors only where the environment carried the pair,
// so a stored app reached a transport with no connector behind it and the
// surface answered its declared 501. Resolving per call is what lets the same
// connector serve either source, and lets a changed app take effect without a
// restart.
//
// Only Exchange and AccessToken live here. Building a consent URL cannot report
// an error, so it stays with the transport, which resolves the app itself and
// builds a concrete client from it.
type googleAppAuthorizer struct {
	// resolve reports the stored app; `stored` false means this installation has
	// none and the environment's app, where there is one, is what to use.
	resolve func(ctx context.Context) (clientID, clientSecret string, stored bool, err error)
	// env is the app the deployment composed, or nil where it composed none.
	env googleconn.Authorizer
	// build turns a resolved pair into a client for one provider — the one thing
	// that differs between mail and calendar, which own separate OAuth types and
	// request separate scopes.
	build func(clientID, clientSecret string) googleconn.Authorizer
}

// client resolves the app for this call. A resolution error is returned rather
// than falling back: a sealed secret that will not open means the vault's root
// key moved, and quietly connecting with an older environment copy would hide
// that behind a flow that half works.
//
//nolint:ireturn // returns the Authorizer seam by design — the caller holds an interface so tests substitute a stub
func (a googleAppAuthorizer) client(ctx context.Context) (googleconn.Authorizer, error) {
	if a.resolve != nil {
		id, secret, stored, err := a.resolve(ctx)
		if err != nil {
			return nil, err
		}
		if stored {
			return a.build(id, secret), nil
		}
	}
	if a.env != nil {
		return a.env, nil
	}
	return nil, errNoGoogleApp
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

// newGmailAuthorizer builds the mail connector's per-call authorizer over the
// given stored-app resolver, with the environment's app as the fallback.
func newGmailAuthorizer(resolve googleAppResolver, c GmailConfig) googleAppAuthorizer {
	var env googleconn.Authorizer
	if c.canSync() {
		env = newGmailOAuth(c)
	}
	return googleAppAuthorizer{
		resolve: resolve,
		env:     env,
		build: func(id, secret string) googleconn.Authorizer {
			return gmail.NewOAuth(gmail.OAuthConfig{ClientID: id, ClientSecret: secret, Scopes: gmailScopes})
		},
	}
}

// newGcalAuthorizer is the calendar half of the SAME app. It requests the
// calendar scope alone, which is why it builds its own client rather than
// sharing the mail one: a shared credential would accrete the other's grant.
func newGcalAuthorizer(resolve googleAppResolver, c GmailConfig) googleAppAuthorizer {
	var env googleconn.Authorizer
	if c.canSync() {
		env = newGcalOAuth(c)
	}
	return googleAppAuthorizer{
		resolve: resolve,
		env:     env,
		build: func(id, secret string) googleconn.Authorizer {
			return gcal.NewOAuth(gcal.OAuthConfig{ClientID: id, ClientSecret: secret})
		},
	}
}

// newGoogleAppResolver reads the installation's stored Google app.
//
// One constructor for both process roles. The api resolves it to run a person's
// consent flow and the worker to refresh a token on the sync poll, and those
// two asking the same question in two hand-written spellings is how they would
// come to disagree about which Google project this installation is.
//
// On a system principal: the caller is usually a rep connecting their own
// mailbox, who holds no capture grant, and on the worker there is no human
// caller at all. The installation's own configuration is not the rep's to be
// entitled to — it is what the server uses to do what they ARE entitled to.
// Same reasoning the model binding is resolved under, and it keeps the object
// gate rather than declaring the entry ungated.
func newGoogleAppResolver(pool *pgxpool.Pool, vault keyvault.Vault, log *slog.Logger) googleAppResolver {
	if pool == nil || vault == nil {
		return nil
	}
	store := capture.NewGoogleAppStore(NewSettingsStore(pool), vault, log)
	return googleAppCredentialsFrom(store)
}

// googleAppCredentialsFrom adapts the store to the resolver shape. Split from
// the constructor so a test can drive the workspace rule against a store it
// built itself.
func googleAppCredentialsFrom(store *capture.GoogleAppStore) googleAppResolver {
	return func(ctx context.Context) (string, string, bool, error) {
		ws, ok := principal.WorkspaceID(ctx)
		if !ok {
			// No workspace bound is a caller that has not authenticated; the
			// transport's own gate judges that, and answering "no app" here
			// would let it read as an unconfigured installation.
			return "", "", false, nil
		}
		return store.Credentials(bootCtx(ctx, ws, googleAppReadActor))
	}
}
