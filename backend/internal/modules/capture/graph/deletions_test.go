// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graph

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFolderTombstonesDoNotAuthorizeDeletingAnArchivedMessage(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Prefer") != immutableMessageIDs {
			t.Error("delta omitted immutable ids")
		}
		if r.URL.Path != "/me/mailFolders/inbox/messages/delta" {
			t.Errorf("folder removal triggered a mailbox absence lookup: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		writeJSON(w, map[string]any{"value": []map[string]any{{"id": "archived-id", "@removed": map[string]string{"reason": "deleted"}}}, "@odata.deltaLink": server.URL + "/next"})
	}))
	defer server.Close()
	api := NewAPI(server.Client(), server.URL)
	messages, removed, next, err := api.Delta(context.Background(), "token", server.URL+"/me/mailFolders/inbox/messages/delta")
	if err != nil || len(messages) != 0 || len(removed) != 0 || next != server.URL+"/next" {
		t.Fatalf("ambiguous tombstone authorized deletion: messages=%v removed=%v next=%s err=%v", messages, removed, next, err)
	}
}
