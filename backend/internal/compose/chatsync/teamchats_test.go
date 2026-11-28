// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/modules/chatlinks/teams"
)

type teamRoutingHTTP struct{ t *testing.T }

func (h teamRoutingHTTP) Do(r *http.Request) (*http.Response, error) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1.0/"), "/")
	if len(parts) < 2 || parts[0] != "teams" {
		h.t.Fatalf("unexpected Graph path: %s", r.URL.Path)
	}
	team := parts[1]
	body := `{"id":"` + team + `","displayName":"` + team + `"}`
	if len(parts) > 2 {
		switch parts[2] {
		case "installedApps":
			body = `{"value":[{"teamsAppDefinition":{"azureADAppId":"APP"}}]}`
		case "channels":
			if len(parts) == 3 {
				body = `{"value":[{"id":"CHANNEL","displayName":"General","membershipType":"standard"}]}`
				break
			}
			body = `{"id":"CHANNEL","displayName":"` + team + ` pilot","membershipType":"standard"}`
		default:
			h.t.Fatalf("unexpected team resource: %s", r.URL.Path)
		}
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
}

func TestApprovedTeamRouterPreservesEachChannelAndRejectsOtherTenants(t *testing.T) {
	credential := chatwire.BotCredential{Workspace: "TENANT", Member: "A", NativeTeam: "native-a", BotMember: "BOT", ServiceURL: "https://smba.trafficmanager.net/amer/", Credential: messaging.Credential{AppID: "APP"}}
	first := teams.NewBot(teamRoutingHTTP{t}, "graph", "connector", credential)
	credential.Member, credential.NativeTeam = "B", "native-b"
	second, err := first.ForTeam(credential)
	if err != nil {
		t.Fatal(err)
	}
	router := teamChats{Bot: first, bots: map[string]*teams.Bot{"A": first, "B": second}}
	for _, team := range []string{"A", "B"} {
		channel, err := router.Channel(context.Background(), team+"/CHANNEL")
		if err != nil || channel.Slug != team+"/CHANNEL" || channel.Name != team+" pilot" {
			t.Fatalf("wrong team route: %+v %v", channel, err)
		}
	}
	if _, err := router.Channel(context.Background(), "UNAPPROVED/CHANNEL"); !errors.Is(err, chatwire.ErrNotMember) {
		t.Fatalf("unapproved team accepted: %v", err)
	}
	channels, err := router.Channels(context.Background())
	if err != nil || len(channels) != 2 || channels[0].Name != "A / General" || channels[1].Name != "B / General" {
		t.Fatalf("ambiguous team choices: %+v %v", channels, err)
	}
	location, err := router.LocateMessage(context.Background(), "B/CHANNEL", "1234")
	if err != nil || location.Root != "1234" {
		t.Fatalf("second-team source not routed: %+v %v", location, err)
	}
	if _, err := router.LocateMessage(context.Background(), "OTHER/CHANNEL", "1234"); !errors.Is(err, chatwire.ErrNotMember) {
		t.Fatalf("unapproved source routed: %v", err)
	}
	credential.Workspace = "OTHER_TENANT"
	if _, err := first.ForTeam(credential); !errors.Is(err, messaging.ErrInvalid) {
		t.Fatalf("cross-tenant token reuse: %v", err)
	}
}
