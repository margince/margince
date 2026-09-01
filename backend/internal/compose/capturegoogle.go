// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The Google half of the capture wiring: the deployment's Gmail OAuth app, the
// two OAuth clients it mints (mail and calendar), and the registries that carry
// them. Split from capture.go, which owns the Sink and the provider-agnostic
// registry — one concept per file, and the Google app is the one piece of that
// wiring an operator supplies rather than the product deciding.

import (
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/gcal"
	"github.com/margince/margince/backend/internal/modules/capture/gmail"
	"github.com/margince/margince/backend/internal/modules/capture/graph"
	"github.com/margince/margince/backend/internal/modules/capture/graphcal"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/keyvault"
)

// GmailConfig is the composed Gmail OAuth app for a deployment (RC-8): one app
// per deployment, supplied by whoever operates it (EP05.8 — per-workspace apps
// are a follow-up). ClientID+ClientSecret enable the background sync (token
// refresh); StateKey+PublicBaseURL additionally enable the connect/callback
// transport (the signed state and the redirect target).
type GmailConfig struct {
	ClientID     string
	ClientSecret string
	StateKey     string
	// PublicBaseURL is the canonical public/front origin (the SPA): the
	// post-consent landing, and the default callback base for a same-origin
	// deployment.
	PublicBaseURL string
	// APIBaseURL is the api's externally-reachable base, used only for the
	// callback redirect_uri. Empty for a same-origin deployment (the callback
	// rides PublicBaseURL); a split dev stack sets it to the api URL.
	APIBaseURL string
}

// canSync reports whether the connector can be registered + polled (token
// refresh needs the client id/secret).
func (c GmailConfig) canSync() bool { return c.ClientID != "" && c.ClientSecret != "" }

// minStateKeyLen is the floor for the OAuth state-signing HMAC key; a shorter
// key would make the signed state cheaply forgeable.
const minStateKeyLen = 32

// canConnect reports whether the human-facing connect/callback transport can
// run: it needs the sync creds plus the deployment prerequisites below.
func (c GmailConfig) canConnect() bool {
	return c.canSync() && c.canSignState()
}

// canSignState reports the prerequisites that are the DEPLOYMENT's rather than
// the Google app's: a callback URL, and a state key of at least minStateKeyLen
// bytes (a weak key is refused, not silently accepted).
//
// Split out because the app's credentials may now arrive at RUNTIME, from the
// stored setting, while these two cannot — nothing sets a signing key through
// the UI. So the transport mounts on these alone and asks for the app when a
// request needs one. Gating the mount on the credentials as well is what would
// leave a stored-app installation with an unbuilt signer: `oauthApp` would
// answer with the app, the 501 gate would pass, and the flow would HMAC its
// state with an empty key — silently bypassing the floor this very function
// exists to enforce.
func (c GmailConfig) canSignState() bool {
	return len(c.StateKey) >= minStateKeyLen && c.PublicBaseURL != ""
}

// Enabled reports whether the connect/callback transport is fully configured —
// the same condition WithGmailCapture gates on, exported so a caller (cmd) can
// log accurately rather than guessing from the client id alone.
func (c GmailConfig) Enabled() bool { return c.canConnect() }

//nolint:ireturn // returns the gmail.OAuth seam by design (a fakeable interface)
func newGmailOAuth(c GmailConfig) gmail.OAuth {
	return gmail.NewOAuth(gmail.OAuthConfig{
		ClientID:     c.ClientID,
		ClientSecret: c.ClientSecret,
		Scopes:       gmailScopes,
	})
}

// newGcalOAuth builds the calendar connector's OAuth client. It shares the same
// Google app credentials as Gmail (one app per deployment) but authorizes
// SEPARATELY, requesting the calendar scope alone — the gcal package owns that
// scope and its own error sentinels, so calendar diagnostics never surface as
// "gmail:" and the credential never accretes Gmail's mail-read grant.
//
//nolint:ireturn // returns the gcal.OAuth seam by design (a fakeable interface)
func newGcalOAuth(c GmailConfig) gcal.OAuth {
	return gcal.NewOAuth(gcal.OAuthConfig{
		ClientID:     c.ClientID,
		ClientSecret: c.ClientSecret,
	})
}

// newCaptureRegistryWithGoogle registers the Google connectors where the app
// can be resolved from EITHER source: the stored setting this installation
// wrote, or the pair the deployment composed.
//
// The registration used to require the environment's pair, which made the
// stored app unusable rather than merely unread: the transport asks the
// registry whether a connector exists before it will run the consent flow, so
// an installation that set its app through Settings was sent to the declared
// 501 and had no way to connect Gmail at all. A resolver is enough to register
// on, because the connector resolves the app when it uses it.
func newCaptureRegistryWithGoogle(
	pool *pgxpool.Pool,
	vault keyvault.Vault,
	resolve googleAppResolver,
	c GmailConfig,
	cfg CaptureConfig,
) *capture.Registry {
	reg := NewCaptureRegistry(pool, vault, cfg)
	if googleAppReachable(resolve, c) {
		reg.Register(gmail.New(newGmailAuthorizer(resolve, c), gmail.NewAPI(nil, "")).
			WithBounceSink(newBounceSink(pool)))
		reg.Register(gcal.New(newGcalAuthorizer(resolve, c), gcal.NewAPI(nil, "")))
	}
	return reg
}

// googleAppReachable reports whether anything in this composition could supply
// the Google app. Registering on neither would leave a connector that fails
// every call with "no app configured", which is a worse answer than the
// declared 501: it looks configured and is not.
func googleAppReachable(resolve googleAppResolver, c GmailConfig) bool {
	return resolve != nil || c.canSync()
}

// CaptureSyncRegistry is the worker's sweep registry: always non-nil —
// the standing IMAP connector needs no deployment config — with the Google and
// Microsoft connectors added when their OAuth apps are configured. Each vendor
// contributes BOTH its mail and its calendar connector, because one app serves
// both and a calendar that connects but never syncs reads as an empty calendar
// rather than as a broken one. A provider nobody registered simply never
// appears in the dispatcher's provider list.
func CaptureSyncRegistry(pool *pgxpool.Pool, vault keyvault.Vault, c GmailConfig, g GraphConfig, cfg CaptureConfig, log *slog.Logger) *capture.Registry {
	// The worker resolves the STORED app exactly as the api does. Without this
	// a mailbox connected against a stored app would connect and then never
	// sync: the poll would find no Google connector registered and skip it
	// silently, which reads as an empty inbox rather than as a broken one.
	reg := newCaptureRegistryWithGoogle(pool, vault, newGoogleAppResolver(pool, vault, log), c, cfg)
	if g.canSync() {
		reg.Register(graph.New(newGraphOAuth(g), graph.NewAPI(nil, "")).
			WithBounceSink(newBounceSink(pool)))
		reg.Register(graphcal.New(newGraphCalOAuth(g), graphcal.NewAPI(nil, "")))
	}
	return reg
}

// WithGmailCapture wires the Gmail OAuth connect/callback/disconnect/list
// transport (api role). It requires the vault (so WithKeyvault must precede it
// in the option list) and a fully-configured app; absent any of those the
// connector surface keeps its declared-but-unimplemented 501 by omission.
//
// It ALSO re-installs the outbound send pre-flight (WithSendAuthority) over the
// richer registry it builds here, upgrading the MAILBOX half of that check: a
// user whose mailbox holds no send scope is refused at request time rather than
// at transmission, where only an operator sees it. The CHANNEL half — a reply on
// a channel this workspace bound no bot for — does not depend on this option;
// WithKeyvault installs it unconditionally (comment below).
// googleBackedConnectors are the connect flows one Google OAuth app serves, in
// the order an operator registers them. Each is a SEPARATE route under its own
// provider key, so registering one does not cover the other.
var googleBackedConnectors = []struct {
	purpose  crmcontracts.GoogleAppRedirectUriPurpose
	provider string
}{
	{crmcontracts.MailboxConnect, providerGmail},
	{crmcontracts.CalendarConnect, providerGcal},
}

// WithGmailCapture mounts the Gmail and Google Calendar connect flow on the
// server: the OAuth clients, the signed state, and the registry the background
// sync shares with them.
//
// It records whether the deployment's Google app is configured at all before it
// gates anything, because the send pre-flight and the setup surface both answer
// from that one fact rather than each deciding for themselves.
func WithGmailCapture(c GmailConfig, cfg CaptureConfig) Option {
	return func(s *Server, pool *pgxpool.Pool) {
		s.gmailAppConfigured = c.canSync() // the send pre-flight's fact, recorded before the gate below
		// The setup surface reads the same fact. Stamped HERE and only here:
		// WithGmailCapture requires the vault and so always runs after
		// WithKeyvault, which means a copy taken there would always read false.
		s.envGoogleApp = s.gmailAppConfigured
		// The Google-app screen reads the same two-source resolution the connector
		// performs, so it needs the environment's client id and not merely the
		// fact that there is one: an operator checking their console against what
		// this installation actually uses needs the value, and it is not a secret
		// — it rides in every authorization redirect.
		if s.gmailAppConfigured {
			s.envClientID = c.ClientID
		}
		// Published BEFORE the vault gate below, and for the same reason the
		// sign-in URI is published before its own completeness gate: an operator
		// registers both URLs while CREATING the OAuth client, which is before
		// either flow can possibly work. The URL is a property of this
		// deployment's origins rather than of anything the operator has yet
		// configured, and it is not derivable from the sign-in one — that rides a
		// different base on a split deployment.
		// BOTH connectors, under the keys their own routes are served on. Gmail
		// and Calendar are separate flows sharing one Google app, so an operator
		// who registers only one gets redirect_uri_mismatch on the other — and
		// the key is `gmail`/`gcal`, never the `google` the sign-in route uses.
		// A SLICE, not a map: this list is a contract array an operator reads
		// top to bottom, and Go randomizes map iteration — the two connector
		// rows would swap places between boots of the same binary.
		for _, connector := range googleBackedConnectors {
			if uri := connectorCallbackURL(c.APIBaseURL, c.PublicBaseURL, connector.provider); uri != "" {
				s.redirectURIs = append(s.redirectURIs,
					crmcontracts.GoogleAppRedirectUri{Purpose: connector.purpose, Url: uri})
			}
		}
		// Without a vault the connect flow can't seal the refresh token, so
		// mounting the endpoints would only fail at the callback — leave the
		// surface its declared 501 instead. (WithKeyvault must precede this.)
		//
		// The APP's credentials are deliberately not part of this condition any
		// more: they may arrive at runtime from the stored setting, and an
		// installation that has one must not be left with an unbuilt signer. What
		// stays required is what only a deployment can supply — the state key and
		// the callback base — so a mounted transport always has a signing key
		// that clears minStateKeyLen.
		if !c.canSignState() || s.vault == nil {
			return
		}
		// The env-composed clients exist only where the ENVIRONMENT actually
		// carries the app. Built unconditionally they are a pair of usable-looking
		// clients holding an empty client id, and gmailApp's fallback would reach
		// for them the moment the stored app is not servable — sending a person to
		// Google's consent screen with `client_id=`, which fails there rather than
		// here and gives them nothing to act on. Nil is what makes the declared
		// 501 the answer instead.
		var (
			gmailOAuth gmail.OAuth
			gcalOAuth  gcal.OAuth
		)
		if c.canSync() {
			gmailOAuth, gcalOAuth = newGmailOAuth(c), newGcalOAuth(c)
		}
		s.connectorHandlers = connectorHandlers{
			registry:      newCaptureRegistryWithGoogle(pool, s.vault, s.googleAppResolver, c, cfg),
			authority:     identity.NewService(pool),
			oauth:         gmailOAuth,
			gmailAPI:      gmail.NewAPI(nil, ""),
			gcalOAuth:     gcalOAuth,
			gcalAPI:       gcal.NewAPI(nil, ""),
			signer:        newStateSigner([]byte(c.StateKey)),
			publicBaseURL: c.PublicBaseURL,
			apiBaseURL:    c.APIBaseURL,
			publicOrigin:  s.originStatus,
			// Named here because this literal REPLACES the struct: omitting it is
			// how the stored app became unreachable while every test still passed.
			googleCredentials: s.googleAppResolver,
		}
		// The send pre-flight reads the registry the connect flow just wrote to
		// — the same one, not a second construction: a mailbox the user connects
		// here must be the mailbox the check asks about. WithKeyvault already
		// wired this same call over the plain registry before this option ran;
		// re-wiring it here is NOT redundant, because this registry is a
		// different, richer object (newCaptureRegistryWithGoogle) — without this
		// line the mailbox half would keep answering off a registry with no
		// Gmail connector. The channel half answers identically off either
		// object: ChannelSendCapable is a pool query, not a connector lookup.
		installSendPreflight(s, pool)
	}
}
