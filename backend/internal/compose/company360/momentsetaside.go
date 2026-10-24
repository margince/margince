// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package company360

// Setting the account's moment aside.
//
// The card's "Not now" lands in suggestion_dismissal, the store the advice rows
// beside it already use. Both are this reader saying "not this, not now" about
// one account. Both are keyed on what the item fired on, and re-arm when that
// evidence moves. A second store would be a second answer to "what did this
// reader put away here?".

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/owedwork"
)

// withoutSetAsidePromises drops the promises whose card this reader put away.
//
// Only a human's own set-asides apply, as on the contact page. A passport
// carries its granting human's id, and an agent reading the account must see
// every promise, including the ones a human put away.
func (s *Service) withoutSetAsidePromises(
	ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, now time.Time, items []owedwork.Item,
) ([]owedwork.Item, error) {
	if len(items) == 0 {
		return items, nil
	}
	if err := auth.RequireHuman(ctx); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return items, nil
		}
		return nil, err
	}
	fingerprints := make([]string, len(items))
	for i, item := range items {
		fingerprints[i] = promiseFingerprint(now, item)
	}
	dismissed, err := s.dismissedFingerprints(ctx, tx, companyID, fingerprints)
	if err != nil {
		return nil, err
	}
	kept := make([]owedwork.Item, 0, len(items))
	for i, item := range items {
		if !dismissed[fingerprints[i]] {
			kept = append(kept, item)
		}
	}
	return kept, nil
}

// raisesMoment reports whether this fingerprint is one of the cards the
// account's open promises would draw for this caller. Every open promise
// counts, not only the one on top. A card set aside a moment ago is no longer
// on top, and its undo needs the dismissal to have been stored.
func (s *Service) raisesMoment(
	ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, now time.Time, within *ids.ProjectID, fingerprint string,
) (bool, error) {
	items, _, err := s.openPromises(ctx, tx, companyID, now, AssembleOptions{ProjectID: within})
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		// A caller without the activity grant is never shown the card.
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, item := range items {
		if promiseFingerprint(now, item) == fingerprint {
			return true, nil
		}
	}
	// Not yet on a "Done?" question puts that question away, keyed on its mail.
	asked, err := doneQuestions(ctx, tx, now, momentCopyFor("en"), items)
	if err != nil {
		return false, err
	}
	for _, question := range asked {
		if question.EvidenceFingerprint == fingerprint {
			return true, nil
		}
	}
	return false, nil
}

// promiseFingerprint is the evidence fingerprint the card for this promise
// carries, read off the drawn card rather than derived a second time.
func promiseFingerprint(now time.Time, item owedwork.Item) string {
	return momentCopyFor("en").promiseCard(now, item, false).EvidenceFingerprint
}
