// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contact360

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

func TestEveryDirectionIsOneTheContractSpells(t *testing.T) {
	t.Parallel()
	for _, direction := range relstrength.Directions() {
		if !crmcontracts.RelationshipDirection(direction).Valid() {
			t.Errorf("relstrength direction %q is not in the contract's RelationshipDirection enum", direction)
		}
	}
}
