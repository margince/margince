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
	// resolves contacts only, so a lead-only address carried NO header at all.
	token, ok, err := a.store.WithdrawalTokenForEmail(ctx, recipientEmail, purposeKey)
	if err != nil || ok {
		return token, ok, err
	}
	// A live credential already covers this address, so its link is still in a
	// message somewhere and still works. The preference token is the fallback
	// that gives THIS message a header of its own.
	return a.store.PreferenceTokenForEmail(ctx, recipientEmail)
}

// ManageToken answers the preference token the preference centre resolves.
//
// ALWAYS A PREFERENCE TOKEN, never the withdrawal credential the stop links
// carry. The two are different capabilities: a withdrawal credential stops mail
// and reads nothing, which is what lets it outlive the message it was sent in.
// Handing it to the manage link left every "Manage preferences" link resolving
// nothing — the page could not read the subject their own purposes, because the
// credential it was given was never meant to.
//
// A recipient this mint cannot resolve — a lead-only address holds no contact
// record — gets no manage token and the link falls back to the stop credential,
// which draws the withdraw-only page. That page is honest about what it can do;
// a dead preference link is not.
func (a preferenceLinkAdapter) ManageToken(ctx context.Context, recipientEmail string) (string, bool, error) {
	return a.store.PreferenceTokenForEmail(ctx, recipientEmail)
}
