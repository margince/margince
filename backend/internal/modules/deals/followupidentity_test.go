// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"encoding/json"
	"testing"

	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// The identity field must appear in the PAYLOAD with the same string value, or
// canonicalIdentity refuses the staging. This pins the JSON spelling of DealID
// rather than trusting that a uuid marshals the way the identity is built.
func TestFollowUpProposalCarriesItsDealIDAsAPlainString(t *testing.T) {
	id := ids.NewV7()
	raw, err := json.Marshal(FollowUpProposal{DealID: ids.From[ids.DealKind](id)})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	got, ok := fields["deal_id"]
	if !ok {
		t.Fatal("the payload has no deal_id, so an identity keyed on it can never match")
	}
	var asString string
	if err := json.Unmarshal(got, &asString); err != nil {
		t.Fatalf("deal_id is not a JSON string (%s): identity members must be strings", got)
	}
	if asString != id.String() {
		t.Errorf("deal_id = %q, want %q — the identity would not containment-match", asString, id.String())
	}
}
