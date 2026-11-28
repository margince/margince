// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package agentaccess

// A route-backed write takes a caller's retry key when its route claims one, and
// the repeat replays the first answer and charges nothing. Each call is its
// own trace, so the record the envelope carries is what must match.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/agentvolume"
	"github.com/margince/margince/backend/internal/platform/redistest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestARouteBackedWriteRepeatedUnderItsRetryKeyWritesOnce(t *testing.T) {
	// A real meter, so the test sees what the replay charged.
	e := setupConnectorWith(t, compose.WithAgentVolume(agentvolume.New(redistest.Client(t), agentvolume.Limits{}, time.Hour).WithCompany(agentvolume.FixedCompany(ids.UUID{1}))))
	bearer, passport := apptest.PassportWithID(t, e.AppEnv, "retrying client", "read", "write")
	// Unique per run, because a lead origin's label is unique and the lane repeats tests.
	run := ids.NewV7().String()
	label, key := "Retry Keyed Lead Origin "+run, "lost-answer-"+run
	create := func() string {
		got := mcpRaw(t, e.AppEnv, http.MethodPost, "/mcp", fmt.Sprintf(
			`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"change_settings","arguments":{`+
				`"action":"create_lead_origin","idempotency_key":%q,"request_body":{"label":%q}}}}`, key, label),
			withContentType(bearer))
		if got.StatusCode != http.StatusOK {
			t.Fatalf("tools/call → %d %s", got.StatusCode, got.Body)
		}
		var answer struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal([]byte(toolText(t, rpcResult(t, got.Body))), &answer); err != nil {
			t.Fatalf("the answer is not an envelope: %v", err)
		}
		return string(answer.Data)
	}

	first := create()
	if retry := create(); retry != first {
		t.Errorf("the retry answered\n%s\nwant the first call's answer\n%s", retry, first)
	}
	var count int
	if err := e.Owner.QueryRow(t.Context(), `SELECT count(*) FROM lead_origin WHERE label = $1`, label).Scan(&count); err != nil {
		t.Fatalf("counting lead origins: %v", err)
	}
	if count != 1 {
		t.Fatalf("%d lead origins named %q, want the one the first call wrote", count, label)
	}
	// The replay changed nothing, so it spent no second write.
	var listed struct {
		Data []struct {
			ID    string `json:"id"`
			Usage struct {
				Writes struct {
					Used int `json:"used"`
				} `json:"writes"`
			} `json:"usage"`
		} `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/passports", nil, nil, &listed); status != http.StatusOK {
		t.Fatalf("passport list → %d", status)
	}
	spent := -1
	for _, p := range listed.Data {
		if p.ID == passport.String() {
			spent = p.Usage.Writes.Used
		}
	}
	if spent != 1 {
		t.Errorf("a run and its replay spent %d writes (-1: passport not listed), want 1", spent)
	}
}
