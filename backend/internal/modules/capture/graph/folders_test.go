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

	got, _, err := NewAPI(srv.Client(), srv.URL).ListFolders(context.Background(), "access-1")
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

	if _, _, err := NewAPI(srv.Client(), srv.URL).ListFolders(context.Background(), "access-1"); err == nil {
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

	got, _, err := NewAPI(srv.Client(), srv.URL).ListFolders(context.Background(), "access-1")
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

	got, _, err := c.ListContainers(context.Background(), authBytes(t))
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
	if _, _, err := c.ListContainers(context.Background(), []byte("not json")); err == nil {
		t.Fatal("a malformed auth bundle listed containers instead of failing")
	}
}

// The folder somebody wants kept out of capture is usually NOT at the root: it
// is under Inbox, where a mail client puts a folder you make while reading. A
// listing of the root alone offers everything except the folders somebody
// actually wants, so the walk descends — and a child carries its parents' names
// so the owner recognises it and two "Archive"s stay apart.
func TestListFoldersDescendsIntoChildFolders(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/me/mailFolders", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"value": []map[string]any{
				{"id": "inbox", "displayName": "Posteingang", "childFolderCount": 1},
				{"id": "sent", "displayName": "Gesendet", "childFolderCount": 0},
			},
		})
	})
	mux.HandleFunc("/me/mailFolders/inbox/childFolders", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"value": []map[string]any{
				{"id": "privat", "displayName": "Privat", "childFolderCount": 0},
			},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	got, _, err := NewAPI(srv.Client(), srv.URL).ListFolders(context.Background(), "access-1")
	if err != nil {
		t.Fatalf("ListFolders: %v", err)
	}
	byID := map[string]string{}
	for _, f := range got {
		byID[f.ID] = f.Name
	}
	if byID["privat"] != "Posteingang/Privat" {
		t.Fatalf("nested folder = %q, want it qualified by its parent", byID["privat"])
	}
	if byID["inbox"] != "Posteingang" || byID["sent"] != "Gesendet" {
		t.Fatalf("root folders = %+v, want them offered unqualified", byID)
	}
}

// A folder that reports no children is not asked for them: the walk spends one
// request per folder that has something below it, not per folder.
func TestListFoldersDoesNotAskLeavesForChildren(t *testing.T) {
	asked := false
	mux := http.NewServeMux()
	mux.HandleFunc("/me/mailFolders", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"value": []map[string]any{
				{"id": "sent", "displayName": "Gesendet", "childFolderCount": 0},
			},
		})
	})
	mux.HandleFunc("/me/mailFolders/sent/childFolders", func(w http.ResponseWriter, _ *http.Request) {
		asked = true
		writeJSON(w, map[string]any{"value": []map[string]any{}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	if _, _, err := NewAPI(srv.Client(), srv.URL).ListFolders(context.Background(), "access-1"); err != nil {
		t.Fatalf("ListFolders: %v", err)
	}
	if asked {
		t.Error("a folder with no children was asked for its children")
	}
}

// A tree that never bottoms out cannot turn one listing into an unbounded
// crawl: the walk stops and returns what it read, because a long list that
// stops is more useful to a picker than no list at all.
func TestListFoldersStopsOnAnEndlesslyNestedMailbox(t *testing.T) {
	mux := http.NewServeMux()
	// Every folder claims one child, and every child claims another.
	endless := func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"value": []map[string]any{
				{"id": "deeper", "displayName": "Tiefer", "childFolderCount": 1},
			},
		})
	}
	mux.HandleFunc("/me/mailFolders", endless)
	mux.HandleFunc("/me/mailFolders/deeper/childFolders", endless)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	got, _, err := NewAPI(srv.Client(), srv.URL).ListFolders(context.Background(), "access-1")
	if err != nil {
		t.Fatalf("ListFolders: %v", err)
	}
	if len(got) == 0 || len(got) > folderListMaxPages {
		t.Fatalf("got %d folders, want a bounded non-empty listing", len(got))
	}
}

// A WALK THAT RAN OUT OF BUDGET SAYS SO.
//
// The bound is generous and deliberate — a long list that stops beats no list
// at all — but the picker has to be told. Somebody whose folder is missing from
// a list presented as complete concludes the mailbox has no such folder, and
// stops looking for the thing they came to exclude.
func TestListFoldersSaysWhenTheBudgetRanOut(t *testing.T) {
	var srv *httptest.Server
	mux := http.NewServeMux()
	// Every page offers another, so the walk can only ever end by running out.
	mux.HandleFunc("/me/mailFolders", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"value":           []map[string]string{{"id": "f", "displayName": "Ordner"}},
			"@odata.nextLink": srv.URL + "/me/mailFolders?page=next",
		})
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	got, truncated, err := NewAPI(srv.Client(), srv.URL).ListFolders(context.Background(), "access-1")
	if err != nil {
		t.Fatalf("ListFolders: %v", err)
	}
	if len(got) != folderListMaxPages {
		t.Fatalf("read %d folder(s) from an endless mailbox, want the page budget of %d",
			len(got), folderListMaxPages)
	}
	if !truncated {
		t.Error("a walk that spent its whole page budget reports a complete mailbox")
	}
}

// A mailbox the walk finished reports complete, so the flag cannot become
// something every listing carries.
func TestListFoldersSaysNothingWhenItFinished(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/me/mailFolders", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"value": []map[string]string{{"id": "f1", "displayName": "Privat"}},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	got, truncated, err := NewAPI(srv.Client(), srv.URL).ListFolders(context.Background(), "access-1")
	if err != nil {
		t.Fatalf("ListFolders: %v", err)
	}
	if len(got) != 1 || truncated {
		t.Errorf("a finished walk over %d folder(s) reports truncated=%v", len(got), truncated)
	}
}
