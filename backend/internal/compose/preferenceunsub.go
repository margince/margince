// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// preferenceLinkAdapter satisfies activities.UnsubscribeLinker over the
// consent module — the cross-module edge of the send path's RFC 8058
// header, injected here so activities never imports its sibling. A locked
// (transactional) purpose carries no unsubscribe surface; every other
// address resolves to its lazily-minted preference token.

import (
	"context"

	"github.com/margince/margince/backend/internal/modules/consent"
)

type preferenceLinkAdapter struct {
	store *consent.Store
}

func (a preferenceLinkAdapter) UnsubscribeToken(ctx context.Context, recipientEmail, purposeKey string) (string, bool, error) {
	if consent.LockedPurpose(purposeKey) {
		return "", false, nil
	}
	// THE WITHDRAWAL CREDENTIAL FIRST, because this token goes into a message
	// that outlives it. A preference token slides 30 days and is revoked on the
	// next send's rotation, so the header on an older message stops working and
	// the recipient is told their unsubscribe link is invalid — which is the
	// one thing RFC 8058 exists to prevent.
	//
	// It also reaches recipients the preference token cannot: that mint
	// resolves persons only, so a lead-only address carried NO header at all.
	token, ok, err := a.store.WithdrawalTokenForEmail(ctx, recipientEmail, purposeKey)
	if err != nil || ok {
		return token, ok, err
	}
	// A live credential already covers this address, so its link is still in a
	// message somewhere and still works. The preference token is the fallback
	// that gives THIS message a header of its own.
	return a.store.PreferenceTokenForEmail(ctx, recipientEmail)
}
