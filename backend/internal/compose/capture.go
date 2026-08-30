// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The capture path, assembled: the one Sink over the pool (with the
// approvals engine as its merge-stager — dedupe collisions become 🟡
// proposals, never auto-merges), the connector registry with identity
// as the live-authority resolver — composed here so capture never
// imports identity or approvals (ADR-0054 §9).

import (
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/gcal"
	"github.com/margince/margince/backend/internal/modules/capture/gmail"
	"github.com/margince/margince/backend/internal/modules/capture/graph"
	"github.com/margince/margince/backend/internal/modules/capture/imap"
	"github.com/margince/margince/backend/internal/modules/capture/offlinedemo"
	"github.com/margince/margince/backend/internal/modules/capture/telegram"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/deployconfig"
	"github.com/margince/margince/backend/internal/platform/keyvault"
)

// gmailScopes are the Google scopes a Gmail connection requests: mail read for
// capture, and send for the governed outbound path. They ride ONE consent
// because Google will not add a scope to an existing refresh token — a second
// grant would mean a second connection for the same mailbox.
//
// The send scope permits transmission only; it cannot read, modify, or delete.
// The pair is still least-privilege: no gmail.modify, no settings, no delete.
// The calendar connector owns its own calendar-read scope inside the gcal
// package.
//
// The send entry is the connector's OWN constant, not a copy of its text: what
// the consent requests and what the connector re-checks are then one string by
// construction. The second literal — the scope comms demands at the authority
// gate — cannot be imported by either (comms must not reach into a capture
// provider), so a fitness test binds it here instead: sendscope_test.go.
var gmailScopes = []string{
	"https://www.googleapis.com/auth/gmail.readonly",
	gmail.SendScope,
}

// graphScopes are the Microsoft identity platform scopes the read-only Graph
// capture connector requests: mail read + the signed-in user's profile (the
// mailbox owner lookup) + offline_access (the refresh token). No send, no
// modify.
var graphScopes = []string{"offline_access", "User.Read", "Mail.Read"}

// CaptureConfig is the deployment's capture list-config, threaded from
// margince.yaml's `capture:` block into the Sink's suppression gates: the
// CAP-PARAM-6 transactional/ESP additions plus its allowlist (ADR-0072) — plus
// the process logger the Sink's post-commit steps report through. The zero
// value is the pinned baseline with no deployment additions and the default
// logger.
//
// The consumer-mail list (CAP-PARAM-5) is deliberately NOT here: it is the
// workspace's own, edited in the settings surface and read per transaction, so
// a correction takes effect on the next message instead of the next restart.
type CaptureConfig struct {
	TransactionalExtra []string // capture.transactional_extra (CAP-PARAM-6 infra eSLDs)
	TransactionalNever []string // capture.transactional_never (CAP-PARAM-6 allowlist)
	// TracePayloads is capture.trace_payloads: the 24-hour trace keeps each
	// message's sender and subject. Off by default and settable only in the
	// deployment file — a member must not be able to turn on retention of their
	// colleagues' subjects.
	TracePayloads bool
	// Logger carries the process logger to the post-commit steps the Sink
	// drives, where a fault is reported rather than returned (nothing may fail
	// a capture). Nil falls back to the default logger — the site_lead accept
	// path composes a Sink without a deployment config at all.
	Logger *slog.Logger
	// Blob is the object store a captured message's files are written to. Nil
	// is a role that keeps no files: the messages still land, and their
	// attachments do not — an unconfigured store must not cost correspondence.
	//
	// Held as the STORE rather than as a built keeper so that whoever sets this
	// last still wins: an option that assigns this whole struct cannot silently
	// drop a separately-assigned keeper, which is a failure with no error and
	// no missing file to notice.
	Blob blobstore.Store
}

// logger is the configured logger, or the process default.
func (c CaptureConfig) logger() *slog.Logger {
	if c.Logger != nil {
		return c.Logger
	}
	return slog.Default()
}

// WithCaptureConfig records the deployment's capture suppression-list config on
// the Server so EVERY registry construction — the Gmail one, the vault-rebuilt
// IMAP/fallback one (WithKeyvault), and the graph-only one (WithGraphCapture) —
// applies the transactional/free-mail additions, not only the Gmail path. Apply
// it before WithKeyvault/WithGraphCapture in the option list; omitting it keeps
// the pinned baselines.
func WithCaptureConfig(cfg CaptureConfig) Option {
	return func(s *Server, _ *pgxpool.Pool) { s.captureConfig = cfg }
}

// CaptureConfigFromDeploy maps the deployment's `capture:` block onto the
// compose suppression config the Sink gates read (CAP-PARAM-6, ADR-0072).
//
// Every role that boots with a config file goes through here, which is why the
// stale-key warnings are reported here: a setting that is still accepted but no
// longer acts must say so once, at boot, or an operator goes on believing the
// file governs something it does not.
func CaptureConfigFromDeploy(c deployconfig.Capture, log *slog.Logger) CaptureConfig {
	for _, warning := range c.Warnings() {
		log.Warn("capture configuration: " + warning)
	}
	return CaptureConfig{
		TransactionalExtra: c.TransactionalExtra,
		TransactionalNever: c.TransactionalNever,
		TracePayloads:      c.TracePayloads,
		Logger:             log,
	}
}

// NewCaptureRegistry builds the connector registry; process roles register
// their compiled-in connectors on it and drive SyncOnce. The vault seals and
// resolves each connection's credential (nil is valid for a role that only
// runs the transient one-shot pull, which persists no credential). cfg carries
// the deployment's suppression-list additions and the logger; the zero value is
// the baselines and the default logger.
//
// A nil pool builds the registry for ENUMERATION only: nothing here queries, so
// the construction is complete without one, and only a caller that goes on to
// sync or send needs a real pool. CoreChannelProviders is the shipped caller
// that takes it that way — asking which transports this binary compiled in is a
// question about the binary, not about any database.
func NewCaptureRegistry(pool *pgxpool.Pool, vault keyvault.Vault, cfg CaptureConfig) *capture.Registry {
	db := InstallationDB(pool)
	r := capture.NewRegistry(db, newCaptureSink(pool, cfg), identity.NewService(pool), vault).
		// The digest's projects section is answered here because its reads
		// span the deals module's tables (digestprojects.go).
		WithDigestProjects(digestProjectsSource)
	// What is waiting for the reader, counted through the same seams the Today
	// surface counts it with (digestreview.go). Skipped without a pool, which
	// is the enumerate-only construction CoreChannelProviders makes: it builds
	// no digest, so a source reading a database it does not have would fail on
	// a path that never counts anything.
	if pool != nil {
		r = r.WithDigestReview(newDigestReviewSource(pool, approvals.NewService(db)))
	}
	// The standing IMAP connector needs no deployment config — credentials
	// are per-connection, vault-sealed — so every capture-capable role
	// carries it. With a pool it also records the delivery reports the
	// mailbox receives; enumerate-only constructions keep the old
	// drop-everything behaviour, because there is no database to record into.
	standingIMAP := imap.NewStanding()
	if pool != nil {
		standingIMAP = standingIMAP.WithBounceSink(newBounceSink(pool))
	}
	r.Register(standingIMAP)
	// Telegram is registered on the same terms and for the same reason: a bot
	// binding's token is per-connection and vault-sealed, so there is no
	// deployment-wide app to configure. It is the registration that lets the send
	// path resolve the workspace's bot at all — Registry.ChannelSenderFor
	// type-asserts the message seam off this map, so an unregistered connector
	// reads as "this installation has no Telegram integration" and parks every
	// reply a rep writes.
	r.Register(telegram.New(telegram.NewAPI(nil, "")))
	// The offline demo connector, registered unconditionally and INERT until a
	// capture_connection names it — which only the demo seeder and
	// scripts/seed-dev.sql do. It reaches no network and holds no credential,
	// so there is nothing to configure and nothing to gate on; the finance
	// mirror's offline_demo provider is registered on the same terms.
	r.Register(offlinedemo.New(offlineDemoDirectory{pool: pool}))
	// The derived channel vocabulary is NOT reconciled here. Constructing this
	// registry is config-gated — a role builds it only when a keyvault root key
	// is configured — and the registry write is not, so it runs as its own boot
	// step (ReconcileChannelProviders) that every role reaches. See there.
	return r
}

// newCaptureSink assembles the ONE fully-guarded Sink over the pool — the
// merge-stager and the counterparty auto-create resolver attached. Every
// capture path shares this spelling: the connector
// registry above, and the site_lead accept effect (siteleadaccept.go),
// which captures through the Sink directly without needing a registry.
func newCaptureSink(pool *pgxpool.Pool, cfg CaptureConfig) *capture.Sink {
	ensurer := peopleEnsurer{
		store:  newCounterpartyStore(pool),
		triage: newDomainTriageTrigger(pool, cfg.logger()),
		log:    cfg.logger(),
	}
	return capture.NewSink(InstallationDB(pool)).
		// The files a captured message carried, written by the module that owns
		// the attachment table. Built here, from the store, so every role that
		// composes a sink gets the same one — the worker runs mail capture and
		// never sees the api's options.
		WithFileKeeper(capturedFileKeeper{store: activities.NewStore(InstallationDB(pool)).WithBlobstore(cfg.Blob)}).
		WithStager(mergeStager{svc: approvals.NewService(InstallationDB(pool))}).
		// The ADR-0063 auto-create pipeline: every captured mail ensures
		// its counterparty exists, through the people module's ONE dedupe
		// chokepoint — composed here so capture never imports people. The
		// free-mail (CAP-PARAM-5) and transactional/ESP (CAP-PARAM-6, ADR-0072)
		// gates decide which senders derive no company / no counterparty.
		WithEnsurer(ensurer,
			capture.NewTransactionalList(cfg.TransactionalExtra, cfg.TransactionalNever)).
		// The channel twin of the line above (telegram-oa design §6.4): an
		// inbound channel message reaches the SAME module through its own
		// contract — one adapter serving two seams, so the two ensures cannot
		// drift onto different dedupe implementations.
		WithChannelEnsurer(ensurer).
		// The project attribution ladder's subject-key rung, answered by the
		// module that owns the project table — composed here for the same
		// reason the counterparty resolver is: capture must not import a
		// sibling, and which project a subject names is a question about
		// another module's records.
		// The stamp beside it comes from activities, which owns `activity` —
		// filing an activity under a project qualifies its correspondence as
		// a Handelsbrief (D5), and that classification commits with the link.
		WithProjectAttribution(capture.ProjectAttribution{
			Keys:  ProjectsStore(pool),
			Stamp: activities.StampCorrespondenceForProject,
		}).
		// The 24-hour trace's payload posture. It rides the Sink because the
		// Sink is where a payload would be written, and it is a deployment
		// decision rather than a workspace one -- there is no API that flips it.
		WithTracePayloads(cfg.TracePayloads)
}

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
// the standing IMAP connector needs no deployment config — with the gmail
// and graph connectors added when their OAuth apps are configured. A provider
// nobody registered simply never appears in the dispatcher's provider list.
func CaptureSyncRegistry(pool *pgxpool.Pool, vault keyvault.Vault, c GmailConfig, g GraphConfig, cfg CaptureConfig, log *slog.Logger) *capture.Registry {
	// The worker resolves the STORED app exactly as the api does. Without this
	// a mailbox connected against a stored app would connect and then never
	// sync: the poll would find no Google connector registered and skip it
	// silently, which reads as an empty inbox rather than as a broken one.
	reg := newCaptureRegistryWithGoogle(pool, vault, newGoogleAppResolver(pool, vault, log), c, cfg)
	if g.canSync() {
		reg.Register(graph.New(newGraphOAuth(g), graph.NewAPI(nil, "")).
			WithBounceSink(newBounceSink(pool)))
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
func WithGmailCapture(c GmailConfig, cfg CaptureConfig) Option {
	return func(s *Server, pool *pgxpool.Pool) {
		s.gmailAppConfigured = c.canSync() // the send pre-flight's fact, recorded before the gate below
		// The setup surface reads the same fact. Stamped HERE and only here:
		// WithGmailCapture requires the vault and so always runs after
		// WithKeyvault, which means a copy taken there would always read false.
		s.envGoogleApp = s.gmailAppConfigured
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
