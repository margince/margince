// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestTeamsChannelSetupUsesVerifiedInstallationAndPinnedApp(t *testing.T) {
	const app = "018f3a1b-0000-7000-8000-00000000a001"
	const tenant = "018f3a1b-0000-7000-8000-00000000a002"
	e := integration.Setup(t)
	e.GrantRole(t, e.AdminUser, "admin")
	e.GrantRole(t, e.Rep1, "rep")
	admin := integration.OnSeat(e.Admin(), principal.SeatFull)
	store := chatlinks.NewStore(e.DB(), nil, nil, nil)
	source := chatSourceHandlers{pool: e.Pool, links: store}
	oauth := messengerOAuthHandlers{cfg: GraphConfig{ClientID: app, ClientSecret: "synthetic-only", Tenant: tenant, PublicBaseURL: "https://crm.example.test"}}
	server := Server{chatSourceHandlers: source, messengerOAuthHandlers: oauth}
	systemActor, _ := principal.Actor(admin)
	systemActor.Type = principal.PrincipalSystem
	system := principal.WithActor(admin, systemActor)
	install := chatlinks.TeamInstallation{AppID: app, Tenant: tenant, Team: ids.NewV7().String(), NativeTeam: "19:verified@thread.tacv2", BotMember: "28:" + app, ServiceURL: "https://smba.trafficmanager.net/amer/", Name: "Verified team", Installed: true, SignedAt: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)}
	if err := store.RecordTeamInstallation(system, install); err != nil {
		t.Fatal(err)
	}
	foreign := install
	foreign.AppID = ids.NewV7().String()
	foreign.Name = "Different app team"
	if err := store.RecordTeamInstallation(system, foreign); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/v1/channel-providers/teams/setup", nil).WithContext(admin)
	response := httptest.NewRecorder()
	server.GetTeamsChannelSetup(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("setup: %d %s", response.Code, response.Body.String())
	}
	var setup crmcontracts.TeamsChannelSetup
	if err := json.Unmarshal(response.Body.Bytes(), &setup); err != nil {
		t.Fatal(err)
	}
	if !setup.Configured || setup.AppId != app || setup.TenantId != tenant || len(setup.Installations) != 1 || setup.Installations[0].Name != "Verified team" || !strings.HasSuffix(setup.EventsUrl, "/v1/channel-events/teams/bot") {
		t.Fatalf("setup did not pin installation: %#v", setup)
	}
	if strings.Contains(response.Body.String(), "synthetic-only") || strings.Contains(response.Body.String(), "service_url") {
		t.Fatal("setup exposed credential or private native reference")
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/v1/channel-providers/teams/package", strings.NewReader(`{"privacy_url":"https://legal.example.test/privacy","terms_url":"https://legal.example.test/terms"}`)).WithContext(admin)
	server.DownloadTeamsChannelPackage(response, request)
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/zip" {
		t.Fatalf("package: %d %s", response.Code, response.Body.String())
	}
	archive, err := zip.NewReader(bytes.NewReader(response.Body.Bytes()), int64(response.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, file := range archive.File {
		if file.Name == "manifest.json" {
			input, err := file.Open()
			if err != nil {
				t.Fatal(err)
			}
			manifest, readErr := io.ReadAll(input)
			closeErr := input.Close()
			if readErr != nil || closeErr != nil {
				t.Fatalf("read package: %v %v", readErr, closeErr)
			}
			if !bytes.Contains(manifest, []byte(app)) || !bytes.Contains(manifest, []byte("https://legal.example.test/privacy")) || bytes.Contains(manifest, []byte("synthetic-only")) {
				t.Fatalf("package lost pinned identity or disclosed a secret: %s", manifest)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("download has no manifest")
	}
	rep := integration.OnSeat(e.As(e.Rep1, nil, integration.AccountRepPerms), principal.SeatFull)
	response = httptest.NewRecorder()
	server.GetTeamsChannelSetup(response, httptest.NewRequest(http.MethodGet, "/v1/channel-providers/teams/setup", nil).WithContext(rep))
	if response.Code == http.StatusOK && strings.Contains(response.Body.String(), "Verified team") {
		t.Fatal("unapproved seat discovered installation names")
	}
	response = httptest.NewRecorder()
	server.DownloadTeamsChannelPackage(response, request.WithContext(rep))
	if response.Code != http.StatusForbidden {
		t.Fatalf("unapproved package download: %d", response.Code)
	}
	response = httptest.NewRecorder()
	server.ConnectTeamsChannels(response, httptest.NewRequest(http.MethodPost, "/v1/channel-providers/teams/connect", strings.NewReader(`{"installation_id":"`+ids.NewV7().String()+`"}`)).WithContext(admin))
	if response.Code != http.StatusConflict {
		t.Fatalf("unverified installation accepted: %d %s", response.Code, response.Body.String())
	}
}
