// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package chatsync

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/oidctoken"
	"github.com/margince/margince/backend/internal/compose/oidctoken/oidctokentest"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type teamsProtocolHTTP struct {
	client *http.Client
	api    *url.URL
	keys   *url.URL
}

func (h teamsProtocolHTTP) Do(request *http.Request) (*http.Response, error) {
	forwarded := request.Clone(request.Context())
	target := *request.URL
	if request.URL.Host == "login.botframework.com" {
		target = *h.keys
	} else {
		target.Scheme, target.Host = h.api.Scheme, h.api.Host
		target.Path = strings.TrimPrefix(target.Path, "/v1.0")
	}
	forwarded.URL = &target
	return h.client.Do(forwarded)
}

func TestTeamsSignedActivityBindsNativeReferencesAndRemovalPausesLinks(t *testing.T) {
	const app = "018f3a1b-0000-7000-8000-00000000a001"
	const tenant = "018f3a1b-0000-7000-8000-00000000a002"
	const team = "018f3a1b-0000-7000-8000-00000000a003"
	const service = "https://smba.trafficmanager.net/amer/"
	const native = "19:native-team@thread.tacv2"
	e := setupFlush(t)
	var connection ids.UUID
	if err := e.owner.QueryRow(context.Background(), `SELECT connection_id FROM chat_link WHERE id=$1`, e.link).Scan(&connection); err != nil {
		t.Fatal(err)
	}
	e.exec(t, `UPDATE channel_connection SET provider='teams',channel_id=$2,address=NULL WHERE id=$1`, connection, tenant+"/"+team)
	e.exec(t, `UPDATE chat_link SET provider='teams',channel_slug=$2 WHERE id=$1`, e.link, team+"/C_ONE")
	rig := oidctokentest.NewRig(t)
	rig.Endorsements = []string{"msteams"}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var response any
		switch r.URL.Path {
		case "/" + tenant + "/oauth2/v2.0/token":
			if err := r.ParseForm(); err != nil {
				t.Error(err)
			}
			token := "graph-synthetic"
			if r.Form.Get("scope") == "https://api.botframework.com/.default" {
				token = "connector-synthetic"
			}
			response = map[string]any{"access_token": token, "expires_in": 3600, "token_type": "Bearer"}
		case "/amer/v3/teams/" + native:
			if r.Header.Get("Authorization") != "Bearer connector-synthetic" {
				t.Error("team identity lookup used a Graph or human token")
			}
			response = map[string]string{"id": native, "aadGroupId": team, "name": "Account team"}
		default:
			t.Errorf("unexpected Teams boundary %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Error(err)
		}
	}))
	defer api.Close()
	apiURL, err := url.Parse(api.URL)
	if err != nil {
		t.Fatal(err)
	}
	keysURL, err := url.Parse(rig.JWKSURL())
	if err != nil {
		t.Fatal(err)
	}
	boundary := teamsProtocolHTTP{client: api.Client(), api: apiURL, keys: keysURL}
	verifier := oidctoken.NewBotFramework().WithHTTPClient(&http.Client{Transport: botTransport{boundary}}).SetClock(func() time.Time { return rig.Base })
	registry := capture.NewRegistry(e.db(), nil, nil, nil)
	pulls := 0
	parent := &Receiver{Links: e.store, Replays: fakeReplays{}, Marks: fakeMarks{}, Now: func() time.Time { return rig.Base }, Log: e.flusher.Log, Pull: func(_ context.Context, ws, conn ids.UUID) error {
		if ws != e.ws || conn != connection {
			t.Error("notice queued a different installation")
		}
		pulls++
		return nil
	}}
	receiver := TeamsReceiver{Receiver: parent, Store: e.store, HTTP: boundary, Verifier: verifier, App: func(context.Context, string) (capture.ConnectorApp, error) {
		return capture.ConnectorApp{ClientID: app, Tenant: tenant, ClientSecretRef: "synthetic-client-secret"}, nil
	}, Binding: func(ctx context.Context) (capture.LiveBot, error) { return registry.LiveBotIdentity(ctx, "teams") }}
	token := rig.Mint(t, oidctokentest.KID, "RS256", map[string]any{"iss": "https://api.botframework.com", "aud": app, "nbf": rig.Base.Add(-time.Minute).Unix(), "iat": nil, "serviceurl": service})
	body := map[string]any{"id": "root-native-id", "type": "message", "timestamp": rig.Base, "serviceUrl": service, "channelId": "msteams", "text": "PRIVATE-SOURCE-MARKER", "recipient": map[string]string{"id": "28:" + app}, "conversation": map[string]string{"id": "opaque-conversation-reference", "tenantId": tenant}, "channelData": map[string]any{"tenant": map[string]string{"id": tenant}, "team": map[string]string{"id": native}, "channel": map[string]string{"id": "C_ONE"}}}
	deliver := func() (chatlinks.Delivery, error) {
		raw, err := json.Marshal(body)
		return chatlinks.Delivery{Authorization: "Bearer " + token, Body: raw}, err
	}
	delivery, err := deliver()
	if err != nil {
		t.Fatal(err)
	}
	ctx := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.ws), ids.NewV7())
	got, err := receiver.Receive(ctx, delivery)
	if err != nil || statusOf(got) != "pull_queued" || pulls != 1 {
		t.Fatalf("signed native activity failed: %+v %v pulls=%d", got, err, pulls)
	}
	engine := principal.WithActor(ctx, chatLinksPrincipal)
	installation, err := e.store.TeamInstallationByNative(engine, app, tenant, native)
	if err != nil || !installation.Installed || installation.Team != team {
		t.Fatalf("installation binding missing: %+v %v", installation, err)
	}
	ref, err := e.store.TeamThread(engine, app, tenant, team+"/C_ONE", "root-native-id")
	if err != nil || ref.Conversation != "opaque-conversation-reference" {
		t.Fatalf("native conversation reference was reconstructed or lost: %+v %v", ref, err)
	}
	var retained string
	if err := e.owner.QueryRow(context.Background(), `SELECT row_to_json(t)::text FROM chat_team_thread t`).Scan(&retained); err != nil || strings.Contains(retained, "PRIVATE-SOURCE-MARKER") {
		t.Fatalf("retained source content: %q %v", retained, err)
	}
	got, err = receiver.Receive(ctx, delivery)
	if err != nil || statusOf(got) != "already_taken" || pulls != 1 {
		t.Fatalf("replay applied twice: %+v %v pulls=%d", got, err, pulls)
	}
	body["serviceUrl"] = "https://smba.trafficmanager.net/emea/"
	forged, err := deliver()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := receiver.Receive(ctx, forged); !errors.Is(err, chatlinks.ErrEventRejected) || pulls != 1 {
		t.Fatalf("service URL not bound to JWT: %v pulls=%d", err, pulls)
	}
	body["serviceUrl"], body["type"], body["action"], body["timestamp"] = service, "installationUpdate", "remove", rig.Base.Add(time.Minute)
	removal, err := deliver()
	if err != nil {
		t.Fatal(err)
	}
	got, err = receiver.Receive(ctx, removal)
	if err != nil || statusOf(got) != "link_paused" {
		t.Fatalf("verified removal did not pause sync: %+v %v", got, err)
	}
	installation, err = e.store.TeamInstallationByNative(engine, app, tenant, native)
	if err != nil || installation.Installed {
		t.Fatalf("removed app retained installation authority: %+v %v", installation, err)
	}
	var state string
	if err := e.owner.QueryRow(context.Background(), `SELECT status FROM chat_link WHERE id=$1`, e.link).Scan(&state); err != nil || state != "channel_gone" {
		t.Fatalf("removed app link stayed live: %q %v", state, err)
	}
}
