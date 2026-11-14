// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func humanWith(objects map[string]principal.ObjectGrant) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:test", UserID: ids.NewV7(), SeatType: principal.SeatFull,
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll, Objects: objects},
	})
}

func manifestAt(ctx context.Context, base string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/v1/integrations/dispact/manifest", nil)
	Handlers{}.WithManifestBase(func() string { return base }).GetDispactAppManifest(rec, req)
	return rec
}

func TestTheManifestIsBuiltFromThePublicBaseURL(t *testing.T) {
	admin := humanWith(map[string]principal.ObjectGrant{botConnectObject: {Create: true}})
	rec := manifestAt(admin, "https://crm.example.test/")
	if rec.Code != http.StatusOK {
		t.Fatalf("an admin asking for the manifest got %d %s", rec.Code, rec.Body.String())
	}
	var got crmcontracts.DispactAppManifest
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Slug != "margince" || got.Mcp.Endpoint != "https://crm.example.test/mcp" ||
		got.Events.Url != "https://crm.example.test/v1/channel-events/dispact" {
		t.Fatalf("the manifest is %+v", got)
	}
	if got.Scopes != "chat:write:channel channel:create channel:members:write users:read:email channel:history:read channel:files:read" {
		t.Fatalf("the manifest asks for scopes %q", got.Scopes)
	}
	if !slices.Equal(got.Events.Types, []string{"message.created", "thread.replied", "message.updated", "channel.archived"}) {
		t.Fatalf("the manifest subscribes to %v", got.Events.Types)
	}
}

func TestTheManifestNeedsTheRightToConnectABot(t *testing.T) {
	rep := humanWith(map[string]principal.ObjectGrant{botConnectObject: {Read: true}})
	if rec := manifestAt(rep, "https://crm.example.test"); rec.Code != http.StatusForbidden {
		t.Fatalf("a member who may not connect a bot got %d", rec.Code)
	}
}

func TestNoPublicBaseURLServesNoManifest(t *testing.T) {
	admin := humanWith(map[string]principal.ObjectGrant{botConnectObject: {Create: true}})
	if rec := manifestAt(admin, ""); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("an install with no public base URL answered %d", rec.Code)
	}
}
