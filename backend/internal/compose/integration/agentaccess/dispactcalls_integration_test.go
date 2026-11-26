// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package agentaccess

// The Dispact app's calls over the composed /mcp. A call token signed by the
// connected install acts as the member it names, through their own passport.
// It sees only the app's fixed list. The token rules themselves are unit
// tests in dispactapp; this suite proves they are what the real route runs.

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/dispactapp"
	"github.com/margince/margince/backend/internal/compose/oidctoken/oidctokentest"
	"github.com/margince/margince/backend/internal/platform/redistest"
	"github.com/margince/margince/backend/internal/platform/replayguard"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	dispactTeam  = "team-dispact"
	dispactAdmin = "granter@fable.test"
)

type dispactEnv struct {
	*connectorEnv
	rig *oidctokentest.Rig
}

// setupDispact connects a Dispact bot whose address is the rig, so the rig's
// keys are the install's keys.
func setupDispact(t *testing.T) *dispactEnv {
	t.Helper()
	rig := oidctokentest.NewRig(t)
	e := setupConnectorWith(t, compose.WithDispactCalls(replayguard.New(redistest.Client(t)), true))
	if _, err := e.Owner.Exec(context.Background(), `
		INSERT INTO channel_connection (provider, channel_id, channel_label, address, credential_ref, status, connected_by)
		SELECT 'dispact', $1, 'Dispact', $2, 'vault:test', 'connected', id FROM app_user WHERE email = $3`,
		dispactTeam, rig.Srv.URL, dispactAdmin); err != nil {
		t.Fatalf("connecting the Dispact bot: %v", err)
	}
	return &dispactEnv{connectorEnv: e, rig: rig}
}

// token mints a call token valid now, for email, with over applied.
func (e *dispactEnv) token(t *testing.T, email string, over map[string]any) string {
	t.Helper()
	now := time.Now()
	claims := map[string]any{
		"iss": e.rig.Srv.URL, "aud": dispactapp.Audience, "sub": "dispact:" + email,
		"iat": now.Add(-5 * time.Second).Unix(), "exp": now.Add(55 * time.Second).Unix(),
		"jti": ids.NewV7().String(), "workspace_id": dispactTeam,
		"email": email, "email_verified": true, "display_name": "Someone",
	}
	maps.Copy(claims, over)
	return e.rig.Mint(t, oidctokentest.KID, "RS256", claims)
}

func (e *dispactEnv) call(t *testing.T, bearer, tool string) httpResult {
	t.Helper()
	return mcpRaw(t, e.AppEnv, http.MethodPost, "/mcp",
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"`+tool+`","arguments":{}}}`,
		map[string]string{"Content-Type": "application/json", "Authorization": "Bearer " + bearer})
}

func toolNames(t *testing.T, body string) []string {
	t.Helper()
	var decoded struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		t.Fatalf("decoding tools/list: %v (%s)", err, body)
	}
	var names []string
	for _, tool := range decoded.Result.Tools {
		names = append(names, tool.Name)
	}
	return names
}

func TestDispactListsTheAgreedToolsAndNothingElse(t *testing.T) {
	e := setupDispact(t)
	resp := listTools(t, e.AppEnv, e.token(t, dispactAdmin, nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("tools/list with a call token → %d %s", resp.StatusCode, resp.Body)
	}
	names := toolNames(t, resp.Body)
	for _, name := range dispactapp.Tools {
		// file_channel_message is listed once its verb is registered.
		if name != "file_channel_message" && !slices.Contains(names, name) {
			t.Errorf("the app's list is missing %s: no tool on this server answers to that name", name)
		}
	}
	for _, name := range names {
		if !slices.Contains(dispactapp.Tools, name) {
			t.Errorf("the app's list carries %s, which Dispact never agreed to", name)
		}
	}
}

func TestADispactCallActsThroughTheCallersOwnPassport(t *testing.T) {
	e := setupDispact(t)
	resp := e.call(t, e.token(t, dispactAdmin, nil), "whoami")
	if resp.StatusCode != http.StatusOK || strings.Contains(resp.Body, `"isError":true`) {
		t.Fatalf("whoami with a call token → %d %s", resp.StatusCode, resp.Body)
	}
	var onBehalfOf ids.UUID
	if err := e.Owner.QueryRow(context.Background(), `
		SELECT p.on_behalf_of FROM passport p JOIN app_user u ON u.id = p.on_behalf_of
		 WHERE p.app_issuer = $1 AND p.app_subject = $2 AND u.email = $3`,
		e.rig.Srv.URL, "dispact:"+dispactAdmin, dispactAdmin).Scan(&onBehalfOf); err != nil {
		t.Fatalf("no app passport was minted for the caller: %v", err)
	}
	if !strings.Contains(resp.Body, onBehalfOf.String()) {
		t.Fatalf("whoami did not answer for the caller's member %s: %s", onBehalfOf, resp.Body)
	}
}

func TestADispactCallCannotReachAToolOutsideTheList(t *testing.T) {
	e := setupDispact(t)
	resp := e.call(t, e.token(t, dispactAdmin, nil), "create_record")
	if !strings.Contains(resp.Body, `"isError":true`) || !strings.Contains(resp.Body, "unknown tool") {
		t.Fatalf("create_record through the Dispact app answered %d %s, want unknown tool", resp.StatusCode, resp.Body)
	}
}

func TestTheRouteRefusesABadCallToken(t *testing.T) {
	e := setupDispact(t)
	replayed := e.token(t, dispactAdmin, nil)
	if resp := listTools(t, e.AppEnv, replayed); resp.StatusCode != http.StatusOK {
		t.Fatalf("first use → %d %s", resp.StatusCode, resp.Body)
	}
	for name, bearer := range map[string]string{
		"a replayed token":     replayed,
		"an unconfirmed email": e.token(t, dispactAdmin, map[string]any{"email_verified": false}),
		"another team":         e.token(t, dispactAdmin, map[string]any{"workspace_id": "team-other"}),
		"another issuer":       e.token(t, dispactAdmin, map[string]any{"iss": "https://other.example.test"}),
	} {
		if resp := listTools(t, e.AppEnv, bearer); resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s → %d %s, want 401", name, resp.StatusCode, resp.Body)
		}
	}
}

func TestACallerWithNoMemberIsListedButRefusedA403(t *testing.T) {
	e := setupDispact(t)
	if resp := listTools(t, e.AppEnv, e.token(t, "stranger@elsewhere.test", nil)); resp.StatusCode != http.StatusOK {
		t.Fatalf("a seatless tools/list → %d %s, want the list", resp.StatusCode, resp.Body)
	}
	resp := e.call(t, e.token(t, "stranger@elsewhere.test", nil), "whoami")
	if resp.StatusCode != http.StatusForbidden || !strings.Contains(resp.Body, dispactapp.NoSeatCode) {
		t.Fatalf("a seatless tools/call → %d %s, want 403 %s", resp.StatusCode, resp.Body, dispactapp.NoSeatCode)
	}
}

func TestTheManifestNamesThisInstallationsMCPAddress(t *testing.T) {
	e := setupConnectorWith(t, compose.WithPublicBaseURL("https://crm.example.test"), compose.WithChatLinks(nil, true, nil))
	resp := mcpRaw(t, e.AppEnv, http.MethodGet, "/v1/integrations/dispact/manifest", "", nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(resp.Body, `"endpoint":"https://crm.example.test/mcp"`) {
		t.Fatalf("the manifest → %d %s", resp.StatusCode, resp.Body)
	}
}
