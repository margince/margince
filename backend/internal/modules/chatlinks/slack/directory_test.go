// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package slack

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLargeAudienceUsesAFreshDirectoryPageInsteadOfPerMemberCalls(t *testing.T) {
	guest := false
	directoryReads := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var response any
		switch r.URL.Path {
		case "/conversations.info":
			response = map[string]any{"ok": true, "channel": map[string]any{"id": "C_PILOT", "name": "Pilot", "is_private": true, "is_member": true}}
		case "/conversations.members":
			people := []string{}
			for i := range 120 {
				people = append(people, fmt.Sprintf("U%d", i))
			}
			response = map[string]any{"ok": true, "members": people}
		case "/users.list":
			directoryReads++
			people := []any{}
			for i := range 120 {
				people = append(people, map[string]any{"id": fmt.Sprintf("U%d", i), "team_id": "TEAM", "real_name": "Colleague", "is_restricted": guest && i == 119})
			}
			response = map[string]any{"ok": true, "members": people}
		default:
			t.Errorf("unexpected per-member request %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	client := New(server.Client(), "synthetic", "TEAM", "U0")
	client.http.Base = server.URL
	first, err := client.Conversation(context.Background(), "C_PILOT")
	if err != nil || len(first.Members) != 120 || first.External || directoryReads != 1 {
		t.Fatalf("large roster: %+v %v reads=%d", first, err, directoryReads)
	}
	guest = true
	second, err := client.Conversation(context.Background(), "C_PILOT")
	if err != nil || !second.External || directoryReads != 2 {
		t.Fatalf("reused stale internal standing: %+v %v reads=%d", second, err, directoryReads)
	}
}

func TestPeopleSearchCrossesAnEmptyMatchingPage(t *testing.T) {
	reads := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reads++
		response := map[string]any{"ok": true, "members": []any{map[string]string{"id": "A", "team_id": "TEAM", "real_name": "Alex"}}, "response_metadata": map[string]string{"next_cursor": "later"}}
		if r.URL.Query().Get("cursor") == "later" {
			response = map[string]any{"ok": true, "members": []any{map[string]string{"id": "J", "team_id": "TEAM", "real_name": "Jules"}}}
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	client := New(server.Client(), "synthetic", "TEAM", "REP")
	client.http.Base = server.URL
	people, next, err := client.People(context.Background(), "Jules", "")
	if err != nil || len(people) != 1 || people[0].ID != "J" || next != "" || reads != 2 {
		t.Fatalf("search stopped on a nonmatching page: %+v next=%s err=%v reads=%d", people, next, err, reads)
	}
}

func TestJoinedChannelDiscoveryDoesNotNeedTheOptionalMembershipFlag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users.conversations" {
			t.Errorf("not scoped to calling user: %s", r.URL.Path)
		}
		if _, err := w.Write([]byte(`{"ok":true,"channels":[{"id":"C_JOINED","name":"Pilot","is_private":true}]}`)); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	client := New(server.Client(), "fixture", "TEAM", "REP")
	client.http.Base = server.URL
	channels, next, err := client.Channels(context.Background(), "")
	if err != nil || len(channels) != 1 || channels[0].ID != "C_JOINED" || next != "" {
		t.Fatalf("joined channel hidden: %+v %q %v", channels, next, err)
	}
}

func TestPeopleCanBeFoundByTheirSlackHandle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte(`{"ok":true,"members":[{"id":"U_OPS","team_id":"TEAM","name":"pilot-ops","real_name":"Jordan Lee","profile":{"display_name":"JL"}}]}`)); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	client := New(server.Client(), "fixture", "TEAM", "REP")
	client.http.Base = server.URL
	people, _, err := client.People(context.Background(), "pilot-ops", "")
	if err != nil || len(people) != 1 || people[0].Name != "Jordan Lee" {
		t.Fatalf("native handle not found: %+v %v", people, err)
	}
}
