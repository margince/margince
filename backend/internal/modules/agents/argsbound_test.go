// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// The transport admits 8 MiB for attach_document; every other tool keeps 1 MiB.
func TestAnOrdinaryToolRefusesArgumentsOverTheBodyBound(t *testing.T) {
	tool := &fakeTool{spec: mcp.ToolSpec{
		Name: "read_record", Title: "read_record", Version: testToolVersion, Description: describedForRegistration,
		RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute,
		InputSchema: json.RawMessage(`{"type":"object","properties":{"note":{"type":"string"}}}`),
	}}
	r := NewRegistry(nil, auth.NewGate(fullSeatAuthority{}))
	r.Register(tool)
	args := `{"note":"` + strings.Repeat("x", httperr.MaxBodyBytes) + `"}`

	_, err := r.Invoke(scopedAgentCtx(principal.ScopeRead), "read_record", json.RawMessage(args))
	var bad *BadArgsError
	if !errors.As(err, &bad) || !strings.Contains(bad.Error(), "1.0 MB this tool accepts") {
		t.Fatalf("err = %v, want a refusal naming the 1 MiB bound", err)
	}
	if tool.handled {
		t.Error("the oversize call reached the handler")
	}
}

func TestAttachDocumentTakesArgumentsPastTheOrdinaryBound(t *testing.T) {
	docs := &fakeDocuments{}
	file := make([]byte, 3*httperr.MaxBodyBytes/2)
	args := attachCall(t, map[string]any{"content_base64": base64.StdEncoding.EncodeToString(file)})
	if len(args) <= 2*httperr.MaxBodyBytes {
		t.Fatalf("the call is %d bytes, which does not test past the ordinary bound", len(args))
	}
	if _, err := documentsRegistry(docs).Invoke(docsCtx(), "attach_document", args); err != nil {
		t.Fatalf("a %d-byte attach was refused: %v", len(args), err)
	}
	if len(docs.attached) != 1 || len(docs.attached[0].Content) != len(file) {
		t.Error("the file did not reach the store whole")
	}
}
