// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"slices"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// marginTierMask is the seat these tests are about: one that reads the partner
// register without reading the commercial terms on it.
func marginTierMask() principal.FieldMask {
	return principal.FieldMask{Object: "partner", Field: "margin_tier", Condition: principal.MaskAlways}
}

// partnerReader is a row-scoped seat, because a principal reading every row is
// withheld nothing and a fixture on that scope would assert nothing.
func partnerReader(masks ...principal.FieldMask) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman,
		Permissions: principal.Permissions{
			RowScope:   principal.RowScopeTeam,
			Objects:    map[string]principal.ObjectGrant{"partner": {Read: true}, "company": {Read: true}},
			FieldMasks: masks,
		},
	})
}

func partnerPage(tiers ...crmcontracts.PartnerMarginTier) []crmcontracts.Partner {
	page := make([]crmcontracts.Partner, 0, len(tiers))
	for _, tier := range tiers {
		page = append(page, crmcontracts.Partner{
			CompanyId: openapi_types.UUID(ids.NewV7()), MarginTier: &tier,
		})
	}
	return page
}

// The wire contract in one row: the tier goes out null and the row names it, so
// a reader tells a withheld tier from a partner nobody has tiered yet.
//
// The pass is handed no transaction because it needs none — `partner` carries
// no owner, so no mask on it can be conditioned on write authority and the
// answer costs no statement.
func TestAMaskedSeatReadsAPartnerWithoutItsMarginTier(t *testing.T) {
	t.Parallel()
	page := partnerPage(crmcontracts.PartnerMarginTierTier115, crmcontracts.PartnerMarginTierTier220)

	if err := maskPartners(partnerReader(marginTierMask()), nil, page); err != nil {
		t.Fatalf("masking a partner page: %v", err)
	}
	for i, p := range page {
		if p.MarginTier != nil {
			t.Errorf("row %d kept its margin tier: %v", i, *p.MarginTier)
		}
		if p.MaskedFields == nil || !slices.Contains(*p.MaskedFields, "margin_tier") {
			t.Errorf("row %d named %v, want margin_tier — a withheld null must say it was withheld",
				i, p.MaskedFields)
		}
	}
}

// The other direction, or the test above would pass over a pass that withheld
// the tier from every seat.
func TestASeatWithNoMarginMaskReadsTheTier(t *testing.T) {
	t.Parallel()
	page := partnerPage(crmcontracts.PartnerMarginTierTier325)

	if err := maskPartners(partnerReader(), nil, page); err != nil {
		t.Fatalf("masking a partner page: %v", err)
	}
	if page[0].MarginTier == nil || *page[0].MarginTier != crmcontracts.PartnerMarginTierTier325 {
		t.Errorf("margin_tier = %v, want the tier an unmasked seat reads", page[0].MarginTier)
	}
	if page[0].MaskedFields != nil {
		t.Errorf("masked_fields = %v on a row nothing was withheld from", *page[0].MaskedFields)
	}
}

// Ordering by a value is reading it: a page ordered by tiers the caller may not
// see hands them over through the order, so the list refuses the key rather
// than answering in some other order.
func TestAMaskedSeatMayNotOrderThePartnerListByTheTier(t *testing.T) {
	t.Parallel()
	for _, spec := range []string{"margin_tier", "-margin_tier"} {
		var refused *values.ParseError
		err := refuseMaskedPartnerSort(partnerReader(marginTierMask()), &spec)
		if !errors.As(err, &refused) || refused.Code != auth.CodeFieldMasked {
			t.Errorf("sort=%s answered %v, want the masked-field refusal", spec, err)
		}
	}
}

// A sort key nothing withholds is never refused, and neither is a masked column
// for a seat that reads it.
func TestAnUnmaskedSeatOrdersThePartnerListByTheTier(t *testing.T) {
	t.Parallel()
	tier, stage := "margin_tier", "relationship_stage"
	if err := refuseMaskedPartnerSort(partnerReader(), &tier); err != nil {
		t.Errorf("sort=margin_tier for a seat that reads it: %v", err)
	}
	if err := refuseMaskedPartnerSort(partnerReader(marginTierMask()), &stage); err != nil {
		t.Errorf("sort=relationship_stage under a margin mask: %v", err)
	}
}
