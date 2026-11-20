// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package agentaccess

// A retry key belongs to the connected agent, not to one passport row. A refresh
// retires the passport and mints a replacement under the same grant, and the
// retry of a write whose answer was lost often arrives on that replacement.

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
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
	const name = "Refresh Retry Tool Contact"

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
	const name = "Refresh Retry REST Contact"
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
