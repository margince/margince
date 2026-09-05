// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Assembling the lane the installation's own mail rides, out of parts three
// different options supply.
//
// The lane needs three things and no option carries all three: WithOperatorMail
// brings the relay, WithPublicBaseURL brings the origin a link is built on, and
// WithKeyvault brings somewhere to seal the link. Options run in whatever order
// a role composes them, so each of the three re-runs the assembly and the last
// one to arrive completes it. That is the same backfill WithKeyvault already
// does for the object store and the data reset, and for the same reason: an
// option that assumed it ran last would silently wire half a lane.

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/platform/mailer"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ControllerRelayFor adapts an operator relay to the dispatcher's controller
// seam, or reports none when this deployment has no relay.
//
// Exported because the worker role composes the lane from its own config, and
// building a second SMTP client there would resolve the same sealed credential
// twice and let the two drift.
//
//nolint:ireturn // returns the comms-side seam by design: the adapter type is unexported
func ControllerRelayFor(m mailer.Mailer) comms.ControllerRelay {
	if m == nil {
		return nil
	}
	return mailerRelay{m}
}

// WithConfirmLinkVault wires ONLY the vault the confirm link is sealed in.
//
// Separate from WithKeyvault, which does considerably more: it also installs the
// outbound send pre-flight over the capture registry, so a role that composed it
// merely to seal a confirm link would start refusing ordinary sends with
// "mailbox not send capable". The two are different asks, and a role that wants
// the lane should not have to accept the other.
//
// A role composing WithKeyvault gets this for free — that option sets the same
// field — so no deployment needs both.
func WithConfirmLinkVault(vault keyvault.Vault) Option {
	return func(s *Server, pool *pgxpool.Pool) {
		s.vault = vault
		s.rewireConfirmationLane(pool)
	}
}

// WithControllerMail wires the job runner the installation's own notices are
// queued on.
//
// Separate from WithDelivery, which takes an already-built stager and cannot
// hand this path a runner. The lane also needs a relay (WithOperatorMail), a
// vault (WithKeyvault) and a public origin (WithPublicBaseURL); each of the four
// re-runs the assembly, so they compose in any order and the last to arrive
// completes it.
func WithControllerMail(runner *jobs.Runner) Option {
	return func(s *Server, pool *pgxpool.Pool) {
		s.confirmRunner = runner
		s.rewireConfirmationLane(pool)
	}
}

// mailerRelay adapts the operator relay to the dispatcher's controller seam.
//
// An adapter rather than a second interface on mailer.Mailer, because what the
// two want differs: a password reset is fire-and-forget text, and a controller
// delivery claims an RFC822 identity the row is keyed on. The identity is
// carried through here so a relay that honours it keys the sent copy the same
// way the delivery row does.
type mailerRelay struct{ m mailer.Mailer }

// SendControllerMail hands one rendered message to the operator relay.
func (r mailerRelay) SendControllerMail(ctx context.Context, msg comms.ControllerMessage) error {
	return r.m.Send(ctx, msg.To, msg.Subject, msg.TextBody)
}

// confirmLinkVault seals a confirm link for the installation's own workspace.
//
// The workspace comes from the CONTEXT rather than from construction, because a
// vault entry is workspace-scoped and the store that writes one runs inside a
// request whose workspace is already established. A call with none is a
// programming fault, not a tenancy question, so it fails rather than guessing.
type confirmLinkVault struct{ v keyvault.Vault }

// Put seals the plaintext link and returns the reference the delivery carries.
func (c confirmLinkVault) Put(ctx context.Context, secret string) (string, error) {
	ws, ok := principal.WorkspaceID(ctx)
	if !ok {
		return "", errNoWorkspaceForConfirmLink
	}
	ref, err := c.v.Put(ctx, ids.From[ids.WorkspaceKind](ws), []byte(secret))
	if err != nil {
		return "", err
	}
	return string(ref), nil
}

// controllerPayloads reads and destroys the sealed links at dispatch.
//
// The workspace comes from the CONTEXT, like the seal side: the dispatcher runs
// inside a job whose workspace is already bound, and a vault entry is scoped to
// it. Taking one at construction would let a worker built for one installation
// read another's material, which is the one mistake this type must not make.
type controllerPayloads struct{ v keyvault.Vault }

// Get returns the plaintext link for one delivery.
func (c controllerPayloads) Get(ctx context.Context, ref string) (string, error) {
	ws, ok := principal.WorkspaceID(ctx)
	if !ok {
		return "", errNoWorkspaceForConfirmLink
	}
	secret, err := c.v.Get(ctx, ids.From[ids.WorkspaceKind](ws), keyvault.Ref(ref))
	if err != nil {
		return "", err
	}
	return string(secret), nil
}

// Delete destroys the material once the message can no longer be sent.
func (c controllerPayloads) Delete(ctx context.Context, ref string) error {
	ws, ok := principal.WorkspaceID(ctx)
	if !ok {
		return errNoWorkspaceForConfirmLink
	}
	return c.v.Delete(ctx, ids.From[ids.WorkspaceKind](ws), keyvault.Ref(ref))
}

// rewireConfirmationLane rebuilds the lane from whatever parts have arrived.
//
// It is called by each of the three options that supply a part. Until all three
// are present the consent store keeps no lane at all, and issueLink reports a
// link it minted and did not send — which is the honest answer for an
// installation that cannot mail one.
func (s *Server) rewireConfirmationLane(pool *pgxpool.Pool) {
	if s.controllerRelay == nil || s.vault == nil || s.confirmLinkBase == "" || s.confirmRunner == nil {
		return
	}
	s.consentHandlers = s.WithConfirmationLane(
		NewControllerMailQueue(pool, s.confirmRunner),
		confirmLinkVault{s.vault},
		s.confirmLinkBase,
	)
}

// errNoWorkspaceForConfirmLink marks a seal attempted with no workspace in
// context. It is a programming fault rather than a tenancy answer: the store
// that seals a link runs inside a request whose workspace is established, so
// reaching here means a caller built one outside that.
var errNoWorkspaceForConfirmLink = errors.New("compose: sealing a confirm link outside workspace context")
