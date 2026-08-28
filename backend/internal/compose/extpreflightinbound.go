// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"fmt"

	"github.com/margince/margince/backend/pkg/extension"
)

// preflightInbound validates one unit's anonymous edges through the same
// published Validate the manifest generator runs, and refuses the two things a
// single declaration cannot see: a slug declared twice within the unit, and a
// secret the unit never declared.
//
// The second is the one worth stating. An endpoint verifies against a
// user-scoped secret by NAME, and a name the unit did not declare is a name the
// secrets port will never hold a value for — so the endpoint would mount, serve,
// and refuse every request that ever reached it, looking exactly like a sender
// with the wrong key. Boot is the last place that mistake is cheap.
func preflightInbound(e extension.Extension) error {
	if len(e.Inbound) == 0 {
		return nil
	}
	declared := make(map[string]bool, len(e.Secrets))
	for _, s := range e.Secrets {
		// User-scoped only. A workspace secret is the installation's own
		// credential at a provider; an inbound edge verifies what ONE member's
		// counterparty signed, and a shared secret would let any member's sender
		// sign for any other's endpoint.
		if s.Scope == extension.SecretScopeUser {
			declared[s.Key] = true
		}
	}
	seen := make(map[string]bool, len(e.Inbound))
	for _, endpoint := range e.Inbound {
		if err := endpoint.Validate(); err != nil {
			return fmt.Errorf("compose: extension %q: %w", e.Name, err)
		}
		if seen[endpoint.Slug] {
			return fmt.Errorf("compose: extension %q declares inbound endpoint %q twice", e.Name, endpoint.Slug)
		}
		seen[endpoint.Slug] = true
		if !declared[endpoint.Secret] {
			return fmt.Errorf("compose: extension %q's inbound endpoint %q verifies against secret %q, which the unit does not declare as a user-scoped SecretsRequest — the endpoint would mount and refuse every request that reached it, indistinguishable from a sender holding the wrong key",
				e.Name, endpoint.Slug, endpoint.Secret)
		}
	}
	return nil
}

// reserveInboundMounts refuses two units asking for the same mounted path.
//
// It is a fact about the composed SET, so it is accumulated across units rather
// than asked of one declaration — the same shape claimedProviders takes. The
// path is `/webhooks/ext/<unit>/<slug>`, so a collision needs both halves equal,
// which the per-unit namespace reservation already makes impossible. It is held
// anyway: the mount is derived from the pair, and a check that the pair is
// unique must not rest on a second check happening to make it so.
func reserveInboundMounts(e extension.Extension, mounts map[string]extension.Name) error {
	for _, endpoint := range e.Inbound {
		path := string(e.Name) + "/" + endpoint.Slug
		if owner, taken := mounts[path]; taken {
			return fmt.Errorf("compose: extensions %q and %q both ask to mount /webhooks/ext/%s", owner, e.Name, path)
		}
		mounts[path] = e.Name
	}
	return nil
}
