// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package teams

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
)

func TestStandardTeamRosterUsesScopedMembershipWhenTenantIsOmitted(t *testing.T) {
	for _, guest := range []bool{false, true} {
		roles := `[]`
		if guest {
			roles = `["guest"]`
		}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response := `{"id":"CHANNEL","displayName":"Pilot","membershipType":"standard"}`
			if r.URL.Path == "/teams/TEAM/members" {
				response = `{"value":[{"@odata.type":"#microsoft.graph.aadUserConversationMember","id":"membership","roles":` + roles + `,"displayName":"Alex","userId":"U1","email":"alex@example.test"}]}`
			}
			if _, err := w.Write([]byte(response)); err != nil {
				t.Error(err)
			}
		}))
		c := New(server.Client(), "fixture", "TENANT", "U1")
		c.http.Base = server.URL
		room, err := c.Conversation(context.Background(), "TEAM/CHANNEL")
		server.Close()
		if err != nil || !room.AudienceKnown || room.External != guest {
			t.Fatalf("guest=%v room=%+v err=%v", guest, room, err)
		}
	}
}

func TestTeamsFileAndCardMessagesKeepVisibleBusinessContext(t *testing.T) {
	for _, test := range []struct {
		raw, want  string
		incomplete bool
	}{
		{`{"id":"M1","body":{"contentType":"html","content":"<attachment id=\"A\"></attachment>"},"attachments":[{"id":"A","contentType":"reference","name":"Pilot terms.pdf"}]}`, "Pilot terms.pdf: content not read", true},
		{`{"id":"M2","body":{"contentType":"html","content":""},"attachments":[{"contentType":"application/vnd.microsoft.card.adaptive","content":"{\"body\":[{\"text\":\"Legal approval required\"}]}"}]}`, "Legal approval required", false},
	} {
		var source message
		if err := json.Unmarshal([]byte(test.raw), &source); err != nil {
			t.Fatal(err)
		}
		normalized, err := source.normalized()
		if err != nil || !strings.Contains(normalized.Text, test.want) || normalized.ContentIncomplete != test.incomplete {
			t.Fatalf("normalized=%+v err=%v", normalized, err)
		}
	}
}

func TestChannelPermissionFailureLeavesOtherTeamResourcesUsable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/forbidden" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if _, err := w.Write([]byte(`{"id":"healthy"}`)); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	c := New(server.Client(), "valid-app", "TENANT", "")
	c.http.Base = server.URL
	_, err := c.http.Call(context.Background(), http.MethodGet, "/forbidden", nil)
	failure := teamsBotFailure(err)
	if !errors.Is(failure, chatwire.ErrNotMember) || errors.Is(failure, chatwire.ErrTokenRejected) {
		t.Fatalf("channel-scoped failure: %v", failure)
	}
	if _, err := c.http.Call(context.Background(), http.MethodGet, "/healthy", nil); err != nil {
		t.Fatalf("healthy channel stopped: %v", err)
	}
	if !errors.Is(err, messaging.ErrForbidden) {
		t.Fatalf("lost provider denial: %v", err)
	}
}

func TestChannelScanKeepsAnOldRootWithNewRepliesAndStopsAtOldCompleteThreads(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := `{"id":"CHANNEL","membershipType":"standard"}`
		if strings.HasSuffix(r.URL.Path, "/messages") {
			if r.URL.Query().Get("$expand") != "replies" || r.URL.Query().Has("$filter") || r.URL.Query().Has("$orderby") {
				t.Errorf("unsupported Graph scan: %s", r.URL.RawQuery)
			}
			response = `{"value":[{"id":"old-root","createdDateTime":"2026-01-01T09:00:00Z","replies":[{"id":"new-reply","createdDateTime":"2026-06-01T13:00:00Z"}]},{"id":"overflow","createdDateTime":"2026-01-01T09:00:00Z","replies@odata.nextLink":"https://graph.microsoft.com/v1.0/tail"},{"id":"old-complete","createdDateTime":"2026-01-01T09:00:00Z"}],"@odata.nextLink":"https://graph.microsoft.com/v1.0/teams/TEAM/channels/CHANNEL/messages?$skiptoken=tail"}`
		}
		if _, err := w.Write([]byte(response)); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	bot := NewBot(server.Client(), "fixture", "fixture", chatwire.BotCredential{Workspace: "TENANT", Member: "TEAM"})
	bot.installationVerified = true
	bot.graph.http.Base = server.URL
	since := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	page, err := bot.Roots(context.Background(), "TEAM/CHANNEL", since, "")
	if err != nil || len(page.Roots) != 2 || page.Next != "" || page.Roots[0].ID != "old-root" || !page.Roots[0].UpdatedAt.After(since) {
		t.Fatalf("lost changed threads or scanned past cutoff: %+v %v", page, err)
	}
	again, err := bot.Roots(context.Background(), "TEAM/CHANNEL", since.Add(time.Minute), "")
	if err != nil || len(again.Roots) != 2 || again.Roots[1].Revision == page.Roots[1].Revision {
		t.Fatalf("incomplete reply chain was not scheduled for canonical recheck: %+v %v", again, err)
	}
}

func TestDiscoveryListsExistingChatsBeforeTeamChannelsWithoutTrustingExpandedAudience(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reply := `{"value":[]}`
		if r.URL.Path == "/me/chats" {
			reply = `{"value":[{"id":"EXISTING","chatType":"oneOnOne","members":[{"userId":"SELF","displayName":"Self"},{"userId":"COLLEAGUE","displayName":"Rita"}]}]}`
			if r.URL.Query().Get("$expand") != "members" {
				t.Error("chat discovery omitted readable labels")
			}
		} else if r.URL.Path != "/me/joinedTeams" {
			t.Errorf("unexpected discovery request: %s", r.URL.Path)
		}
		if _, err := w.Write([]byte(reply)); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	client := New(server.Client(), "participant", "TENANT", "SELF")
	client.http.Base = server.URL
	rooms, cursor, err := client.Channels(context.Background(), "")
	if err != nil || len(rooms) != 1 || rooms[0].ID != "EXISTING" || rooms[0].Name != "Rita" || rooms[0].AudienceKnown || cursor == "" {
		t.Fatalf("existing chat cannot be selected safely: %+v %q %v", rooms, cursor, err)
	}
	rooms, cursor, err = client.Channels(context.Background(), cursor)
	if err != nil || len(rooms) != 0 || cursor != "" {
		t.Fatalf("discovery did not finish: %+v %q %v", rooms, cursor, err)
	}
}
