// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package agentaccess

// A retry key and a task handle belong to the connected agent, not to one
// passport row. A refresh retires the passport and mints a replacement under the
// same grant, and the retry or the poll that follows often arrives on it.

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// renewedAccess presents the refresh token and returns the pair it was swapped for.
func (o *oauthEnv) renewedAccess(t *testing.T, refreshToken string) (accessToken, nextRefresh string) {
	t.Helper()
	status, body := o.renew(t, refreshToken, nil)
	if status != http.StatusOK {
		t.Fatalf("refresh → %d %v", status, body)
	}
	accessToken, _ = body["access_token"].(string)
	nextRefresh, _ = body["refresh_token"].(string)
	if accessToken == "" || nextRefresh == "" {
		t.Fatalf("the refresh left the client without credentials: %v", body)
	}
	return accessToken, nextRefresh
}

func (o *oauthEnv) createContactTool(t *testing.T, accessToken, name, key string) string {
	t.Helper()
	call := o.rpc(t, accessToken, "", fmt.Sprintf(
		`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"create_record","arguments":{`+
			`"record_type":"contact","idempotency_key":%q,"fields":{"full_name":%q,`+
			`"linkedin_url":"https://www.linkedin.com/in/fixture-refresh-retry"}}}}`, key, name))
	return toolText(t, call)
}

func TestAToolRetryAfterATokenRefreshReplaysTheFirstWrite(t *testing.T) {
	o := setupOAuth(t)
	access, refresh := o.connect(t)
	name := "Refresh Retry Tool Contact " + ids.NewV7().String()

	first := o.createContactTool(t, access, name, "lost-answer-1")
	access, _ = o.renewedAccess(t, refresh)
	retry := o.createContactTool(t, access, name, "lost-answer-1")

	if retry != first {
		t.Errorf("the retry on the refreshed token answered\n%s\nwant the first call's answer\n%s", retry, first)
	}
	assertOwnerCount(t, o, 1, `SELECT count(*) FROM contact WHERE full_name = $1`, name)
}

func TestARESTRetryAfterATokenRefreshReplaysTheFirstWrite(t *testing.T) {
	o := setupOAuth(t)
	access, refresh := o.connect(t)
	name := "Refresh Retry REST Contact " + ids.NewV7().String()
	create := func(bearer string) string {
		var created struct {
			ID string `json:"id"`
		}
		headers := map[string]string{"Authorization": "Bearer " + bearer, "Idempotency-Key": "lost-answer-2"}
		if status := o.Call(t, "POST", "/v1/contacts", integration.AnyMap{
			"full_name": name, "source": "manual",
			"emails": []integration.AnyMap{{"email": "retry@visitor.example", "email_type": "work", "is_primary": true}},
		},
			headers, &created); status != http.StatusCreated || created.ID == "" {
			t.Fatalf("agent contact create → %d %+v", status, created)
		}
		return created.ID
	}

	first := create(access)
	access, _ = o.renewedAccess(t, refresh)
	if retry := create(access); retry != first {
		t.Errorf("the retry on the refreshed token created %s, want the first call's %s replayed", retry, first)
	}
	assertOwnerCount(t, o, 1, `SELECT count(*) FROM contact WHERE full_name = $1`, name)
}

// taskHandle reads the handle a task-mode call answered with.
var taskHandle = regexp.MustCompile(`"taskId":"([0-9a-f-]{36})"`)

func TestATaskHandleOutlivesItsConnectionsTokenRefresh(t *testing.T) {
	o := setupOAuth(t)
	code := o.authorize(t, url.Values{"scope": {"read enrich offline_access"}})
	_, tokens := o.exchange(t, url.Values{"code": {code}})
	access, _ := tokens["access_token"].(string)
	refresh, _ := tokens["refresh_token"].(string)
	var company struct {
		ID string `json:"id"`
	}
	if status := o.Call(t, "POST", "/v1/companies", integration.AnyMap{
		"source": "manual", "display_name": "Task Handle GmbH",
		"domains": []integration.AnyMap{{"domain": "task-handle.test", "is_primary": true}},
	}, nil, &company); status != http.StatusCreated {
		t.Fatalf("company create → %d", status)
	}
	bearer := func(token string) map[string]string { return map[string]string{"Authorization": "Bearer " + token} }

	// enrich waits for a human even on its user's passport, so task mode hands back a handle.
	staged := mcpRaw(t, o.AppEnv, http.MethodPost, "/mcp", fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{%s,"name":"enrich","arguments":{"company_id":%q}}}`,
		tasksMeta, company.ID), modernHeaders(bearer(access), "tools/call", "enrich"))
	handle := taskHandle.FindStringSubmatch(staged.Body)
	if handle == nil {
		t.Fatalf("a task-mode enrich answered no handle: %d %s", staged.StatusCode, staged.Body)
	}

	access, _ = o.renewedAccess(t, refresh)
	polled := mcpRaw(t, o.AppEnv, http.MethodPost, "/mcp", fmt.Sprintf(
		`{"jsonrpc":"2.0","id":2,"method":"tasks/get","params":{%s,"taskId":%q}}`, tasksMeta, handle[1]),
		modernHeaders(bearer(access), "tasks/get", handle[1]))
	if result := rpcResult(t, polled.Body); result["status"] != "working" {
		t.Fatalf("the refreshed token polled its connection's task and got %v", result)
	}
}
