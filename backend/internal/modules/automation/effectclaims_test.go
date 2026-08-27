// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

// The claim-first create without a database: the seam contract
// (EffectClaims) is what makes these provable at unit speed — a lost
// claim folds instead of writing, a missing store fails loudly, and the
// fingerprint separates genuinely different effects.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// scriptedClaims answers every Claim with a fixed verdict and records the
// keys it was asked for.
type scriptedClaims struct {
	grant bool
	asked []string
}

func (c *scriptedClaims) Claim(_ context.Context, handler, occurrenceKey, fingerprint string) (bool, error) {
	c.asked = append(c.asked, handler+"|"+occurrenceKey+"|"+fingerprint)
	return c.grant, nil
}

// unitCountingProvider records creates; only Create is ever reached.
type unitCountingProvider struct {
	datasource.SystemOfRecordProvider
	created int
}

func (p *unitCountingProvider) Create(context.Context, datasource.CreateInput) (datasource.EntityRef, error) {
	p.created++
	return datasource.EntityRef{}, nil
}

func createTaskEffect(t *testing.T, handler string) workflow.Effect {
	t.Helper()
	args, err := json.Marshal(map[string]any{"kind": "task", "subject": "Follow up"})
	if err != nil {
		t.Fatal(err)
	}
	return workflow.Effect{
		Handler:       handler,
		OccurrenceKey: handler + ":occurrence-1",
		Actions: []workflow.Action{{
			Kind:   workflow.ActionCreateTask,
			Target: datasource.EntityRef{Type: "lead", ID: ids.NewV7()},
			Args:   args,
		}},
	}
}

func TestAnEngineCreateWithoutAClaimStoreFailsLoudly(t *testing.T) {
	provider := &unitCountingProvider{}
	_, err := ApplyActions(context.Background(), Executors{Provider: provider}, createTaskEffect(t, "route_lead"))
	if !errors.Is(err, ErrNoEffectClaims) {
		t.Fatalf("err = %v, want ErrNoEffectClaims — an unclaimed engine create is the N-copies bug reopened", err)
	}
	if provider.created != 0 {
		t.Fatal("the create ran despite the missing claim store")
	}
}

func TestALostClaimFoldsTheCreateAndSaysSo(t *testing.T) {
	provider := &unitCountingProvider{}
	claims := &scriptedClaims{grant: false}
	applied, err := ApplyActions(context.Background(), Executors{Provider: provider, Claims: claims}, createTaskEffect(t, "route_lead"))
	if err != nil {
		t.Fatalf("ApplyActions: %v", err)
	}
	if provider.created != 0 {
		t.Fatal("a lost claim still created the record — the dedupe deduplicates nothing")
	}
	if len(applied) != 1 || !applied[0].Deduplicated {
		t.Fatalf("applied = %+v, want the one action marked Deduplicated — a fold the trace cannot see is a write the run row lies about", applied)
	}
}

func TestAWonClaimCreatesUnmarked(t *testing.T) {
	provider := &unitCountingProvider{}
	claims := &scriptedClaims{grant: true}
	applied, err := ApplyActions(context.Background(), Executors{Provider: provider, Claims: claims}, createTaskEffect(t, "route_lead"))
	if err != nil {
		t.Fatalf("ApplyActions: %v", err)
	}
	if provider.created != 1 {
		t.Fatalf("created = %d, want 1", provider.created)
	}
	if applied[0].Deduplicated {
		t.Fatal("the winning firing's action is marked Deduplicated — the trace disowns a write it made")
	}
}

func TestAnEffectOutsideTheEngineAppliesUnclaimed(t *testing.T) {
	provider := &unitCountingProvider{}
	eff := createTaskEffect(t, "")
	eff.Handler = ""
	applied, err := ApplyActions(context.Background(), Executors{Provider: provider}, eff)
	if err != nil {
		t.Fatalf("ApplyActions: %v", err)
	}
	if provider.created != 1 || len(applied) != 1 {
		t.Fatalf("created = %d, applied = %d — a non-engine caller keeps the pre-claim contract", provider.created, len(applied))
	}
}

func TestDifferentArgsFingerprintApart(t *testing.T) {
	target := datasource.EntityRef{Type: "lead", ID: ids.NewV7()}
	one, err := effectFingerprint(workflow.Action{
		Kind: workflow.ActionCreateTask, Target: target,
		Args: json.RawMessage(`{"due_at":"2026-08-29","subject":"Follow up"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	// The same effect with its keys reordered is the SAME effect…
	same, err := effectFingerprint(workflow.Action{
		Kind: workflow.ActionCreateTask, Target: target,
		Args: json.RawMessage(`{"subject":"Follow up","due_at":"2026-08-29"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if one != same {
		t.Fatal("key order split one effect into two fingerprints — canonicalization is not canonical")
	}
	// …and a different due date is a different effect.
	other, err := effectFingerprint(workflow.Action{
		Kind: workflow.ActionCreateTask, Target: target,
		Args: json.RawMessage(`{"due_at":"2026-08-31","subject":"Follow up"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if one == other {
		t.Fatal("two different parameterizations share a fingerprint — the claim would eat a legitimate create")
	}
}
