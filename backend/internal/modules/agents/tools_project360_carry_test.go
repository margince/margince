// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// An assistant reads both date readings a screen does. So it can tell an
// agreement that has not started from one that has run out.
func TestProject360CarriesAContractsTermState(t *testing.T) {
	status := crmcontracts.ContractStatusActive
	outOfContract := false
	notStarted := crmcontracts.ContractTermStateNotStarted
	got := project360Contracts([]crmcontracts.Contract{{
		Title: "Starts next month", Status: &status,
		UnderContract: &outOfContract, TermState: &notStarted,
	}})
	if len(got) != 1 {
		t.Fatalf("carried %d contracts, want 1", len(got))
	}
	if got[0].Status != "active" || got[0].UnderContract || got[0].TermState != "not_started" {
		t.Errorf("carried %+v, want active, out of contract, not_started", got[0])
	}
}

// A timeline row tells the agent which side wrote it, as compose's writer side
// answers. A colleague's mail can arrive inbound and still be ours.
func TestProject360CarriesTheSideThatWroteARow(t *testing.T) {
	inbound := crmcontracts.ActivityDirectionInbound
	got := project360Activities([]crmcontracts.Activity{{Kind: crmcontracts.ActivityKindEmail, Direction: &inbound}},
		func(crmcontracts.Activity) string { return string(crmcontracts.ActivityDirectionOutbound) })
	if len(got) != 1 || got[0].Direction != "outbound" {
		t.Errorf("carried %+v, want the writer side outbound rather than the transport's inbound", got)
	}
}
