// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func largeBodyHandler(t *testing.T, registry *Registry) (*httpMCPHandler, *httptest.Server) {
	t.Helper()
	h, ok := NewHTTPHandler(registry, func(r *http.Request) (context.Context, error) {
		return principal.WithActor(principal.WithWorkspaceID(r.Context(), ids.NewV7()), principal.Principal{
			Type: principal.PrincipalAgent, ID: "agent:large", OnBehalfOf: ids.NewV7(),
			Scopes: principal.NewScopeSet(principal.ScopeRead),
		}), nil
	}, func(*http.Request) string { return "" }, "margince-crm", "test", discardLog()).(*httpMCPHandler)
	if !ok {
		t.Fatal("NewHTTPHandler no longer answers the MCP handler")
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return h, srv
}

// stalledAnswer is what the server said to a sender whose body never finished.
type stalledAnswer struct {
	status int
	header http.Header
	body   []byte
	// closes is whether the server said it closes the connection; ReadResponse
	// moves Connection: close out of the header and into this.
	closes bool
}

// stalledPost writes a POST's head and then only part of its body, and reads
// what the server answers while the rest never arrives.
func stalledPost(t *testing.T, srv *httptest.Server, head, sent string) stalledAnswer {
	t.Helper()
	conn, err := net.Dial("tcp", srv.Listener.Addr().String())
	if err != nil {
		t.Fatalf("dialling the server: %v", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			t.Errorf("closing the connection: %v", err)
		}
	}()
	// A guard, not a wait: a server that answers does so long before it, and
	// one that is still reading would otherwise hang the test.
	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatalf("setting the guard deadline: %v", err)
	}
	if _, err := fmt.Fprintf(conn, "POST / HTTP/1.1\r\nHost: mcp\r\nContent-Type: application/json\r\n%s\r\n%s",
		head, sent); err != nil {
		t.Fatalf("writing the request: %v", err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("no answer while the body was still owed: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("closing the response body: %v", err)
		}
	}()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading the answer: %v", err)
	}
	return stalledAnswer{status: resp.StatusCode, header: resp.Header, body: body, closes: resp.Close}
}

// chunkedPastTheBound is a chunked body that has just outgrown the ordinary
// bound and then stops, owing the rest.
var chunkedPastTheBound = fmt.Sprintf("%x\r\n%s\r\n", httperr.MaxBodyBytes+1, strings.Repeat(" ", httperr.MaxBodyBytes+1))

// A large request that finds every slot taken is refused at once, before its
// body is read and without the server waiting to drain the rest of it.
func TestABusyLargeRequestIsRefusedAtOnce(t *testing.T) {
	for name, tc := range map[string]struct{ head, sent string }{
		"a declared length": {head: fmt.Sprintf("Content-Length: %d\r\n", MaxMCPRequestBytes)},
		"chunked":           {head: "Transfer-Encoding: chunked\r\n", sent: chunkedPastTheBound},
	} {
		t.Run(name, func(t *testing.T) {
			h, srv := largeBodyHandler(t, NewRegistry(nil, nil))
			for range maxLargeMCPBodiesInFlight {
				h.largeBodies <- struct{}{}
			}

			answer := stalledPost(t, srv, tc.head, tc.sent)

			if answer.status != http.StatusServiceUnavailable || answer.header.Get("Retry-After") == "" {
				t.Fatalf("answered %d (Retry-After %q), want 503 with Retry-After", answer.status, answer.header.Get("Retry-After"))
			}
			if !answer.closes {
				t.Error("the refusal does not close the connection, so the server would wait to drain the unsent body")
			}
			var problem struct{ Code, Detail string }
			if err := json.Unmarshal(answer.body, &problem); err != nil || problem.Detail == "" {
				t.Errorf("the refusal is not a readable problem (%+v): %v", problem, err)
			}
		})
	}
}

// One agent holds one large slot, so a single passport cannot take every slot
// and turn away every other workspace's file.
func TestOneAgentHoldsOneLargeSlot(t *testing.T) {
	h, srv := largeBodyHandler(t, NewRegistry(nil, nil))
	h.largeBodyReadBudget = 20 * time.Millisecond
	h.holdLargeSlotFor(t, "agent:large")

	answer := stalledPost(t, srv, fmt.Sprintf("Content-Length: %d\r\n", MaxMCPRequestBytes), "")

	if answer.status != http.StatusTooManyRequests || !answer.closes {
		t.Fatalf("a second large call from one agent answered %d (closes %t), want 429 and a closed connection",
			answer.status, answer.closes)
	}

	h.releaseLargeSlotOf(t, "agent:large")
	h.holdLargeSlotFor(t, "agent:other")

	if answer := stalledPost(t, srv, fmt.Sprintf("Content-Length: %d\r\n", MaxMCPRequestBytes), "{"); answer.status != http.StatusRequestTimeout {
		t.Errorf("another agent's large call answered %d, want it admitted and then timed out (408)", answer.status)
	}
	h.largeMu.Lock()
	_, held := h.largeHolders["agent:large"]
	h.largeMu.Unlock()
	if held {
		t.Error("the agent still holds a large slot after it was answered")
	}
}

// holdLargeSlotFor and releaseLargeSlotOf stand in for a call in flight.
func (h *httpMCPHandler) holdLargeSlotFor(t *testing.T, id string) {
	t.Helper()
	h.largeMu.Lock()
	defer h.largeMu.Unlock()
	h.largeHolders[id] = struct{}{}
	h.largeBodies <- struct{}{}
}

func (h *httpMCPHandler) releaseLargeSlotOf(t *testing.T, id string) {
	t.Helper()
	h.largeMu.Lock()
	defer h.largeMu.Unlock()
	delete(h.largeHolders, id)
	<-h.largeBodies
}

// A body declared over the MCP limit is refused for its size, never for want
// of a slot it could not use.
func TestAnOversizedDeclaredRequestIsRefusedBeforeASlot(t *testing.T) {
	h, srv := largeBodyHandler(t, NewRegistry(nil, nil))
	for range maxLargeMCPBodiesInFlight {
		h.largeBodies <- struct{}{}
	}

	answer := stalledPost(t, srv, fmt.Sprintf("Content-Length: %d\r\n", MaxMCPRequestBytes+1), "")

	if answer.status != http.StatusRequestEntityTooLarge {
		t.Errorf("an oversized declared body answered %d, want 413: %s", answer.status, answer.body)
	}
}

// A sender that stops partway holds its slot only for the read budget; the
// slot is free again once it is answered, whether it declared a length or not.
func TestAStalledLargeSenderGivesItsSlotBack(t *testing.T) {
	for name, tc := range map[string]struct{ head, sent string }{
		"a declared length": {head: fmt.Sprintf("Content-Length: %d\r\n", 2*httperr.MaxBodyBytes), sent: "{"},
		"chunked": {
			head: "Transfer-Encoding: chunked\r\n",
			sent: chunkedPastTheBound,
		},
	} {
		t.Run(name, func(t *testing.T) {
			h, srv := largeBodyHandler(t, NewRegistry(nil, nil))
			h.largeBodyReadBudget = 20 * time.Millisecond

			answer := stalledPost(t, srv, tc.head, tc.sent)

			if answer.status != http.StatusRequestTimeout {
				t.Errorf("a stalled sender was answered %d, want 408: %s", answer.status, answer.body)
			}
			if held := len(h.largeBodies); held != 0 {
				t.Errorf("%d slots are still held after the stalled sender was answered", held)
			}
		})
	}
}

// blockingTool answers only when released, so a test can look at the handler
// while a call is in flight.
type blockingTool struct {
	echoTool
	entered, release chan struct{}
}

func (b blockingTool) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	close(b.entered)
	<-b.release
	return b.echoTool.Handle(ctx, in)
}

func TestALargeRequestHoldsItsSlotUntilItIsAnswered(t *testing.T) {
	registry := NewRegistry(nil, auth.NewGate(fullSeatAuthority{}))
	tool := blockingTool{
		echoTool: echoTool{spec: objectSpec("read_record", principal.ScopeRead), out: json.RawMessage(`{"ok":true}`)},
		entered:  make(chan struct{}), release: make(chan struct{}),
	}
	registry.Register(tool)
	h, srv := largeBodyHandler(t, registry)

	req, err := http.NewRequest(http.MethodPost, srv.URL,
		strings.NewReader(modernCallBody+strings.Repeat(" ", httperr.MaxBodyBytes)))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	for name, value := range modernCallHeaders() {
		req.Header.Set(name, value)
	}
	answered := make(chan error, 1)
	go func() {
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			err = resp.Body.Close()
			if err == nil && resp.StatusCode != http.StatusOK {
				err = fmt.Errorf("answered %d, want 200", resp.StatusCode)
			}
		}
		answered <- err
	}()
	<-tool.entered
	if held := len(h.largeBodies); held != 1 {
		t.Errorf("%d slots are held while a large call runs, want 1", held)
	}
	close(tool.release)
	if err := <-answered; err != nil {
		t.Fatalf("the large call: %v", err)
	}
	if held := len(h.largeBodies); held != 0 {
		t.Errorf("%d slots are held after the large call was answered, want 0", held)
	}
}
