// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graph

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"testing"
)

func TestTheDeletionsRoundReadsRecoverableItemsDeletions(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/me/mailFolders/recoverableitemsdeletions/messages/delta" || r.URL.Query().Get("$select") != "id,internetMessageId" {
			t.Errorf("unexpected request %s", r.URL)
		}
		if r.Header.Get("Prefer") != immutableMessageIDs {
			t.Error("the Deletions round omitted immutable ids")
		}
		writeJSON(w, map[string]any{
			"value": []map[string]any{
				{"id": "hard-deleted", "internetMessageId": "<h@mail.example>"},
				{"id": "restored", "@removed": map[string]string{"reason": "deleted"}},
			},
			"@odata.deltaLink": server.URL + "/next",
		})
	}))
	defer server.Close()
	var arrived []string
	next, err := NewAPI(server.Client(), server.URL).DeletionsDelta(context.Background(), "token", "", func(_ context.Context, page DeltaPage) error {
		for _, m := range page.Messages {
			arrived = append(arrived, m.ID+" "+m.InternetMessageID)
		}
		return nil
	})
	if err != nil || next != server.URL+"/next" {
		t.Fatalf("next=%q err=%v", next, err)
	}
	if len(arrived) != 1 || arrived[0] != "hard-deleted <h@mail.example>" {
		t.Fatalf("arrivals %v, want only hard-deleted; a message leaving Deletions is no arrival", arrived)
	}
}

func TestTheDeletionsListingStopsAtItsPageBound(t *testing.T) {
	pages := 0
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		pages++
		writeJSON(w, map[string]any{
			"value":           []map[string]string{{"id": "id-" + strconv.Itoa(pages)}},
			"@odata.nextLink": server.URL + "/me/mailFolders/recoverableitemsdeletions/messages?page=" + strconv.Itoa(pages+1),
		})
	}))
	defer server.Close()
	ids, err := NewAPI(server.Client(), server.URL).ListDeletions(context.Background(), "token")
	if err != nil {
		t.Fatal(err)
	}
	if pages != deletionsListMaxPages || len(ids) != deletionsListMaxPages {
		t.Fatalf("read %d pages and %d ids, want the bound %d", pages, len(ids), deletionsListMaxPages)
	}
}

func TestAMessageLookupTellsAMissingMessageFromAFault(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		body    map[string]any
		folder  string
		found   bool
		wantErr bool
	}{
		{"filed", http.StatusOK, map[string]any{"parentFolderId": "folder-x"}, "folder-x", true, false},
		{"not found", http.StatusNotFound, map[string]any{}, "", false, false},
		{"server fault", http.StatusInternalServerError, map[string]any{}, "", false, true},
		{"an answer naming no folder", http.StatusOK, map[string]any{}, "", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/me/messages/m1" {
					t.Errorf("unexpected request %s", r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				writeJSON(w, tc.body)
			}))
			defer server.Close()
			folder, found, err := NewAPI(server.Client(), server.URL).MessageFolder(context.Background(), "token", "m1")
			if folder != tc.folder || found != tc.found || (err != nil) != tc.wantErr {
				t.Fatalf("folder=%q found=%v err=%v", folder, found, err)
			}
			if tc.wantErr && !errors.Is(err, ErrUnreachable) {
				t.Errorf("a fault is not classed unreachable: %v", err)
			}
		})
	}
}

func TestAMessageSearchListsEveryFolderHoldingTheIdentity(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		body    map[string]any
		folders []string
		wantErr bool
	}{
		{"two copies", http.StatusOK, map[string]any{"value": []map[string]string{{"parentFolderId": "a"}, {"parentFolderId": "b"}}}, []string{"a", "b"}, false},
		{"no copy", http.StatusOK, map[string]any{"value": []map[string]string{}}, []string{}, false},
		{"server fault", http.StatusInternalServerError, map[string]any{}, nil, true},
		{"a copy naming no folder", http.StatusOK, map[string]any{"value": []map[string]string{{}}}, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/me/messages" || r.URL.Query().Get("$filter") != "internetMessageId eq '<o''k@mail.example>'" {
					t.Errorf("unexpected request %s", r.URL)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				writeJSON(w, tc.body)
			}))
			defer server.Close()
			folders, err := NewAPI(server.Client(), server.URL).FindMessage(context.Background(), "token", "<o'k@mail.example>")
			if !slices.Equal(folders, tc.folders) || (err != nil) != tc.wantErr {
				t.Fatalf("folders=%v err=%v", folders, err)
			}
			if tc.wantErr && !errors.Is(err, ErrUnreachable) {
				t.Errorf("a fault is not classed unreachable: %v", err)
			}
		})
	}
}
