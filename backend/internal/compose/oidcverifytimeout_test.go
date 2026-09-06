// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What a slow identity provider costs a sign-in.
//
// The two outbound metadata reads that gate an in-flight authentication used to
// differ by 6x — 5s for the consent metadata, 30s for the keys — for the same
// kind of read, with no reason recorded for the difference.
//
// The caching is what decides the number, and it points the opposite way from
// the intuition that a longer timeout is safer: refreshes COALESCE, so every
// sign-in arriving during one slow fetch waits on it. A generous bound is
// therefore not patience with one caller, it is an outage for all of them.
//
// That the two are now EQUAL is held in backend/gates — the constants live in
// two modules that cannot import each other, so the parity is a source-level
// claim rather than one a package test can make.

import (
	"testing"
	"time"
)

// And it is a real bound rather than none. A zero Timeout on an http.Client
// means no timeout at all, which is the shape this reads like at a glance.
func TestTheKeyFetchIsActuallyBounded(t *testing.T) {
	t.Parallel()

	verifier := newOIDCVerifier("https://issuer.test/jwks", nil, nil)

	switch {
	case verifier.client.Timeout == 0:
		t.Error("the key-fetch client has no timeout: a provider that never answers holds every " +
			"sign-in arriving during the refresh it is coalescing, indefinitely")
	case verifier.client.Timeout > 10*time.Second:
		t.Errorf("the key-fetch client waits %s — long enough that a provider hiccup becomes a "+
			"sign-in outage, because the waiters are shared", verifier.client.Timeout)
	}
}
