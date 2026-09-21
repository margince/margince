// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// Whether this installation offers email + password sign-in at all.
//
// An installation that puts an identity provider in front of Margince asks for
// exactly one thing here: that the password door be shut, so a credential the
// IdP does not govern cannot open a session. `auth.password.enabled=false` is
// that switch, and this file is where the deployment's answer meets the two
// surfaces that must agree about it — the anonymous capabilities probe the
// login screen renders from, and the routes behind it.

// WithPasswordLogin injects whether the deployment offers the password method
// (deployconfig `auth.password.enabled`).
//
// The field it sets is NEGATIVE so that its zero value is "offered", which is
// what every handler set built without this option has always done and must
// keep doing: a composition that never mentions authentication methods serves
// the one method every installation starts with, and a fail-closed default
// here would mean no way in at all rather than a narrower one.
//
// The composition root refuses to boot a deployment that turns this off with
// no federated provider mounted, which is where that question is answerable —
// this module knows what it was told and not what else was composed.
func (h Handlers) WithPasswordLogin(enabled bool) Handlers {
	h.passwordLoginDisabled = !enabled
	return h
}

// passwordLoginOffered reports whether a password may open a session here. The
// probe and the routes read it rather than each testing the field, so what a
// login screen renders and what the route it posts to does are answers to the
// same question — the dead affordance GetAuthCapabilities exists to prevent.
func (h Handlers) passwordLoginOffered() bool {
	return !h.passwordLoginDisabled
}

// selfServiceRecoveryOffered reports whether POST /auth/forgot-password can do
// anything: the transport and the canonical base a link is built on, AND a
// password method to recover into.
//
// The ADMIN-issued set-password link is deliberately not this question. That
// one provisions a seat rather than offering a way in, and an installation
// turning the method back on must not have to re-provision everybody first.
func (h Handlers) selfServiceRecoveryOffered() bool {
	return h.passwordLoginOffered() && h.canSendPasswordLink()
}

// selfServiceRecoveryOff is what forgot-password says when it cannot complete,
// for either reason. It names the flow rather than the missing half: which one
// an installation is short of is its operator's business and not an anonymous
// caller's.
const selfServiceRecoveryOff = "self-service password recovery is not available at this installation"

// passwordMethodOff is what a route says when a deployment closed the password
// door. 501 rather than 401: there is no credential to be wrong about, and a
// caller told "invalid email or password" would keep trying one. It is the same
// answer forgot-password already gives an installation with no mail transport —
// the route is implemented and this deployment does not offer it.
const passwordMethodOff = "password sign-in is not enabled for this installation"
