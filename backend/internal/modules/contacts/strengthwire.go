// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"math"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// StrengthToWire renders a strength result onto the contract's shared shape,
// which the contact route, the company route and the company view all answer.
// factors.direction has no domain field, so it is recomputed from the counts.
func StrengthToWire(rs RelationshipStrength, now time.Time) crmcontracts.RelationshipStrength {
	inbound, outbound := rs.Inbound90d, rs.Outbound90d
	direction := 0.0
	if directed := inbound + outbound; directed > 0 {
		direction = 1 - math.Abs(float64(inbound-outbound))/float64(directed)
	}
	contributing := make([]openapi_types.UUID, len(rs.ContributingIDs))
	for i, activityID := range rs.ContributingIDs {
		contributing[i] = openapi_types.UUID(activityID.UUID)
	}
	computedAt := now
	wire := crmcontracts.RelationshipStrength{
		Score:                   rs.Strength,
		Bucket:                  StrengthBucketToWire(rs.Bucket),
		LastInteraction:         rs.LastInteraction,
		ComputedAt:              &computedAt,
		Inbound90d:              &inbound,
		Outbound90d:             &outbound,
		ContributingActivityIds: &contributing,
	}
	wire.Factors.Recency = float32(rs.Recency)
	wire.Factors.Frequency = float32(rs.Frequency)
	wire.Factors.Reciprocity = float32(rs.Reciprocity)
	wire.Factors.Direction = float32(direction)
	return wire
}

// StrengthBucketToWire types the domain's display bucket for the contract. A
// value the kernel never emits reads as none, not as a value the enum lacks.
func StrengthBucketToWire(bucket string) crmcontracts.RelationshipStrengthBucket {
	switch bucket {
	case relstrength.BucketWeak:
		return crmcontracts.RelationshipStrengthBucketWeak
	case relstrength.BucketModerate:
		return crmcontracts.RelationshipStrengthBucketModerate
	case relstrength.BucketStrong:
		return crmcontracts.RelationshipStrengthBucketStrong
	default:
		return crmcontracts.RelationshipStrengthBucketNone
	}
}
