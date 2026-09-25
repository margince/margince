// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graph

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// A mailbox with many folders answers in pages, and stopping at the first would
// silently offer a picker part of somebody's mailbox — which reads as "that
// folder cannot be excluded" rather than as a truncated list.
func TestListFoldersFollowsEveryPage(t *testing.T) {
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/me/mailFolders", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			writeJSON(w, map[string]any{
				"value": []map[string]string{{"id": "f2", "displayName": "Familie"}},
			})
			return
		}
		writeJSON(w, map[string]any{
			"value":           []map[string]string{{"id": "f1", "displayName": "Privat"}},
			"@odata.nextLink": srv.URL + "/me/mailFolders?page=2",
		})
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	got, err := NewAPI(srv.Client(), srv.URL).ListFolders(context.Background(), "access-1")
	if err != nil {
		t.Fatalf("ListFolders: %v", err)
	}
	if len(got) != 2 || got[0].Name != "Privat" || got[1].Name != "Familie" {
		t.Fatalf("got %+v, want both pages in the order the provider gave them", got)
	}
}

// A nextLink is a URL the provider chose. Following one off-origin would send
// this mailbox's token somewhere Microsoft did not.
func TestListFoldersRefusesAnOffOriginNextLink(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/me/mailFolders", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"value":           []map[string]string{{"id": "f1", "displayName": "Privat"}},
			"@odata.nextLink": "https://attacker.example/steal-token",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	if _, err := NewAPI(srv.Client(), srv.URL).ListFolders(context.Background(), "access-1"); err == nil {
		t.Fatal("an off-origin nextLink was followed")
	}
}

// A folder with no display name falls back to its id, and one with no id is
// dropped — a rule keyed on nothing would exclude nothing.
func TestListFoldersNamesWhatItCanAndDropsWhatItCannot(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/me/mailFolders", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"value": []map[string]string{
			{"id": "f1", "displayName": ""},
			{"id": "", "displayName": "Ghost"},
		}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	got, err := NewAPI(srv.Client(), srv.URL).ListFolders(context.Background(), "access-1")
	if err != nil {
		t.Fatalf("ListFolders: %v", err)
	}
	if len(got) != 1 || got[0].Name != "f1" {
		t.Fatalf("got %+v, want the id standing in for the missing name and the id-less folder gone", got)
	}
}

// The connector's own verb: it opens the auth state, refreshes, and asks.
func TestListContainersReadsTheMailboxsFolders(t *testing.T) {
	api := &fakeAPI{folders: []connector.NamedContainer{{ID: "f1", Name: "Privat"}}}
	c := pinnedConn(api)

	got, err := c.ListContainers(context.Background(), authBytes(t))
	if err != nil {
		t.Fatalf("ListContainers: %v", err)
	}
	if len(got) != 1 || got[0].Name != "Privat" {
		t.Fatalf("got %+v, want the mailbox's own folder", got)
	}
}

// Malformed auth is a fault the caller must see rather than an empty list: an
// empty picker reads as "this mailbox has no folders", a different and wrong
// answer.
func TestListContainersRefusesMalformedAuth(t *testing.T) {
	c := pinnedConn(&fakeAPI{})
	if _, err := c.ListContainers(context.Background(), []byte("not json")); err == nil {
		t.Fatal("a malformed auth bundle listed containers instead of failing")
	}
}
