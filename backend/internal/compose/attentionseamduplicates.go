// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The dedupe queue's side of the day's surface.
//
// It sits apart from the rest of the seam because it is the one adapter that
// asks the store two different questions about the same page: which pairs are
// open, and which of those a merge would actually accept. Everything else the
// seam binds is a single read.

import (
	"context"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// attentionDuplicates reads the dedupe queue through the people store, which
// applies the both-sides-visible rule to the page and the count alike.
type attentionDuplicates struct{ store *people.Store }

func (d attentionDuplicates) OpenCandidates(ctx context.Context, limit int) ([]attention.DuplicatePair, error) {
	rows, _, err := d.store.ListDedupeCandidates(ctx, people.DedupeQueueInput{Limit: limit})
	if err != nil {
		return nil, err
	}
	pairs := make([]attention.DuplicatePair, 0, len(rows))
	for _, row := range rows {
		pairs = append(pairs, attention.DuplicatePair{
			ID:         row.ID,
			EntityType: row.EntityType,
			Confidence: row.Confidence,
			LeftID:     row.LeftID,
			RightID:    row.RightID,
			Evidence:   comparisons(ctx, row.ID, row.Evidence),
		})
	}
	return pairs, nil
}

// DescribeMany names records of one entity type, under the reader's own scope.
//
// The store read carries the same object grant and the same row scope the
// ordinary get applies — the pair's own row is not permission to read what it
// points at — and asks the scope of the whole set at once, which is what turns
// a page of ten pairs from twenty transactions into three.
func (d attentionDuplicates) DescribeMany(
	ctx context.Context, entityType string, rowIDs []ids.UUID,
) (map[ids.UUID]attention.RecordFace, error) {
	if entityType != flipObjectPerson && entityType != flipObjectOrganization && entityType != flipObjectLead {
		return nil, apperrors.ErrNotFound
	}
	described, err := d.store.DescribeForMerge(ctx, entityType, rowIDs)
	if err != nil {
		return nil, err
	}
	faces := make(map[ids.UUID]attention.RecordFace, len(described))
	for id, row := range described {
		faces[id] = attention.RecordFace{
			Label:        row.Label,
			Detail:       row.Detail,
			CreatedAt:    &row.CreatedAt,
			RelatedCount: row.RelatedCount,
		}
	}
	return faces, nil
}

// DecidableSubset answers which of these records the reader could change, which
// is what decides whether the card offers a verb at all.
//
// It goes through the same store as the naming read, so the card's offer and
// the disposition endpoint's refusal are two readings of one authority rather
// than two rules that can drift.
func (d attentionDuplicates) DecidableSubset(
	ctx context.Context, entityType string, rowIDs []ids.UUID,
) (map[ids.UUID]bool, error) {
	if entityType != flipObjectPerson && entityType != flipObjectOrganization && entityType != flipObjectLead {
		return nil, apperrors.ErrNotFound
	}
	return d.store.DecidableForMerge(ctx, entityType, rowIDs)
}

func (d attentionDuplicates) CountOpen(ctx context.Context) (int, error) {
	return d.store.CountOpenDedupeCandidates(ctx)
}

// SettleablePairs answers which pairs a merge would actually accept.
//
// Only organizations have a refusal beyond authority: two companies each
// carrying live work do not combine (PROJ-LIFE-4), because the merged company
// would be running the same body of work twice or two different ones, and
// nothing in the data says which. People and leads carry no such rule, so they
// are settleable by construction rather than by a query nobody needs.
//
// It reads through the same store the merge refuses from, so the card's offer
// and the write's refusal are two readings of one rule — the property the
// authority answer beside it already has.
func (d attentionDuplicates) SettleablePairs(
	ctx context.Context, pairs []attention.DuplicatePair,
) (map[ids.UUID]bool, error) {
	settleable := make(map[ids.UUID]bool, len(pairs))
	var companies []ids.UUID
	for _, pair := range pairs {
		settleable[pair.ID] = true
		if pair.EntityType == flipObjectOrganization {
			companies = append(companies, pair.LeftID, pair.RightID)
		}
	}
	if len(companies) == 0 {
		return settleable, nil
	}
	carrying, err := d.store.OrganizationsCarryingLiveProjects(ctx, companies)
	if err != nil {
		return nil, err
	}
	for _, pair := range pairs {
		if pair.EntityType == flipObjectOrganization &&
			carrying[pair.LeftID] && carrying[pair.RightID] {
			settleable[pair.ID] = false
		}
	}
	return settleable, nil
}
