// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
)

func TestContactToolsRequireAChannelAndAcceptLinkedIn(t *testing.T) {
	t.Parallel()
	e := Setup(t)
	ctx := e.As(e.AdminUser, nil, AdminPerms)
	registry := compose.NewRegistry(e.Pool, compose.SendPath{})
	_, err := registry.Invoke(ctx, "create_record", json.RawMessage(`{"record_type":"contact","fields":{"full_name":"Tool Name Only"}}`))
	if err == nil {
		t.Fatal("a name-only tool contact was created")
	}
	for _, choice := range []string{"email", "phone", "LinkedIn"} {
		if !strings.Contains(err.Error(), choice) {
			t.Fatalf("missing %s choice: %v", choice, err)
		}
	}
	if n := e.WsCount(t, `SELECT count(*) FROM contact WHERE full_name = 'Tool Name Only'`); n != 0 {
		t.Fatalf("the refused tool wrote %d contacts", n)
	}
	_, err = registry.Invoke(ctx, "create_record", json.RawMessage(`{"record_type":"contact","fields":{"full_name":"Tool LinkedIn Contact","linkedin_url":"https://www.linkedin.com/in/tool-channel-contact"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM contact WHERE full_name = 'Tool LinkedIn Contact'`); n != 1 {
		t.Fatalf("the valid tool wrote %d contacts", n)
	}
}
