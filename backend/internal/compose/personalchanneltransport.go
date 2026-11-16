// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"github.com/margince/margince/backend/internal/modules/personalchannels"
	"github.com/margince/margince/backend/pkg/extension"
)

// corePerMemberTransports are core transports whose credential is one member's
// own. Nothing polls them (a member's client calls in), and each carries text only.
func corePerMemberTransports() []channelProviderFacts {
	return []channelProviderFacts{{
		provider: personalchannels.Provider, transport: transportCore,
		label: providerLabel(personalchannels.Provider), credentialModel: string(extension.CredentialPerMember), suppliesTransport: true,
	}}
}

// corePerMemberProviders names those transports, for the sets built from names.
func corePerMemberProviders() []string { return []string{personalchannels.Provider} }
