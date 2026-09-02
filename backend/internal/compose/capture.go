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

// graphScopes are the Microsoft identity platform permissions a Graph mailbox
// connection requests: mail read for capture, the signed-in user's profile (the
// mailbox owner lookup), offline_access (the refresh token), and send for the
// governed outbound path. No modify, no delete, no shared mailboxes.
//
// They ride ONE consent for the same reason Gmail's pair does — Microsoft will
// not add a permission to an existing refresh token, so asking later would mean
// a second connection for the same mailbox. A mailbox connected before the send
// permission landed captures normally and refuses every send by name until it
// is reconnected.
//
// The send entry is the connector's OWN constant, not a copy of its text: what
// the consent requests and what the connector re-checks are then one string by
// construction. The second literal — the permission comms demands at the
// authority gate — cannot be imported by either (comms must not reach into a
// capture provider), so a fitness test binds it here instead: sendscope_test.go.
var graphScopes = []string{"offline_access", "User.Read", "Mail.Read", graph.SendScope}

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
	// TracePayloads is capture.trace_payloads, already resolved against its
	// default by CaptureConfigFromDeploy: the 24-hour trace keeps each message's
	// sender and subject. ON unless the deployment file turns it off, because a
	// trace of decisions naming nobody cannot answer why a message did not
	// arrive. Settable only in that file — a member must not be able to change
	// retention of their colleagues' subjects in either direction.
	//
	// A plain bool here on purpose: the three-state question is the operator's
	// and is answered at the boundary, by deployconfig's TracesPayloads.
	//
	// Every zero value below this line — the site_lead accept path's
	// `CaptureConfig{}`, the registry enumerations — means no deployment config
	// at all, and those keep the off behaviour they have today rather than
	// inheriting a default meant for a booted role.
	//
	// Held by: TestOnlyTheResolverReadsTheTracePayloadsField (backend/internal/platform/deployconfig/capture_test.go)
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
		TracePayloads:      c.TracesPayloads(),
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
		// The audience derivation, from the module that owns `activity`. It
		// runs INSIDE the capture transaction because the audience it derives
		// is the answer to "may a colleague read this", and a message that is
		// briefly readable before a later pass narrows it has already been
		// readable.
		WithAudienceRecompute(activities.RecomputeAudienceTx).
		// The 24-hour trace's payload posture. It rides the Sink because the
		// Sink is where a payload would be written, and it is a deployment
		// decision rather than a workspace one -- there is no API that flips it.
		WithTracePayloads(cfg.TracePayloads)
}
