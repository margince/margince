// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// callOverMCP posts one tools/call as an agent holding every grant, and answers
// the body the client reads.
func callOverMCP(t *testing.T, name string) string {
	t.Helper()
	h := NewHTTPHandler(fullRegistry(t), func(*http.Request) (context.Context, error) { return agentCtx(), nil },
		func(*http.Request) string { return "" }, "margince-crm", "test", slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	srv := httptest.NewServer(h)
	defer srv.Close()
	resp, err := http.Post(srv.URL, "application/json",
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"`+name+`","arguments":{}}}`))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("closing the response: %v", err)
		}
	}()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading the response: %v", err)
	}
	return string(body)
}

// A client calls only what tools/list offers it. A name the listing leaves out
// answers as a name that does not exist, whether a router's target or a
// superseded tool. The refusal tells a caller nothing the listing did not.
func TestAToolTheListingLeavesOutIsRefusedAtTheDoor(t *testing.T) {
	for _, hidden := range []string{"advance_deal", "relink_thread", "list_records"} {
		if answer := callOverMCP(t, hidden); !strings.Contains(answer, "unknown tool "+hidden) {
			t.Errorf("a call to the hidden %s was served: %s", hidden, answer)
		}
	}
	if answer := callOverMCP(t, "change_tags"); strings.Contains(answer, "unknown tool") {
		t.Errorf("the router the listing offers was refused at the door: %s", answer)
	}
}

// servingUnofferedNames lets one test call a verb by its own name, as a test of
// that verb's arguments does. The test above holds the door.
func servingUnofferedNames(t *testing.T) {
	t.Helper()
	servesUnoffered.Store(true)
	t.Cleanup(func() { servesUnoffered.Store(false) })
}
