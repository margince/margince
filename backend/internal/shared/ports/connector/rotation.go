// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package connector

// The credential-ROTATION half of the connector seam: what a provider that
// REPLACES the durable secret each time it is used needs in order to say so,
// and how the replacement reaches the vault.
//
// Its own file because it is a different obligation from everything else here.
// Auth is otherwise write-once — sealed at Connect and read on every sync — and
// most providers keep it that way: a Google refresh token is issued once and
// stays valid until it is revoked. Microsoft's is not. The identity platform
// issues a NEW refresh token on every redemption, and while the old one keeps
// working for its own lifetime, that lifetime is a ceiling (90 days of
// inactivity for a confidential client) and can be cut short by a password
// change, an admin revoke or a conditional-access policy. A connection whose
// stored secret is never refreshed therefore ages out on a schedule nobody set,
// and its human is asked to reconnect for no reason they can see.
//
// WHY A SINK RATHER THAN A RETURN VALUE. Connector.Sync's signature is frozen,
// and widening it would make every capture-only provider answer a question it
// has no opinion about. A sink also matches when the rotation actually happens:
// mid-pull, inside the token mint, before anything is known about whether the
// sync will succeed. The connector reports it at the moment it learns it, and
// the registry decides what that is worth.
//
// WHY A COPY RATHER THAN A FIELD. A connector is registered ONCE and shared
// across every connection the fleet syncs concurrently; a sink stored on the
// shared instance would carry one connection's credential into another's write.
// WithCredentialSink returns a copy bound to one sync, which is the same shape
// WithBounceSink already uses for the same reason.

import "context"

// CredentialSink receives a durable credential that has replaced the one a sync
// started with. The registry supplies one per sync.
//
// A sink that fails does NOT fail the sync. The old credential is still valid
// when the new one is issued — that is what makes rotation safe to miss — so a
// re-seal that cannot be written costs one cycle's freshness, where failing the
// pull would cost the mail. The implementation reports the fault and the sync
// carries on.
type CredentialSink interface {
	// Rotated is called with the complete replacement Auth bundle, not the
	// secret alone: what is durable inside the bundle is the connector's own
	// business, and a sink that had to understand it would be a second reader
	// of a format only the connector owns.
	Rotated(ctx context.Context, auth Auth) error
}

// CredentialRotator is the OPTIONAL seam a connector implements when its
// provider replaces the durable credential on use. Type-asserted like Watcher,
// Backfiller and EmailSender, so the frozen Connector interface is unchanged
// and a provider whose credential is stable simply does not implement it.
//
// A connector that implements this MUST report only a credential it has
// verified the provider accepted — a bundle reported before the exchange
// succeeded would replace a working secret with one that was never issued.
type CredentialRotator interface {
	// WithCredentialSink returns a COPY of this connector that reports a
	// rotated credential to sink. The receiver is left unchanged, so the
	// registered instance never holds one connection's sink.
	WithCredentialSink(sink CredentialSink) Connector
}

// RotatingSyncer binds a connector to a per-sync credential sink when it can
// take one, and returns it unchanged when it cannot.
//
// One spelling, here beside the interface, because the alternative is a type
// assertion at each call site and the failure mode of a missed one is silent:
// the sync works, the rotation is dropped, and the connection ages out months
// later with nothing pointing back at the omission.
//
//nolint:ireturn // returns the Connector seam by design — the caller holds an interface either way
func RotatingSyncer(c Connector, sink CredentialSink) Connector {
	rotator, ok := c.(CredentialRotator)
	if !ok {
		return c
	}
	return rotator.WithCredentialSink(sink)
}
