// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestPersonalChannelsIsAMintableScope(t *testing.T) {
	if err := admitScopes([]string{"personal_channels"}); err != nil {
		t.Fatalf("personal_channels was refused: %v", err)
	}
	if !slices.Contains(passportScopeVocabulary, principal.ScopePersonalChannels) {
		t.Fatal("personal_channels is not in the scope vocabulary the discovery documents advertise")
	}
}

func TestNoOtherScopeGrantsPersonalChannels(t *testing.T) {
	var older []principal.Scope
	for _, scope := range passportScopeVocabulary {
		if scope != principal.ScopePersonalChannels {
			older = append(older, scope)
		}
	}
	every := principal.NewScopeSet(older...)
	if every.Has(principal.ScopePersonalChannels) {
		t.Fatal("the five older scopes together grant personal_channels")
	}
}

// Pull hands a reply to the caller that asked; Margince itself sends nothing out.
func TestPersonalChannelsPutsNoRequestOnTheWire(t *testing.T) {
	if principal.ScopePersonalChannels.Egresses() {
		t.Fatal("personal_channels reports that it egresses")
	}
}
