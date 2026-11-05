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

// providerPictureTimeout bounds one detached fill: a fetch, a decode, a put and
// a row write. A stuck picture host must not pin a goroutine per sign-in.
const providerPictureTimeout = 30 * time.Second

// WithProviderPictureFill binds the fill a federated sign-in offers its picture to.
func (h Handlers) WithProviderPictureFill(fill ProviderPictureFill) Handlers {
	if h.svc != nil {
		h.svc.pictureFill = fill
	}
	return h
}

// offerProviderPicture runs the fill off the sign-in's path: a slow picture host costs the
// seat its picture, never the member their sign-in. The next sign-in tries again.
func (s *Service) offerProviderPicture(ctx context.Context, user ids.UserID, pictureURL string) {
	if s.pictureFill == nil || pictureURL == "" || user.IsZero() {
		return
	}
	fill := s.pictureFill
	go func() {
		defer recoverDetached("filling a seat's picture from its sign-in")
		fillCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), providerPictureTimeout)
		defer cancel()
		if err := fill(fillCtx, user, pictureURL); err != nil {
			slog.WarnContext(fillCtx, "a sign-in's provider picture was not stored", "user_id", user, "err", err)
		}
	}()
}
