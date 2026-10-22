// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// One record must not read one way through a URL and another through a tool.
// That is the whole reason the marker exists on the tool surface. Nothing in
// the type system ties the words together, so this does.
func TestBothSurfacesSpellTheProvenanceLabelsTheSameWay(t *testing.T) {
	t.Parallel()
	for _, pair := range []struct {
		tool     string
		contract crmcontracts.SearchResultTrustTier
	}{
		{datasource.TrustTierAuthoritative, crmcontracts.SearchResultTrustTierSearchResultTrustTierAuthoritative},
		{datasource.TrustTierExternal, crmcontracts.SearchResultTrustTierSearchResultTrustTierExternal},
	} {
		if pair.tool != string(pair.contract) {
			t.Errorf("the tool surface says %q where the contract says %q, so one record reads two ways",
				pair.tool, pair.contract)
		}
	}
}

// A native read is not an unknown read. Omitting the label reported the weaker
// of two statements about a record this installation holds itself.
func TestARecordThisInstallationHoldsIsLabelledAuthoritative(t *testing.T) {
	t.Parallel()
	got := datasource.FreshnessInfo{Authoritative: true}.TrustTier()

	if got == "" {
		t.Fatal("a native read carries no label, which a reader must take as unknown")
	}
	if got != datasource.TrustTierAuthoritative {
		t.Errorf("a native read is labelled %q, want %q", got, datasource.TrustTierAuthoritative)
	}
}

// The copy case is the one the marker was added for, and it keeps its answer.
func TestACopyOfAnotherSystemsRecordStaysExternal(t *testing.T) {
	t.Parallel()
	if got := (datasource.FreshnessInfo{Authoritative: false}).TrustTier(); got != datasource.TrustTierExternal {
		t.Errorf("a copied record is labelled %q, want %q", got, datasource.TrustTierExternal)
	}
}

// The wire record is what a client parses. The label is set where a record
// becomes tool output, rather than at each tool.
func TestEveryToolsRecordCarriesItsProvenanceLabel(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		authoritative bool
		want          string
	}{
		"a record this installation holds": {true, datasource.TrustTierAuthoritative},
		"a copy of another system's":       {false, datasource.TrustTierExternal},
	} {
		got := newWireRecord(context.Background(), datasource.Record{
			Ref:       datasource.EntityRef{Type: datasource.EntityContact, ID: ids.NewV7()},
			Freshness: datasource.FreshnessInfo{Authoritative: tc.authoritative},
		})
		if got.TrustTier != tc.want {
			t.Errorf("%s is labelled %q, want %q", name, got.TrustTier, tc.want)
		}
	}
}
