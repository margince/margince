// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

// Which picture a seat shows: an upload, else the Google account's, else
// initials; and a delete keeps it on initials until the next upload.

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/storedobjects"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// avatarNoon is the clock a delete is stamped with.
var avatarNoon = time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)

type storedAvatar struct {
	key, origin *string
	removedAt   *time.Time
}

func readAvatar(t *testing.T, e *revocationEnv, user ids.UserID) storedAvatar {
	t.Helper()
	var got storedAvatar
	if err := e.owner.QueryRow(context.Background(),
		`SELECT avatar_object_key, avatar_origin, avatar_removed_at FROM app_user WHERE id = $1`, user).
		Scan(&got.key, &got.origin, &got.removedAt); err != nil {
		t.Fatalf("reading the seat's picture columns: %v", err)
	}
	return got
}

// provisional reports whether the stored-object ledger holds key as an intent.
func provisional(t *testing.T, e *revocationEnv, key string) bool {
	t.Helper()
	var held bool
	if err := e.owner.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM stored_object_intent WHERE storage_key = $1 AND kind = 'avatar')`, key).
		Scan(&held); err != nil {
		t.Fatalf("reading the stored-object ledger: %v", err)
	}
	return held
}

func avatarAudits(t *testing.T, e *revocationEnv, user ids.UserID) int {
	t.Helper()
	var n int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log WHERE entity_type = 'user' AND entity_id = $1 AND after ? 'avatar_origin'`, user).
		Scan(&n); err != nil {
		t.Fatalf("counting the picture's audit rows: %v", err)
	}
	return n
}

func systemCtx(e *revocationEnv) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws.UUID)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalSystem, ID: "system:test"})
}

func fillFromGoogle(t *testing.T, e *revocationEnv, user ids.UserID, key string) (bool, *string) {
	t.Helper()
	written, dropped, err := e.svc.FillAvatarFromProvider(systemCtx(e), user, key)
	if err != nil {
		t.Fatalf("filling the picture from Google: %v", err)
	}
	return written, dropped
}

func textOrNull(s *string) string {
	if s == nil {
		return "<null>"
	}
	return *s
}

func TestAnUploadWinsOverTheGooglePicture(t *testing.T) {
	e := setupRevocationEnv(t, "avatar-upload-wins")
	me := e.admin.UserID
	if written, _ := fillFromGoogle(t, e, me, "ws/avatar/google-1.png"); !written {
		t.Fatal("a seat nobody chose a picture for refused the Google picture")
	}

	if err := storedobjects.NewLedger(e.svc.db).Record(e.wsCtx(e.admin), storedobjects.KindAvatar, "ws/avatar/upload-1.png"); err != nil {
		t.Fatalf("declaring the upload provisional, as the upload route does before its put: %v", err)
	}
	if !provisional(t, e, "ws/avatar/upload-1.png") {
		t.Fatal("the seeded upload intent is not in the ledger, so the check below would prove nothing")
	}
	seat, dropped, err := e.svc.SetMyAvatar(e.wsCtx(e.admin), "ws/avatar/upload-1.png")
	if err != nil {
		t.Fatalf("uploading: %v", err)
	}
	if textOrNull(dropped) != "ws/avatar/google-1.png" {
		t.Errorf("the upload hands back %s, want the Google key it replaced", textOrNull(dropped))
	}
	if textOrNull(seat.AvatarKey) != "ws/avatar/upload-1.png" {
		t.Errorf("the answer carries %s, want the uploaded key", textOrNull(seat.AvatarKey))
	}
	if !provisional(t, e, "ws/avatar/google-1.png") || provisional(t, e, "ws/avatar/upload-1.png") {
		t.Error("the replaced key must become provisional and the adopted one must not be")
	}

	if written, _ := fillFromGoogle(t, e, me, "ws/avatar/google-2.png"); written {
		t.Error("a later Google picture replaced the one the member uploaded")
	}
	got := readAvatar(t, e, me)
	if textOrNull(got.key) != "ws/avatar/upload-1.png" || textOrNull(got.origin) != string(avatarUploaded) {
		t.Errorf("app_user holds %s from %s, want the upload", textOrNull(got.key), textOrNull(got.origin))
	}
}

func TestADeletePinsTheSeatToInitialsUntilTheNextUpload(t *testing.T) {
	e := setupRevocationEnv(t, "avatar-delete-pins")
	e.svc.now = func() time.Time { return avatarNoon }
	me := e.admin.UserID
	fillFromGoogle(t, e, me, "ws/avatar/google-1.png")

	seat, removed, err := e.svc.RemoveMyAvatar(e.wsCtx(e.admin))
	if err != nil {
		t.Fatalf("deleting: %v", err)
	}
	if seat.AvatarKey != nil || textOrNull(removed) != "ws/avatar/google-1.png" {
		t.Errorf("the delete answers key %s and hands back %s", textOrNull(seat.AvatarKey), textOrNull(removed))
	}
	got := readAvatar(t, e, me)
	if got.key != nil || got.origin != nil || got.removedAt == nil || !got.removedAt.Equal(avatarNoon) {
		t.Errorf("after the delete app_user holds key %s, origin %s, removed %v", textOrNull(got.key), textOrNull(got.origin), got.removedAt)
	}

	if written, _ := fillFromGoogle(t, e, me, "ws/avatar/google-2.png"); written {
		t.Error("a Google sign-in brought back a picture the member deleted")
	}

	if _, _, err := e.svc.SetMyAvatar(e.wsCtx(e.admin), "ws/avatar/upload-1.png"); err != nil {
		t.Fatalf("uploading after the delete: %v", err)
	}
	got = readAvatar(t, e, me)
	if textOrNull(got.key) != "ws/avatar/upload-1.png" || got.removedAt != nil {
		t.Errorf("an upload after a delete left key %s, removed %v; want the upload and the pin lifted", textOrNull(got.key), got.removedAt)
	}
}

func TestRemovingAPictureTheSeatNeverHadStillKeepsGoogleOut(t *testing.T) {
	e := setupRevocationEnv(t, "avatar-delete-empty")
	me := e.admin.UserID
	for range 2 {
		if _, _, err := e.svc.RemoveMyAvatar(e.wsCtx(e.admin)); err != nil {
			t.Fatalf("deleting a picture the seat does not have: %v", err)
		}
	}
	if n := avatarAudits(t, e, me); n != 1 {
		t.Errorf("two deletes of no picture left %d audit rows, want 1: the second moves nothing", n)
	}
	if written, _ := fillFromGoogle(t, e, me, "ws/avatar/google-1.png"); written {
		t.Error("a Google picture filled a seat whose member asked for initials")
	}
}

func TestTheGooglePictureRefreshesWhileItIsTheSeatsPicture(t *testing.T) {
	e := setupRevocationEnv(t, "avatar-google-refresh")
	me := e.admin.UserID
	fillFromGoogle(t, e, me, "ws/avatar/google-1.png")

	written, dropped := fillFromGoogle(t, e, me, "ws/avatar/google-2.png")
	if !written || textOrNull(dropped) != "ws/avatar/google-1.png" {
		t.Errorf("a new Google picture wrote %v and dropped %s, want it written over the old one", written, textOrNull(dropped))
	}
	before := avatarAudits(t, e, me)
	written, dropped = fillFromGoogle(t, e, me, "ws/avatar/google-2.png")
	if !written || dropped != nil {
		t.Errorf("the same Google picture again reports written %v, dropped %s; want shown, nothing dropped", written, textOrNull(dropped))
	}
	if after := avatarAudits(t, e, me); after != before {
		t.Errorf("the same Google picture again wrote %d audit rows", after-before)
	}
	if got := readAvatar(t, e, me); textOrNull(got.key) != "ws/avatar/google-2.png" || textOrNull(got.origin) != string(avatarFromGoogle) {
		t.Errorf("app_user holds %s from %s, want the newer Google picture", textOrNull(got.key), textOrNull(got.origin))
	}
}

func TestAnyMemberReadsAColleaguesPicture(t *testing.T) {
	e := setupRevocationEnv(t, "avatar-read")
	rep := inviteAndLogin(t, e, "rep@acme.test", "Rep One")
	fillFromGoogle(t, e, e.admin.UserID, "ws/avatar/google-1.png")

	key, err := e.svc.SeatAvatarKey(e.wsCtx(rep), e.admin.UserID)
	if err != nil || key != "ws/avatar/google-1.png" {
		t.Errorf("a rep reading the admin's picture got %q, %v", key, err)
	}

	notFound := map[string]ids.UserID{"no picture": rep.UserID, "unknown seat": ids.New[ids.UserKind]()}
	for name, seat := range notFound {
		if _, err := e.svc.SeatAvatarKey(e.wsCtx(e.admin), seat); !errors.Is(err, apperrors.ErrNotFound) {
			t.Errorf("%s: %v, want not found", name, err)
		}
	}
	archiveSeat(t, e, e.admin.UserID)
	if _, err := e.svc.SeatAvatarKey(e.wsCtx(rep), e.admin.UserID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("an archived seat's picture: %v, want not found", err)
	}
}

func TestTheSweepSeesOnlyPictureKeysNoSeatNames(t *testing.T) {
	e := setupRevocationEnv(t, "avatar-sweep")
	fillFromGoogle(t, e, e.admin.UserID, "ws/avatar/google-1.png")
	unreferenced, err := e.svc.UnreferencedAvatarKeys(systemCtx(e), []string{"ws/avatar/google-1.png", "ws/avatar/orphan.png"})
	if err != nil {
		t.Fatalf("asking which keys are unreferenced: %v", err)
	}
	if len(unreferenced) != 1 || unreferenced[0] != "ws/avatar/orphan.png" {
		t.Errorf("unreferenced = %v, want only the orphan", unreferenced)
	}
}

// The session is minted while the fill still waits on the picture host. A slow
// or dead host costs the seat its picture, never the sign-in.
func TestASignInOffersItsPictureWithoutWaitingForTheFill(t *testing.T) {
	svc, _, userID, email := seedSSOEnv(t, "avatar-sign-in")
	type offer struct {
		user ids.UserID
		url  string
	}
	offered := make(chan offer, 1)
	release := make(chan struct{})
	svc.pictureFill = func(_ context.Context, user ids.UserID, pictureURL string) error {
		offered <- offer{user, pictureURL}
		<-release
		return nil
	}
	defer close(release)

	token, err := svc.LoginViaFederatedIdentity(groupSyncCtx(), "google", OIDCClaims{
		Subject: "sub-picture", Email: email, Picture: "https://lh3.googleusercontent.com/a/carol",
	})
	if err != nil || token == "" {
		t.Fatalf("signing in while the picture fill is held: token %q, %v", token, err)
	}
	var got offer
	select {
	case got = <-offered:
	case <-time.After(30 * time.Second):
		t.Fatal("the sign-in never offered its picture")
	}
	if got.user != userID || got.url != "https://lh3.googleusercontent.com/a/carol" {
		t.Errorf("the fill was offered %v at %q, want the signed-in seat and its picture", got.user, got.url)
	}
}

// Every mapping that renders a seat (SeatUser, wireUser, meResponse and /users/names)
// carries its picture's URL, and every self-service save answers with its zone too.
func TestEveryReadThatNamesASeatCarriesItsPictureURL(t *testing.T) {
	e := setupRevocationEnv(t, "avatar-wire")
	rep := inviteAndLogin(t, e, "rep@acme.test", "Rep One")
	me := e.wsCtx(rep)
	fillFromGoogle(t, e, rep.UserID, "ws/avatar/google-1.png")
	want := "/v1/users/" + rep.UserID.String() + "/avatar?v=" + AvatarETag("ws/avatar/google-1.png")

	loggedIn, session, err := e.svc.Login(principal.WithWorkspaceID(context.Background(), e.ws.UUID), "rep@acme.test", seatNamePassword, noDevice)
	if err != nil {
		t.Fatalf("signing in: %v", err)
	}
	expectPicture(t, "/auth/login", NewHandlers(e.svc).meResponse(me, loggedIn).User.AvatarUrl, want)
	signedIn, err := e.svc.Authenticate(principal.WithWorkspaceID(context.Background(), e.ws.UUID), session.Token)
	if err != nil {
		t.Fatalf("resolving the session: %v", err)
	}
	expectPicture(t, "/me", NewHandlers(e.svc).meResponse(me, signedIn).User.AvatarUrl, want)

	page, err := e.svc.ListUsers(e.wsCtx(e.admin), ListUsersInput{})
	if err != nil {
		t.Fatalf("listing the roster: %v", err)
	}
	listed := slices.IndexFunc(page.Users, func(u userRow) bool { return u.ID == rep.UserID.UUID })
	if listed < 0 {
		t.Fatal("the roster does not list the rep")
	}
	expectPicture(t, "the roster", wireUser(page.Users[listed]).AvatarUrl, want)

	names := nameSeatsBody(t, e, e.admin, []openapi_types.UUID{openapi_types.UUID(rep.UserID.UUID)})
	if len(names.Data) != 1 {
		t.Fatalf("/users/names named %d seats, want the rep", len(names.Data))
	}
	expectPicture(t, "/users/names", names.Data[0].AvatarUrl, want)

	if _, err := e.svc.SaveMyTimezone(me, "Europe/Berlin", TimezoneChosen); err != nil {
		t.Fatalf("saving the zone: %v", err)
	}
	greeting := "Rep"
	saves := map[string]func() (Seat, error){
		"display name": func() (Seat, error) { return e.svc.SaveMyDisplayName(me, "Rep Renamed") },
		"greeting":     func() (Seat, error) { return e.svc.SaveMyGreetingName(me, &greeting) },
		"locale":       func() (Seat, error) { return e.svc.SaveMyLocale(me, "de") },
		"time zone":    func() (Seat, error) { return e.svc.SaveMyTimezone(me, "Europe/Berlin", TimezoneChosen) },
	}
	for name, save := range saves {
		seat, err := save()
		if err != nil {
			t.Fatalf("saving the %s: %v", name, err)
		}
		expectSeat(t, "the "+name+" save", SeatUser(seat), &want)
	}

	uploaded, _, err := e.svc.SetMyAvatar(me, "ws/avatar/upload-1.png")
	if err != nil {
		t.Fatalf("uploading: %v", err)
	}
	newer := "/v1/users/" + rep.UserID.String() + "/avatar?v=" + AvatarETag("ws/avatar/upload-1.png")
	expectSeat(t, "the upload", SeatUser(uploaded), &newer)
	for range 2 {
		removed, _, err := e.svc.RemoveMyAvatar(me)
		if err != nil {
			t.Fatalf("removing: %v", err)
		}
		expectSeat(t, "the removal", SeatUser(removed), nil)
	}
}

func expectPicture(t *testing.T, reader string, got *string, want string) {
	t.Helper()
	if textOrEmpty(got) != want {
		t.Errorf("%s answers the picture %q, want %q", reader, textOrEmpty(got), want)
	}
}

// expectSeat holds a self-service answer to the picture and to the zone the seat holds.
func expectSeat(t *testing.T, save string, user crmcontracts.User, picture *string) {
	t.Helper()
	if textOrEmpty(user.AvatarUrl) != textOrEmpty(picture) {
		t.Errorf("%s answers the picture %q, want %q", save, textOrEmpty(user.AvatarUrl), textOrEmpty(picture))
	}
	if textOrEmpty(user.Timezone) != "Europe/Berlin" {
		t.Errorf("%s answers the zone %q, want the seat's Europe/Berlin", save, textOrEmpty(user.Timezone))
	}
}

func textOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
