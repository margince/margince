// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A seat's picture from its Google account. Every caller that holds a provider
// picture URL takes this one path, so no two of them store it two ways.

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/webread"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// googlePictureHostSuffix is the one host family a picture is fetched from. Not any URL a
// token names: the fetch would reach wherever a claim pointed it.
const googlePictureHostSuffix = ".googleusercontent.com"

// installAvatarPictures binds the picture routes and the sign-in's fill to the options' object
// store. Without one the routes answer 501 and a sign-in fills nothing.
func installAvatarPictures(s *Server, pool *pgxpool.Pool, log *slog.Logger) {
	pictures := newAvatarPictures(InstallationDB(pool), s.blob, webread.New(), log)
	s.avatarHandlers = avatarHandlers{pictures: pictures}
	if s.blob != nil {
		s.authHandlers = s.WithProviderPictureFill(pictures.FillFromPictureURL)
	}
}

// FillFromPictureURL stores a Google account's picture as the seat's own, unless its member
// uploaded or deleted one. It reuses the logo resolve's guarded fetch and the upload's normalize.
func (p *avatarPictures) FillFromPictureURL(ctx context.Context, user ids.UserID, pictureURL string) error {
	if p == nil || p.blob == nil {
		return nil
	}
	if err := googlePictureURL(pictureURL); err != nil {
		return err
	}
	ctx, ws, err := p.fillContext(ctx)
	if err != nil {
		return err
	}
	source, _, err := p.fetch.FetchAsset(ctx, pictureURL)
	if err != nil {
		return fmt.Errorf("compose: fetching a seat's Google picture: %w", err)
	}
	png, err := normalizeAvatar(source)
	if err != nil {
		return fmt.Errorf("compose: normalizing a seat's Google picture: %w", err)
	}
	digest := sha256.Sum256(png)
	key := avatarKey(ws, user, fmt.Sprintf("google-%x", digest[:8]))
	current, err := p.users.SeatAvatarKey(ctx, user)
	if err == nil && current == key {
		return nil
	}
	if err != nil && !errors.Is(err, apperrors.ErrNotFound) {
		return err
	}
	if err := p.store(ctx, key, png); err != nil {
		return err
	}
	written, superseded, err := p.users.FillAvatarFromProvider(ctx, user, key)
	if err != nil {
		return err
	}
	if !written {
		superseded = &key
	}
	p.release(ctx, user, superseded)
	return nil
}

// fillContext binds what a write needs and a sign-in callback lacks: the
// workspace, the system principal the fill is admitted as, and a correlation id.
func (p *avatarPictures) fillContext(ctx context.Context) (context.Context, ids.WorkspaceID, error) {
	ws, err := p.db.Workspace(ctx)
	if err != nil {
		return ctx, ids.WorkspaceID{}, err
	}
	ctx = principal.WithWorkspaceID(ctx, ws.UUID)
	ctx = principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalSystem, ID: "system:provider-picture"})
	if _, traced := principal.CorrelationID(ctx); !traced {
		ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	}
	return ctx, ws, nil
}

// googlePictureURL refuses anything but an https URL on a Google picture host.
func googlePictureURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil ||
		(parsed.Port() != "" && parsed.Port() != "443") ||
		!strings.HasSuffix(strings.ToLower(parsed.Hostname()), googlePictureHostSuffix) {
		return errors.New("compose: a provider picture is fetched only from an https Google picture host")
	}
	return nil
}
