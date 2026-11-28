// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The owners reached over HTTP name the installation's stored-key id. Each case
// gives the installation a stored-key id that differs from the bound workspace.
// A builder that read the request's id then fails it.

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/compose/integration/importjobtest"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/deployconfig"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/runtimeenv"
)

func distinctStoredKey(t *testing.T, e *apptest.AppEnv) ids.WorkspaceID {
	t.Helper()
	stored := ids.NewV7()
	if _, err := e.Owner.Exec(context.Background(),
		`UPDATE installation_state SET value = jsonb_build_object('id', $1::text) WHERE key = 'stored_key_workspace'`,
		stored.String()); err != nil {
		t.Fatalf("moving the stored-key id: %v", err)
	}
	return ids.From[ids.WorkspaceKind](stored)
}

func TestAnImportSourceIsStoredUnderTheStoredKeyPrefix(t *testing.T) {
	e, _ := setupImportAppWithStore(t)
	stored := distinctStoredKey(t, e)
	profile, status := importjobtest.UploadCSV(t, e, "company", "Company\nKestrel Data\n")
	if status != http.StatusOK {
		t.Fatalf("uploading the source = %d, want 200", status)
	}
	if !strings.HasPrefix(profile.SourceRef, blobstore.WorkspaceKey(stored, "import", "")) {
		t.Errorf("source %q is not under the stored-key prefix", profile.SourceRef)
	}
	run, code := importjobtest.CreateRunWithMapping(t, e, "company", profile.SourceRef, map[string]string{"Company": "display_name"})
	if code != http.StatusAccepted {
		t.Fatalf("a run on the source = %d (%+v), want 202: the source check must accept what the upload minted", code, run)
	}
}

func TestAnAvatarIsStoredUnderTheStoredKeyPrefix(t *testing.T) {
	store := blobstore.NewMemory()
	e := apptest.SetupAppWithOptions(t, compose.WithBlobstore(store))
	e.BootstrapWorkspace(t)
	stored := distinctStoredKey(t, e)

	var picture bytes.Buffer
	if err := png.Encode(&picture, solid(40, 40)); err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("file", "me.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(picture.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, e.TS.URL+"/v1/me/avatar", &body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", form.FormDataContentType())
	//nolint:bodyclose // apptest.CloseBody closes it below.
	resp, err := e.Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	apptest.CloseBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("uploading the avatar = %d, want 200", resp.StatusCode)
	}
	if got := countUnder(context.Background(), t, store, legacyPrefix(stored, "avatar")); got != 1 {
		t.Errorf("%d objects under the stored-key avatar prefix, want 1", got)
	}
}

func TestTheResetDeletesTheObjectsStoredUnderTheStoredKeyPrefix(t *testing.T) {
	store := blobstore.NewMemory()
	e := apptest.SetupAppWithOptions(t,
		compose.WithBlobstore(store),
		compose.WithDataReset(nil, deployconfig.Seeds{}, true),
		compose.WithNonProduction(runtimeenv.Production),
		compose.WithDataResetAvailable(true),
	)
	apptest.BootstrapWorkspaceSession(t, e, "Fable E2E", "ada@example.com", "Ada Admin")
	stored := distinctStoredKey(t, e)
	key := blobstore.WorkspaceKey(stored, "attachment", ids.NewV7().String())
	if err := store.Put(context.Background(), key, strings.NewReader("x"), 1, "text/plain"); err != nil {
		t.Fatal(err)
	}
	if code := e.Call(t, "POST", "/v1/admin/reset-data", AnyMap{"confirmation": "Fable E2E"}, nil, nil); code != http.StatusOK {
		t.Fatalf("reset = %d, want 200", code)
	}
	if _, _, err := store.Get(context.Background(), key); err == nil {
		t.Error("an object under the stored-key prefix survived the reset")
	}
}

func solid(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for x := range w {
		for y := range h {
			img.Set(x, y, color.NRGBA{R: 200, A: 255})
		}
	}
	return img
}
