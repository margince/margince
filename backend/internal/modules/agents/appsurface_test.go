// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// testSeatless stands in for the edge's no-seat refusal.
type testSeatless struct{}

func (testSeatless) Error() string { return "no seat" }

func (testSeatless) WriteRefusal(w http.ResponseWriter, r *http.Request) {
	httperr.Write(w, r, &httperr.DetailedError{Status: http.StatusForbidden, Code: "no_seat", Detail: "No member."})
}

// appServer serves a listed read tool, an unlisted write verb and a listed read
// tool. The app's surface names the first two.
func appServer(t *testing.T, seated bool) (*httptest.Server, map[string]*fakeTool) {
	t.Helper()
	tools := map[string]*fakeTool{}
	registry := NewRegistry(nil, auth.NewGate(fullSeatAuthority{}))
	for _, tool := range []struct {
		name     string
		scope    principal.Scope
		unlisted bool
	}{{"find", principal.ScopeRead, false}, {"verb_a", principal.ScopeWrite, true}, {"other", principal.ScopeRead, false}} {
		tools[tool.name] = &fakeTool{spec: mcp.ToolSpec{
			Name: tool.name, Title: tool.name, Version: testToolVersion, Unlisted: tool.unlisted,
			Description: tool.name + " is a tool an app may or may not call.", RequiredScope: tool.scope,
			Tier: mcp.TierAutoExecute, InputSchema: json.RawMessage(`{"type":"object"}`),
		}}
		registry.Register(tools[tool.name])
	}
	authenticate := func(*http.Request) (context.Context, error) {
		ctx := principal.WithWorkspaceID(context.Background(), ids.NewV7())
		surface := AppSurface{Tools: []string{"verb_a", "find", "unregistered"}}
		if !seated {
			surface.Seatless = testSeatless{}
			return WithAppSurface(ctx, surface), nil
		}
		return WithAppSurface(principal.WithActor(ctx, principal.Principal{
			Type: principal.PrincipalAgent, ID: "agent:app", OnBehalfOf: ids.NewV7(), SeatType: principal.SeatFull,
			Permissions: everyRecordedGrant(), Scopes: principal.NewScopeSet(principal.ScopeRead),
		}), surface), nil
	}
	server := httptest.NewServer(NewHTTPHandler(registry, authenticate,
		func(*http.Request) string { return "" }, "margince-crm", "test", discardLog()))
	t.Cleanup(server.Close)
	return server, tools
}

func appRPC(t *testing.T, server *httptest.Server, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("closing the body: %v", err)
		}
	}()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, string(raw)
}

func listedNames(t *testing.T, body string) []string {
	t.Helper()
	var decoded struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		t.Fatalf("decoding %s: %v", body, err)
	}
	var names []string
	for _, tool := range decoded.Result.Tools {
		names = append(names, tool.Name)
	}
	return names
}

const appToolsList = `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`

func appCall(name string) string {
	return `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"` + name + `","arguments":{}}}`
}

func TestAnAppIsListedItsFixedToolsWhateverTheCallersScopes(t *testing.T) {
	server, _ := appServer(t, true)
	_, body := appRPC(t, server, appToolsList)
	if got, want := listedNames(t, body), []string{"verb_a", "find"}; !slices.Equal(got, want) {
		t.Fatalf("the app was listed %v, want %v: its own names, an unlisted verb and a scope the caller lacks included", got, want)
	}
}

func TestAnAppCannotCallAToolOutsideItsList(t *testing.T) {
	server, tools := appServer(t, true)
	_, body := appRPC(t, server, appCall("other"))
	if !strings.Contains(body, `"isError":true`) || tools["other"].handled {
		t.Fatalf("an app called a tool outside its list: %s", body)
	}
	_, body = appRPC(t, server, appCall("find"))
	if strings.Contains(body, `"isError":true`) || !tools["find"].handled {
		t.Fatalf("an app could not call a tool on its list: %s", body)
	}
}

func TestAnAppCallStillMeetsTheCallersScopes(t *testing.T) {
	server, tools := appServer(t, true)
	_, body := appRPC(t, server, appCall("verb_a"))
	if !strings.Contains(body, `"isError":true`) || tools["verb_a"].handled {
		t.Fatalf("a listed write verb ran for a read-only passport: %s", body)
	}
}

func TestASeatlessAppCallerMayListAndNothingElse(t *testing.T) {
	server, tools := appServer(t, false)
	status, body := appRPC(t, server, appToolsList)
	if status != http.StatusOK || len(listedNames(t, body)) != 2 {
		t.Fatalf("a seatless caller's tools/list answered %d %s", status, body)
	}
	status, body = appRPC(t, server, appCall("find"))
	if status != http.StatusForbidden || !strings.Contains(body, "no_seat") || tools["find"].handled {
		t.Fatalf("a seatless caller's tools/call answered %d %s, want the 403 refusal", status, body)
	}
}

// Dispact refreshes an app with tools/list, resources/list and prompts/list,
// as whichever admin pressed refresh. An app is served tools alone, so the
// other two lists answer empty, seated or not.
func TestAnAppReadsEmptyResourceAndPromptListsSeatedOrNot(t *testing.T) {
	for _, seated := range []bool{false, true} {
		server, _ := appServer(t, seated)
		for method, key := range map[string]string{"resources/list": "resources", "resources/templates/list": "resourceTemplates", "prompts/list": "prompts"} {
			status, body := appRPC(t, server, `{"jsonrpc":"2.0","id":1,"method":"`+method+`"}`)
			if status != http.StatusOK || !strings.Contains(body, `"`+key+`":[]`) {
				t.Errorf("seated=%t %s answered %d %s, want an empty %s list", seated, method, status, body, key)
			}
		}
	}
}

func TestAnAppsToolListNamesNoView(t *testing.T) {
	server, _ := appServer(t, true)
	status, body := appRPC(t, server, appToolsList)
	if status != http.StatusOK || strings.Contains(body, "resourceUri") {
		t.Fatalf("an app's tools/list answered %d %s, want no view", status, body)
	}
}
