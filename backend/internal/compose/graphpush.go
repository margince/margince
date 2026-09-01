// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The Microsoft Graph change-notification webhook: the consuming half of the
// subscription the renewal sweep maintains. A notification names a mailbox;
// this endpoint verifies the shared operator token, bumps that connection's
// pacing clock, and enqueues its sync — making Outlook capture push-driven with
// the poll demoted to a safety net, exactly as Gmail's push does. It sits on
// the shared webhook chassis (webhook.go): admission and response discipline
// are the chassis's job, this file declares only what is genuinely Graph's.
//
// TWO things differ from Gmail's, and both come from Microsoft:
//
//   - Graph will not create a subscription until the notification URL proves it
//     is ours: it POSTs there with `?validationToken=` and expects those bytes
//     echoed back as text/plain. That is the chassis's Challenge, and it runs
//     after the token check so the endpoint is never an echo oracle.
//   - A notification's `resource` names a directory object id this system never
//     stored, so it cannot say WHICH mailbox on its own. The subscription
//     carries the owner's address in `clientState`, which Microsoft echoes
//     verbatim — see the connector's Watch for why that field and not another.
//
// A webhook that carries a hint may be dropped. A webhook that carries the only
// copy may not. A Graph notification names a mailbox and a message id — a
// re-fetchable pointer into the delta the sync walks — never message content,
// so it is handled entirely in memory: no raw persisted, no EnqueueTx.

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/jobs"
)

// errNoClientState marks a notification carrying no clientState. Every
// subscription this system creates sets one, so its absence means the
// notification belongs to a subscription somebody else made against this URL —
// unroutable, and nothing a redelivery would fix.
var errNoClientState = errors.New("graph push: notification carries no clientState")

// errUnroutableDelivery is a delivery that named no mailbox at all — an empty
// batch, or one whose every entry lacked clientState. Distinct from the
// single-notification case so a log says which of the two arrived.
var errUnroutableDelivery = errors.New("graph push: the delivery names no mailbox to route to")

// maxNotificationBody bounds the body read, and is the ONLY bound on how much
// one delivery can ask for.
//
// The work a delivery costs is one fleet walk however many mailboxes it names
// (capture.BumpDueByMailboxes), so the batch size is no longer a lever on this
// server — which is why there is no count cap here. A cap would only be able to
// drop a delivery Microsoft legitimately sent: it batches per notification URL,
// and a large installation's burst genuinely names many mailboxes at once.
//
// The body is still bounded, at what a real batch is — a few hundred bytes per
// entry — rather than the megabyte a generic webhook allows.
const maxNotificationBody = 64 << 10

// graphNotifications is Graph's batch envelope: one POST may carry several.
type graphNotifications struct {
	Value []struct {
		// ClientState is the mailbox owner's address, put there by the
		// subscription this system created. Microsoft echoes it verbatim.
		ClientState string `json:"clientState"` //nolint:tagliatelle // Microsoft's wire format; must match to decode
	} `json:"value"`
}

// GraphPushConfig is the notification endpoint's identity. Token is the shared
// URL secret and is required — empty leaves the route absent, not open.
//
// There is no OIDC second factor to add: Microsoft signs nothing on a change
// notification, which is why its own guidance leans on clientState as a secret.
// This deployment spends clientState on routing and keeps the operator token as
// the only admission factor — the same posture Gmail's push has when a
// deployment configures no push identity.
type GraphPushConfig struct {
	Token string
}

// WithGraphPush mounts POST /webhooks/graph. An empty token disables the
// endpoint entirely.
func WithGraphPush(inserter *jobs.Runner, cfg GraphPushConfig) Option {
	return func(s *Server, pool *pgxpool.Pool) {
		if cfg.Token == "" || inserter == nil {
			return
		}
		s.graphPush = Webhook(graphPushSpec(pool, inserter, cfg.Token, s.log), s.log)
	}
}

// graphPushSpec declares Graph's side of the chassis: one operator token shared
// by every mailbox in the deployment (Microsoft delivers to the one URL the
// subscription named — there is no per-mailbox path to key on), the
// endpoint-ownership handshake, and a Handle that never persists the payload it
// is handed.
func graphPushSpec(pool *pgxpool.Pool, inserter *jobs.Runner, token string, log *slog.Logger) WebhookSpec {
	return WebhookSpec{
		Provider: "graph",
		MaxBody:  maxNotificationBody,
		Secret: func(r *http.Request) (want, got string) {
			return token, r.URL.Query().Get("token")
		},
		Challenge: func(r *http.Request) (string, bool) {
			t := r.URL.Query().Get("validationToken")
			return t, t != ""
		},
		Handle: graphBatchHandler(func(ctx context.Context, mailboxes []string) error {
			return bumpGraphMailboxes(ctx, pool, inserter, mailboxes, log)
		}),
		// 202, which is what Microsoft's own guidance asks for and what it
		// treats as delivered. Anything slower or louder than an immediate
		// acknowledgement counts against the endpoint's health, and enough of
		// those and Microsoft drops the subscription.
		OnAccept: http.StatusAccepted,
	}
}

// graphBatchHandler reads the batch envelope and hands each distinct mailbox to
// bump, once.
//
// Split from the bump itself so the envelope rules — what is malformed, what is
// unroutable, and which of the three dispositions each earns — can be proven
// without a database. Those rules decide whether Microsoft retries, and the
// three answers are not interchangeable.
//
// A malformed envelope is Poison: the same bytes would fail identically on
// redelivery. A routing or enqueue failure is Transient: redelivery is exactly
// the recovery path, because a notification is a re-fetchable pointer and the
// delta can always be walked again.
func graphBatchHandler(bump func(context.Context, []string) error) func(context.Context, *http.Request, []byte) (Disposition, error) {
	return func(ctx context.Context, _ *http.Request, body []byte) (Disposition, error) {
		var batch graphNotifications
		if err := json.Unmarshal(body, &batch); err != nil {
			return Poison, err
		}
		if len(batch.Value) == 0 {
			// The DELIVERY named nothing, which is not the same as a
			// notification arriving without state: the sentinel's own comment
			// puts an empty batch here, and a poison log saying "carries no
			// clientState" sends the reader looking for an entry there is none
			// of.
			return Poison, errUnroutableDelivery
		}
		// One entry per mailbox, however many notifications the batch carried
		// for it: Microsoft coalesces a burst into one POST, and a sync per
		// message would be the same sync started N times.
		seen := make(map[string]bool, len(batch.Value))
		mailboxes := make([]string, 0, len(batch.Value))
		for _, n := range batch.Value {
			if n.ClientState == "" || seen[n.ClientState] {
				continue
			}
			seen[n.ClientState] = true
			mailboxes = append(mailboxes, n.ClientState)
		}
		if len(mailboxes) == 0 {
			return Poison, errUnroutableDelivery
		}
		if err := bump(ctx, mailboxes); err != nil {
			return Transient, err
		}
		// A mailbox nobody connected is Accepted too: nothing here a redelivery
		// would fix, and Microsoft must stop retrying.
		return Accepted, nil
	}
}

// bumpGraphMailboxes moves each named mailbox's pacing clock to now and starts
// its sync.
func bumpGraphMailboxes(
	ctx context.Context, pool *pgxpool.Pool, inserter *jobs.Runner, mailboxes []string, log *slog.Logger,
) error {
	// Route by the provider-owned identity in the connector's own cursor — the
	// same column Gmail's push matches on, because both connectors write the
	// connected account's address there. ONE walk for the whole delivery.
	hits, err := capture.BumpDueByMailboxes(ctx, pool, providerGraph, mailboxes)
	if err != nil {
		return err
	}
	// Every hit is attempted before any failure is reported. One delivery names
	// many mailboxes across many workspaces, and returning at the first enqueue
	// fault would leave the rest of the batch unsynced until the redelivery —
	// which arrives with the SAME set, so a mailbox that keeps landing behind a
	// consistently failing one would never be reached at all.
	var faults error
	for _, d := range hits {
		if err := inserter.Enqueue(ctx, CaptureSyncArgs{
			Workspace:    d.Workspace.UUID,
			ConnectionID: d.ID.String(),
			Provider:     providerGraph,
		}, &river.InsertOpts{
			// river's default uniqueness window includes completed jobs;
			// activeSweepStates deliberately excludes them, so this must stay
			// exactly as-is — dropping ByState would suppress a legitimate
			// re-sync any time the prior one had finished.
			UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: activeSweepStates},
		}); err != nil {
			log.ErrorContext(ctx, "graph push: enqueueing sync", "connection", d.ID.String(), "err", err)
			faults = errors.Join(faults, err)
		}
	}
	// Joined, not swallowed: the handler answers Transient on any fault, so
	// Microsoft redelivers and the mailboxes that DID enqueue are protected
	// from a second sync by the uniqueness window rather than by this returning
	// early.
	return faults
}
