// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"github.com/margince/margince/backend/internal/modules/personalchannels"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/pkg/extension"
)

// corePerMemberTransports are the core transports whose credential is one
// member's own rather than one bot the installation shares. They are not capture
// connectors: nothing polls them, a member's own client calls in. Each carries
// text only.
func corePerMemberTransports() []channelProviderFacts {
	return []channelProviderFacts{{
		provider:          personalchannels.Provider,
		transport:         transportCore,
		label:             providerLabel(personalchannels.Provider),
		credentialModel:   string(extension.CredentialPerMember),
		suppliesTransport: true,
		carriage:          connector.Carriage{},
	}}
}

// corePerMemberProviders names those transports, for the reserved and sendable
// sets that are built from names.
func corePerMemberProviders() []string {
	facts := corePerMemberTransports()
	out := make([]string, 0, len(facts))
	for _, f := range facts {
		out = append(out, f.provider)
	}
	return out
}
