// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A Google connect of the seat's own account also refreshes the seat's picture.
// A shared mailbox's picture is nobody's face, so any other account offers none.

import (
	"context"
	"log/slog"
	"strings"

	"github.com/margince/margince/backend/internal/modules/capture/googleconn"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// connectPicture is what a connect needs to offer its account's picture. The zero value
// offers none: a role without an object store keeps no pictures.
type connectPicture struct {
	// seatEmail is the granting human's own sign-in address, read under their principal.
	seatEmail func(ctx context.Context) (string, error)
	userinfo  googleconn.Userinfo
	fill      identity.ProviderPictureFill
}

// newConnectPicture binds the offer to the seat directory and the picture seam.
func newConnectPicture(users *identity.Service, fill identity.ProviderPictureFill) connectPicture {
	return connectPicture{
		seatEmail: func(ctx context.Context) (string, error) {
			_, email, err := users.ActorIdentity(ctx)
			return email, err
		},
		userinfo: googleconn.NewUserinfo(nil, ""),
		fill:     fill,
	}
}

// offerConnectPicture hands a stored connect's picture to the seam, off the callback's path.
func (h connectorHandlers) offerConnectPicture(ctx context.Context, user ids.UUID, consent *consentCode, auth connector.Auth) {
	if source, ok := h.connectPictureSource(ctx, consent, auth); ok {
		identity.OfferProviderPicture(ctx, ids.From[ids.UserKind](user), source, h.picture.fill)
	}
}

// connectPictureSource answers the lookup a connect may offer: only a grant that holds the
// profile scope, and only for the account the sealed bundle names.
func (h connectorHandlers) connectPictureSource(ctx context.Context, consent *consentCode, auth connector.Auth) (identity.PictureSource, bool) {
	p := h.picture
	if p.fill == nil || p.seatEmail == nil || !consent.granted(googleconn.ProfileScope) || consent.grant.AccessToken == "" {
		return nil, false
	}
	account, err := googleconn.AccountLabel(auth)
	if err != nil {
		slog.WarnContext(ctx, "connector callback: the sealed grant names no account to take a picture from", "err", err)
		return nil, false
	}
	accessToken := consent.grant.AccessToken
	return func(ctx context.Context) (string, error) {
		own, err := p.seatEmail(ctx)
		if err != nil {
			return "", err
		}
		if !sameMailbox(own, account) {
			return "", nil
		}
		return p.userinfo.Picture(ctx, accessToken)
	}, true
}

// sameMailbox reports whether the Google account is the seat's own sign-in address.
func sameMailbox(seat, account string) bool {
	seat, account = strings.TrimSpace(seat), strings.TrimSpace(account)
	return seat != "" && strings.EqualFold(seat, account)
}
