// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package company360

import (
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// pageInfo carries a store page onto the wire shape.
func pageInfo(p storekit.Page) crmcontracts.PageInfo {
	info := crmcontracts.PageInfo{HasMore: p.HasMore}
	if p.NextCursor != "" {
		info.NextCursor = &p.NextCursor
	}
	return info
}

// accountStrengthToWire adds the two account-only facts to the shared
// shape: whose relationship carries the score, and how many contacts it
// was chosen from. The shared half comes from compose.StrengthToWire, so
// a bucket rename is made once for both the account roll-up and the
// per-contact routes.
func accountStrengthToWire(account contacts.AccountStrength, now time.Time) *crmcontracts.CompanyStrength {
	base := contacts.StrengthToWire(account.RelationshipStrength, now)
	out := crmcontracts.CompanyStrength{
		Score:                   base.Score,
		Bucket:                  crmcontracts.CompanyStrengthBucket(base.Bucket),
		Factors:                 base.Factors,
		ComputedAt:              base.ComputedAt,
		LastInteraction:         base.LastInteraction,
		Inbound90d:              base.Inbound90d,
		Outbound90d:             base.Outbound90d,
		ContributingActivityIds: base.ContributingActivityIds,
		ContactCount:            account.ContactCount,
	}
	if account.ContributorContactID != nil {
		v := openapi_types.UUID(account.ContributorContactID.UUID)
		out.ContributorContactId = &v
	}
	return &out
}
