// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package modelcontract_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents/runner"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// stepRecorder answers every step with a final answer and keeps the schema
// each step request was sent with.
type stepRecorder struct{ schemas []json.RawMessage }

func (s *stepRecorder) Complete(_ context.Context, req model.Request) (model.Response, runner.Meta, error) {
	s.schemas = append(s.schemas, req.ResponseSchema)
	return model.Response{Text: `{"final":{"summary":"done"}}`}, runner.Meta{ModelID: "test"}, nil
}

func (*stepRecorder) PromptWindow() int { return 0 }

// The in-app runner has no action catalog to read. So the step schema the
// provider enforces names every argument of a router, a rare action's too.
// Only the MCP listing leaves those out.
func TestTheRunnersStepSchemaNamesARareActionsArgument(t *testing.T) {
	registry := servedSurface()
	ctx := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:loop", OnBehalfOf: ids.NewV7(), SeatType: principal.SeatFull,
		Scopes: principal.NewScopeSet(principal.ScopeRead),
	})
	const router = "read_details"
	var rareArgument string
	for _, spec := range registry.Runnable(ctx) {
		if spec.Name == router {
			rareArgument = firstMemberLeftOut(t, spec, registry.ForListing(spec))
		}
	}
	if rareArgument == "" {
		t.Fatalf("the MCP listing of %s names every argument the runner is offered. Either the "+
			"runner reads the shortened listing, or no action of %s is rare and this case tests nothing", router, router)
	}
	brain := &stepRecorder{}
	if _, err := runner.New(registry, brain).Run(ctx, runner.Job{Goal: "read", Tools: []string{router}}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(brain.schemas) == 0 {
		t.Fatal("the run made no model call, so no step schema was checked")
	}
	if !strings.Contains(string(brain.schemas[0]), `"`+rareArgument+`"`) {
		t.Errorf("the runner's step schema for %s leaves out %s, which a rare action takes, so the run cannot send it", router, rareArgument)
	}
}

// firstMemberLeftOut is a member of full's input schema that listed lacks, or
// empty when there is none.
func firstMemberLeftOut(t *testing.T, full, listed mcp.ToolSpec) string {
	t.Helper()
	members := func(spec mcp.ToolSpec) map[string]json.RawMessage {
		var doc struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(spec.InputSchema, &doc); err != nil {
			t.Fatalf("%s: %v", spec.Name, err)
		}
		return doc.Properties
	}
	shown := members(listed)
	for name := range members(full) {
		if _, kept := shown[name]; !kept {
			return name
		}
	}
	return ""
}
