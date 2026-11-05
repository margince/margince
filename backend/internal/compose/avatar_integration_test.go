// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A seat's picture through its routes and the sign-in's fill: every colleague streams it,
// and nothing a write replaces is left behind.

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const googlePicture = "https://lh3.googleusercontent.com/a/rep-one"

type avatarFixture struct {
	e     *integration.Env
	blob  blobstore.Store
	site  *assetSite
	h     avatarHandlers
	me    context.Context
	other context.Context
}

func newAvatarFixture(t *testing.T) avatarFixture {
	t.Helper()
	e := integration.Setup(t)
	blob := blobstore.NewMemory()
	site := &assetSite{assets: map[string][]byte{googlePicture: encodedPicture(t, pngOf, halvesImage(60, 60))}}
	pictures := newAvatarPictures(e.DB(), blob, site, slog.New(slog.DiscardHandler))
	return avatarFixture{
		e: e, blob: blob, site: site, h: avatarHandlers{pictures: pictures},
		me:    e.As(e.Rep1, nil, integration.RepPerms),
		other: e.As(e.Rep2, nil, integration.RepPerms),
	}
}

func (f avatarFixture) upload(ctx context.Context, t *testing.T, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	body, contentType := multipartWith(t, "file", "me.png", content)
	req := httptest.NewRequest(http.MethodPost, "/v1/me/avatar", body).WithContext(ctx)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	f.h.UploadMyAvatar(rec, req)
	return rec
}

func (f avatarFixture) stream(ctx context.Context, user ids.UUID, revision, etag string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/v1/users/"+user.String()+"/avatar", nil).WithContext(ctx)
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	rec := httptest.NewRecorder()
	f.h.GetUserAvatar(rec, req, crmcontracts.Id(user), crmcontracts.GetUserAvatarParams{V: &revision})
	return rec
}

func (f avatarFixture) storedKey(t *testing.T, user ids.UUID) *string {
	t.Helper()
	var key *string
	if err := f.e.Pool.QueryRow(context.Background(),
		`SELECT avatar_object_key FROM app_user WHERE id = $1`, user).Scan(&key); err != nil {
		t.Fatalf("reading the seat's key: %v", err)
	}
	return key
}

func (f avatarFixture) objectExists(t *testing.T, key string) bool {
	t.Helper()
	body, _, err := f.blob.Get(context.Background(), key)
	if errors.Is(err, blobstore.ErrNotFound) {
		return false
	}
	if err != nil {
		t.Fatalf("reading the object store: %v", err)
	}
	if err := body.Close(); err != nil {
		t.Fatal(err)
	}
	return true
}

func decodeUser(t *testing.T, rec *httptest.ResponseRecorder) crmcontracts.User {
	t.Helper()
	var user crmcontracts.User
	if err := json.Unmarshal(rec.Body.Bytes(), &user); err != nil {
		t.Fatalf("decoding the answer: %v: %s", err, rec.Body)
	}
	return user
}

func TestAColleagueStreamsTheUploadedPictureUnderItsRevision(t *testing.T) {
	f := newAvatarFixture(t)
	rec := f.upload(f.me, t, encodedPicture(t, pngOf, halvesImage(80, 40)))
	if rec.Code != http.StatusOK {
		t.Fatalf("upload = %d: %s", rec.Code, rec.Body)
	}
	url := decodeUser(t, rec).AvatarUrl
	if url == nil {
		t.Fatal("the upload's answer carries no avatar_url")
	}

	_, revision, _ := strings.Cut(*url, "?v=")
	got := f.stream(f.other, f.e.Rep1, revision, "")
	if got.Code != http.StatusOK || got.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("a colleague streaming the picture = %d %s", got.Code, got.Header().Get("Content-Type"))
	}
	etag := got.Header().Get("ETag")
	if got.Header().Get("Cache-Control") != "private, max-age=86400, immutable" || etag == "" {
		t.Errorf("headers: Cache-Control %q, ETag %q", got.Header().Get("Cache-Control"), etag)
	}
	if `"`+revision+`"` != etag {
		t.Errorf("avatar_url %s does not carry the revision its stream names, %s", *url, etag)
	}
	stale := f.stream(f.other, f.e.Rep1, "an-older-revision", "")
	if stale.Code != http.StatusOK || stale.Header().Get("Cache-Control") != "private, no-cache" {
		t.Errorf("a stale revision = %d under %q, want the current picture uncached",
			stale.Code, stale.Header().Get("Cache-Control"))
	}
	if again := f.stream(f.other, f.e.Rep1, revision, etag); again.Code != http.StatusNotModified {
		t.Errorf("revalidating the same revision = %d, want 304", again.Code)
	}
}

func TestAReplacedOrDeletedPictureLeavesNoBytesBehind(t *testing.T) {
	f := newAvatarFixture(t)
	f.upload(f.me, t, encodedPicture(t, pngOf, halvesImage(40, 40)))
	first := *f.storedKey(t, f.e.Rep1)
	f.upload(f.me, t, encodedPicture(t, jpegOf, halvesImage(50, 50)))
	second := *f.storedKey(t, f.e.Rep1)
	if f.objectExists(t, first) || !f.objectExists(t, second) {
		t.Errorf("after a replace: the old object exists %v, the new one %v", f.objectExists(t, first), f.objectExists(t, second))
	}

	req := httptest.NewRequest(http.MethodDelete, "/v1/me/avatar", nil).WithContext(f.me)
	rec := httptest.NewRecorder()
	f.h.DeleteMyAvatar(rec, req)
	if rec.Code != http.StatusOK || decodeUser(t, rec).AvatarUrl != nil {
		t.Fatalf("delete = %d: %s", rec.Code, rec.Body)
	}
	if f.objectExists(t, second) {
		t.Error("the deleted picture's bytes are still stored")
	}
	if got := f.stream(f.other, f.e.Rep1, "", ""); got.Code != http.StatusNotFound {
		t.Errorf("streaming a deleted picture = %d, want 404", got.Code)
	}
}

func TestTheUploadRefusesWhatIsNotTheSeatsOwnPicture(t *testing.T) {
	f := newAvatarFixture(t)
	gifBytes := encodedPicture(t, gifOf, halvesImage(20, 20))
	if rec := f.upload(f.me, t, gifBytes); rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("a GIF = %d, want 415: %s", rec.Code, rec.Body)
	}
	agent := principal.WithActor(f.me, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:test", UserID: f.e.Rep1,
	})
	if rec := f.upload(agent, t, encodedPicture(t, pngOf, halvesImage(20, 20))); rec.Code != http.StatusForbidden {
		t.Errorf("an agent uploading its user's picture = %d, want 403", rec.Code)
	}
	if key := f.storedKey(t, f.e.Rep1); key != nil {
		t.Errorf("a refused upload left the seat naming %s", *key)
	}
}

func TestTheStreamIsForMembersAndHidesWhichSeatsExist(t *testing.T) {
	f := newAvatarFixture(t)
	f.upload(f.me, t, encodedPicture(t, pngOf, halvesImage(20, 20)))
	buyer := principal.WithActor(principal.WithWorkspaceID(context.Background(), f.e.WS),
		principal.Principal{Type: principal.PrincipalBuyer, ID: "buyer:room-guest"})
	if rec := f.stream(buyer, f.e.Rep1, "", ""); rec.Code != http.StatusForbidden {
		t.Errorf("a Deal Room buyer streaming a picture = %d, want 403", rec.Code)
	}
	for name, seat := range map[string]ids.UUID{"no picture": f.e.Rep2, "unknown seat": ids.NewV7()} {
		if rec := f.stream(f.me, seat, "", ""); rec.Code != http.StatusNotFound {
			t.Errorf("%s = %d, want 404", name, rec.Code)
		}
	}
}

func TestTheSignInFillStoresTheGooglePictureUntilTheMemberDeletesIt(t *testing.T) {
	f := newAvatarFixture(t)
	rep := ids.From[ids.UserKind](f.e.Rep1)
	pictures := f.h.pictures
	if err := pictures.FillFromPictureURL(context.Background(), rep, googlePicture); err != nil {
		t.Fatalf("filling from Google: %v", err)
	}
	filled := f.storedKey(t, f.e.Rep1)
	if filled == nil || !f.objectExists(t, *filled) {
		t.Fatal("the Google picture was not stored as the seat's own")
	}
	if err := pictures.FillFromPictureURL(context.Background(), rep, googlePicture); err != nil {
		t.Fatalf("the same picture again: %v", err)
	}
	if again := f.storedKey(t, f.e.Rep1); again == nil || *again != *filled {
		t.Error("the same Google picture moved the seat to another key")
	}

	if _, _, err := pictures.users.RemoveMyAvatar(f.me); err != nil {
		t.Fatalf("deleting: %v", err)
	}
	f.site.assets[googlePicture] = encodedPicture(t, pngOf, halvesImage(30, 30))
	if err := pictures.FillFromPictureURL(context.Background(), rep, googlePicture); err != nil {
		t.Fatalf("a sign-in after the delete: %v", err)
	}
	if key := f.storedKey(t, f.e.Rep1); key != nil {
		t.Errorf("a sign-in after the delete gave the seat %s", *key)
	}
}
