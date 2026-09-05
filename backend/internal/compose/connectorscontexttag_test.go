// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Absence on the wire is a connection that chose no word. An empty object would
// say a word WAS chosen and could not be read, which is a different fact and one
// that never happens.
func TestAConnectionWithNoWordCarriesNothingOnTheWire(t *testing.T) {
	if got := contextTagOnWire(nil); got != nil {
		t.Errorf("a connection with no word put %+v on the wire", got)
	}
}

// The archived flag is the whole reason this is an object rather than an id: an
// operator whose filing stopped needs to see why, and a bare id could not say
// it.
func TestAChosenWordCarriesItsNameAndWhetherItIsRetired(t *testing.T) {
	id := ids.NewV7()
	got := contextTagOnWire(&capture.ContextTag{ID: id, Name: "Nord inbox", Archived: true})
	if got == nil {
		t.Fatal("a chosen word put nothing on the wire")
	}
	if got.Id == nil || ids.UUID(*got.Id) != id {
		t.Errorf("the wire names word %v, want %v", got.Id, id)
	}
	if got.Name == nil || *got.Name != "Nord inbox" {
		t.Errorf("the wire names %v rather than the word the vocabulary spells", got.Name)
	}
	if !got.Archived {
		t.Error("a retired word reads as live on the wire, so the operator sees no reason for the silence")
	}
}
