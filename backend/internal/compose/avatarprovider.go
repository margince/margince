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
	"path"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/webread"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// googlePictureHostSuffix is the one host family a picture is fetched from. Not any URL a
// token names: the fetch would reach wherever a claim pointed it.
const googlePictureHostSuffix = ".googleusercontent.com"

// installAvatarPictures binds the picture routes, and the sign-in's and Google connect's fill,
// to the options' object store. Without one the routes answer 501 and nothing fills.
func installAvatarPictures(s *Server, pool *pgxpool.Pool, log *slog.Logger) {
	pictures := newAvatarPictures(InstallationDB(pool), s.blob, webread.New(), log)
	s.avatarHandlers = avatarHandlers{pictures: pictures}
	if s.blob != nil {
		s.authHandlers = s.WithProviderPictureFill(pictures.FillFromPictureURL)
		s.picture = newConnectPicture(pictures.users, pictures.FillFromPictureURL)
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
	fillable, current, err := p.users.ProviderMayFill(ctx, user)
	if err != nil || !fillable {
		return err
	}
	source, err := p.fetchPicture(ctx, pictureURL)
	if err != nil {
		return err
	}
	png, err := normalizeAvatar(source)
	if err != nil {
		return fmt.Errorf("compose: normalizing a seat's Google picture: %w", err)
	}
	digest := sha256.Sum256(png)
	revision := fmt.Sprintf("google-%x-", digest[:8])
	if current != nil && strings.HasPrefix(path.Base(*current), revision) {
		return nil
	}
	// Unique per attempt: a key a later fill reused could be the one a release just deleted.
	key := avatarKey(ws, user, revision+ids.NewV7().String())
	if err := p.putPicture(ctx, key, png); err != nil {
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

// fillContext binds what a write needs and a sign-in callback lacks. That is the workspace,
// and the system principal the fill is admitted as.
func (p *avatarPictures) fillContext(ctx context.Context) (context.Context, ids.WorkspaceID, error) {
	ws, err := p.db.Workspace(ctx)
	if err != nil {
		return ctx, ids.WorkspaceID{}, err
	}
	ctx = principal.WithWorkspaceID(ctx, ws.UUID)
	return principal.SystemActing(ctx, "system:provider-picture"), ws, nil
}

// The picture URL is personal data, and webread's errors carry it, so none of them is wrapped.
var (
	errPictureRobots      = errors.New("compose: the Google picture host's robots.txt refuses this fetch")
	errPictureRedirect    = errors.New("compose: the Google picture host redirected off its own hosts")
	errPictureUnreachable = errors.New("compose: the Google picture host could not be reached")
)

// fetchPicture holds every redirect hop to the host family the first URL was checked against.
func (p *avatarPictures) fetchPicture(ctx context.Context, pictureURL string) ([]byte, error) {
	ctx = webread.WithRedirectGuard(ctx, func(hop *url.URL) error {
		if googlePictureURL(hop.String()) != nil {
			return errPictureRedirect
		}
		return nil
	})
	source, _, err := p.fetch.FetchAsset(ctx, pictureURL)
	var status *webread.StatusError
	switch {
	case err == nil:
		return source, nil
	case errors.As(err, &status):
		return nil, fmt.Errorf("compose: the Google picture host answered %d", status.Status)
	case errors.Is(err, webread.ErrRobotsDisallowed):
		return nil, errPictureRobots
	case errors.Is(err, errPictureRedirect):
		return nil, errPictureRedirect
	default:
		return nil, errPictureUnreachable
	}
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
