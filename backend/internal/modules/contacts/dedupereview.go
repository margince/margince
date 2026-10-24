// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The review lanes' answer: every contact a lane paired the candidate with,
// never only the first. Detection, the writers that file pairs and the nightly
// re-check ask the same pair predicates (contactNamesCollide and
// addressVariantPair). So the re-check keeps a filed pair until its evidence is
// gone.

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ReviewMatch is one contact a review lane paired the candidate with, and the
// evidence the pair is filed with.
type ReviewMatch struct {
	Decision   DedupeDecision
	ContactID  ids.ContactID
	Confidence float64
	// CandidateEmail and IncumbentEmail are the two addresses the
	// address-variant lane paired, and empty for the other lanes.
	CandidateEmail string
	IncumbentEmail string
}

// reviewContact runs the lanes that never route: the address variant, then
// the fuzzy tier and the name-collision lane. DedupeContact reaches it once the
// exact lanes are silent. An edit re-check reaches it directly, because the
// edited contact's own keys would answer every exact lane.
func reviewContact(ctx context.Context, tx pgx.Tx, c ContactCandidate) (ContactResolution, error) {
	var reviews []ReviewMatch
	// Before the name lanes, and without a name. A contact captured as "Ab"
	// from its address is the usual incumbent of a tagged address.
	if c.QueueNameCollisions {
		variants, err := addressVariantContacts(ctx, tx, c)
		if err != nil {
			return ContactResolution{}, err
		}
		reviews = variants
	}
	// A nameless captured contact never fuzzy-matches: with no name there is
	// nothing to score, and company_match alone would collide every colleague
	// onto one record.
	if NormalizeContactName(c.FullName) != "" {
		named, err := fuzzyContact(ctx, tx, c)
		if err != nil {
			return ContactResolution{}, err
		}
		for _, match := range named.Reviews {
			if !slices.ContainsFunc(reviews, func(r ReviewMatch) bool { return r.ContactID == match.ContactID }) {
				reviews = append(reviews, match)
			}
		}
	}
	var subject *ids.UUID
	if c.ExcludeID != nil {
		subject = &c.ExcludeID.UUID
	}
	reviews, err := newRivals(ctx, tx, entityContact, subject, reviews,
		func(r ReviewMatch) ids.UUID { return r.ContactID.UUID })
	if err != nil {
		return ContactResolution{}, err
	}
	return resolutionOf(reviews), nil
}

// resolutionOf names the first review match as the resolution's decision, so
// a caller that reads one contact reads the one ranked first.
func resolutionOf(reviews []ReviewMatch) ContactResolution {
	if len(reviews) == 0 {
		return ContactResolution{Decision: DecisionNoMatch}
	}
	first := reviews[0]
	return ContactResolution{
		Decision: first.Decision, ContactID: first.ContactID, Confidence: first.Confidence, Reviews: reviews,
	}
}

// rankedReviews orders the fuzzy matches ahead of the name collisions, each by
// score and then by the lowest contact id. The order is total, so the queue
// does not shuffle between runs.
func rankedReviews(scored map[ids.ContactID]ReviewMatch) []ReviewMatch {
	ranked := make([]ReviewMatch, 0, len(scored))
	for _, match := range scored {
		ranked = append(ranked, match)
	}
	slices.SortFunc(ranked, func(a, b ReviewMatch) int {
		if fa, fb := a.Decision == DecisionFuzzyReview, b.Decision == DecisionFuzzyReview; fa != fb {
			if fa {
				return -1
			}
			return 1
		}
		if a.Confidence != b.Confidence {
			return cmp.Compare(b.Confidence, a.Confidence)
		}
		return cmp.Compare(a.ContactID.String(), b.ContactID.String())
	})
	return ranked
}

// contactNamesCollide is the name-collision lane's pair test: the two names
// share the contact-name key or the digraph key ("Björn" and "Bjoern").
func contactNamesCollide(a, b string) bool {
	if key := NormalizeContactName(a); key != "" && key == NormalizeContactName(b) {
		return true
	}
	key := digraphNameKey(a)
	return key != "" && key == digraphNameKey(b)
}

// firstVisibleReview is the review match a caller may be shown: the first one
// whose contact the caller can see, or nil. The lanes search the whole
// workspace, so a reader that named or reacted to a hidden match would tell the
// caller that record exists. A caller without the contact read grant sees none.
func firstVisibleReview(ctx context.Context, tx pgx.Tx, m ContactResolution) (*ReviewMatch, error) {
	if len(m.Reviews) == 0 {
		return nil, nil //nolint:nilnil // no review match is the common answer, not a failure.
	}
	if err := auth.Require(ctx, contactEntity, principal.ActionRead); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return nil, nil //nolint:nilnil // a caller who reads no contact is shown none.
		}
		return nil, err
	}
	for i := range m.Reviews {
		visible, err := auth.VisibleTo(ctx, tx, contactEntity, m.Reviews[i].ContactID.UUID)
		if err != nil {
			return nil, fmt.Errorf("contacts: probing a review match's visibility: %w", err)
		}
		if visible {
			return &m.Reviews[i], nil
		}
	}
	return nil, nil //nolint:nilnil // every match is hidden from this caller.
}
