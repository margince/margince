// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Every stored picture is this server's own PNG, never the bytes as they arrived.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"log/slog"
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/imagenorm"
	"github.com/margince/margince/backend/internal/platform/storedobjects"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/assetrev"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// avatarEdge is the stored square's side: a high-density screen's largest
// drawn avatar, with nothing left for the browser to upscale.
const avatarEdge = 512

const avatarKind = "avatar"

// Not SVG: markup is a document a photo route has no reason to rasterize. Not GIF: one
// frame of an animation is not the picture somebody chose.
var avatarFormats = map[string]bool{"png": true, "jpeg": true, "webp": true}

// errAvatarFormat is a picture outside avatarFormats, or one that does not decode.
var errAvatarFormat = &httperr.DetailedError{
	Status: http.StatusUnsupportedMediaType, Code: "unsupported_media_type",
	Detail: "the picture is not a PNG, JPEG or WebP image this server can read; save it as one of those and upload it again",
}

type avatarPictures struct {
	db     *database.DB
	users  *identity.Service
	ledger *storedobjects.Ledger
	// blob is nil in a role that stores no objects: every route then answers 501.
	blob  blobstore.Store
	fetch assetFetcher
	log   *slog.Logger
}

func newAvatarPictures(db *database.DB, blob blobstore.Store, fetch assetFetcher, log *slog.Logger) *avatarPictures {
	return &avatarPictures{
		db: db, users: identity.NewServiceFor(db), ledger: storedobjects.NewLedger(db),
		blob: blob, fetch: fetch, log: log,
	}
}

// The zero avatarHandlers answers every route 501: a Server built without New has no pictures.
type avatarHandlers struct{ pictures *avatarPictures }

// normalizeAvatar re-encodes an accepted picture's centred square as this server's own PNG.
func normalizeAvatar(source []byte) ([]byte, error) {
	if _, format, err := image.DecodeConfig(bytes.NewReader(source)); err != nil || !avatarFormats[format] {
		return nil, errAvatarFormat
	}
	img, err := imagenorm.Decode(source)
	if err != nil {
		return nil, errAvatarFormat
	}
	return imagenorm.SquarePNG(centreSquare(img), avatarEdge)
}

// centreSquare is the largest square at the picture's centre, so a portrait or
// landscape photo fills the circle it is drawn in rather than letterboxing.
func centreSquare(img image.Image) image.Image {
	bounds := img.Bounds()
	edge := min(bounds.Dx(), bounds.Dy())
	x0 := bounds.Min.X + (bounds.Dx()-edge)/2
	y0 := bounds.Min.Y + (bounds.Dy()-edge)/2
	cropper, ok := img.(interface {
		SubImage(image.Rectangle) image.Image
	})
	if !ok {
		return img
	}
	return cropper.SubImage(image.Rect(x0, y0, x0+edge, y0+edge))
}

// avatarKey mints where one seat's picture lives. attempt keeps two writers of
// one seat's picture apart.
func avatarKey(ws ids.WorkspaceID, user ids.UserID, attempt string) string {
	return blobstore.WorkspaceKey(ws, avatarKind, user.String()+"/"+attempt+".png")
}

// putPicture declares the key provisional, then puts the bytes. A failed put leaves
// the intent for the sweep, which deletes whatever partial object it left.
func (p *avatarPictures) putPicture(ctx context.Context, key string, png []byte) error {
	if err := p.ledger.Record(ctx, storedobjects.KindAvatar, key); err != nil {
		return err
	}
	return p.blob.Put(ctx, key, bytes.NewReader(png), int64(len(png)), imagenorm.ContentType)
}

// release deletes bytes a write left unreferenced. Detached and logged: the
// write the caller asked for has happened, and the sweep retries a failure.
func (p *avatarPictures) release(ctx context.Context, user ids.UserID, key *string) {
	deleteUnreferencedLogo(ctx, p.blob, p.log, "seat "+user.String()+" picture", key)
}

// UploadMyAvatar implements POST /me/avatar.
func (h avatarHandlers) UploadMyAvatar(w http.ResponseWriter, r *http.Request) {
	p := h.pictures
	if p == nil || p.blob == nil {
		httperr.NotImplemented(w, r, "uploadMyAvatar")
		return
	}
	if err := auth.RequireHuman(r.Context()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	actor, isActor := principal.Actor(r.Context())
	workspace, inWorkspace := principal.WorkspaceID(r.Context())
	if !isActor || actor.Type != principal.PrincipalHuman || !inWorkspace {
		httperr.Write(w, r, apperrors.ErrPermissionDenied)
		return
	}
	source, _, ok := readImageUpload(w, r)
	if !ok {
		return
	}
	png, err := normalizeAvatar(source)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	user := ids.From[ids.UserKind](actor.UserID)
	key := avatarKey(ids.From[ids.WorkspaceKind](workspace), user, ids.NewV7().String())
	if err := p.putPicture(r.Context(), key, png); err != nil {
		httperr.Write(w, r, err)
		return
	}
	// On a failed write the bytes stay: the row may have committed and name
	// them, and the ledger collects them if it did not.
	seat, superseded, err := p.users.SetMyAvatar(r.Context(), key)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	p.release(r.Context(), user, superseded)
	httperr.WriteJSON(w, http.StatusOK, identity.SeatUser(seat))
}

// DeleteMyAvatar implements DELETE /me/avatar.
func (h avatarHandlers) DeleteMyAvatar(w http.ResponseWriter, r *http.Request) {
	p := h.pictures
	if p == nil {
		httperr.NotImplemented(w, r, "deleteMyAvatar")
		return
	}
	seat, removed, err := p.users.RemoveMyAvatar(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	p.release(r.Context(), seat.UserID, removed)
	httperr.WriteJSON(w, http.StatusOK, identity.SeatUser(seat))
}

// GetUserAvatar implements GET /users/{id}/avatar. A matching If-None-Match answers
// before the object store is touched.
func (h avatarHandlers) GetUserAvatar(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, params crmcontracts.GetUserAvatarParams) {
	p := h.pictures
	if p == nil || p.blob == nil {
		httperr.NotImplemented(w, r, "getUserAvatar")
		return
	}
	key, err := p.users.SeatAvatarKey(r.Context(), ids.From[ids.UserKind](ids.UUID(id)))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	revision := identity.AvatarETag(key)
	etag := `"` + revision + `"`
	cacheControl := avatarCacheControl(params.V, revision)
	if httperr.IfNoneMatchHit(r, etag) {
		httperr.WriteNotModified(w, etag, cacheControl)
		return
	}
	body, object, err := p.blob.Get(r.Context(), key)
	if errors.Is(err, blobstore.ErrNotFound) {
		httperr.Write(w, r, apperrors.ErrNotFound)
		return
	}
	if err != nil {
		httperr.Write(w, r, fmt.Errorf("compose: reading a seat's picture: %w", err))
		return
	}
	httperr.StreamOwnImage(w, r, httperr.OwnImage{
		ContentType: imagenorm.ContentType, ETag: etag, CacheControl: cacheControl,
		Size: object.Size, Body: body,
	}, "seat picture "+id.String())
}

// avatarCacheControl is immutable only for the URL that names these bytes. A stale
// token cached for a day would show the replaced picture at that URL long after.
func avatarCacheControl(asked *string, revision string) string {
	if asked != nil && *asked == revision {
		return assetrev.CacheControl
	}
	return "private, no-cache"
}
