// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"log/slog"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ProviderPictureFill stores a login provider's picture as a seat's own. Compose supplies
// it: the fetch and the object store are not this module's.
type ProviderPictureFill func(ctx context.Context, user ids.UserID, pictureURL string) error

// PictureSource answers which picture URL a fill stores, "" for none. It may ask the provider.
type PictureSource func(ctx context.Context) (string, error)

// providerPictureTimeout bounds one detached fill: a lookup, a fetch, a decode, a put and
// a row write. A stuck picture host must not pin a goroutine per sign-in.
const providerPictureTimeout = 30 * time.Second

// WithProviderPictureFill binds the fill a federated sign-in offers its picture to.
func (h Handlers) WithProviderPictureFill(fill ProviderPictureFill) Handlers {
	if h.svc != nil {
		h.svc.pictureFill = fill
	}
	return h
}

func (s *Service) offerProviderPicture(ctx context.Context, user ids.UserID, pictureURL string) {
	if pictureURL == "" {
		return
	}
	OfferProviderPicture(ctx, user, func(context.Context) (string, error) { return pictureURL, nil }, s.pictureFill)
}

// OfferProviderPicture runs the source and the fill off the caller's path. A slow provider
// costs the seat its picture, never the member their sign-in or connect.
func OfferProviderPicture(ctx context.Context, user ids.UserID, source PictureSource, fill ProviderPictureFill) {
	if fill == nil || source == nil || user.IsZero() {
		return
	}
	go func() {
		defer recoverDetached("filling a seat's picture from its provider")
		fillCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), providerPictureTimeout)
		defer cancel()
		pictureURL, err := source(fillCtx)
		if err == nil && pictureURL != "" {
			err = fill(fillCtx, user, pictureURL)
		}
		if err != nil {
			slog.WarnContext(fillCtx, "a provider picture was not stored", "user_id", user, "err", err)
		}
	}()
}
