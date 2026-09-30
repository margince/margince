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
// keys it was asked for, plus the ones it was asked to confirm.
type scriptedClaims struct {
	grant      bool
	asked      []string
	confirmed  []string
	confirmErr error
}

func (c *scriptedClaims) Claim(_ context.Context, handler, occurrenceKey, fingerprint string) (bool, error) {
	c.asked = append(c.asked, handler+"|"+occurrenceKey+"|"+fingerprint)
	return c.grant, nil
}

func (c *scriptedClaims) Confirm(_ context.Context, handler, occurrenceKey, fingerprint string) error {
	c.confirmed = append(c.confirmed, handler+"|"+occurrenceKey+"|"+fingerprint)
	return c.confirmErr
}

// refusingProvider fails every create, so what the claim does about a create
// that did not land is observable.
type refusingProvider struct {
	datasource.SystemOfRecordProvider
	err error
}

func (p *refusingProvider) Create(context.Context, datasource.CreateInput) (datasource.EntityRef, error) {
	return datasource.EntityRef{}, p.err
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

// A create that LANDED confirms its claim, so no later firing reclaims it.
func TestAnAppliedCreateConfirmsItsClaim(t *testing.T) {
	claims := &scriptedClaims{grant: true}
	provider := &unitCountingProvider{}

	if _, err := ApplyActions(context.Background(),
		Executors{Provider: provider, Claims: claims}, createTaskEffect(t, "route_lead")); err != nil {
		t.Fatalf("ApplyActions: %v", err)
	}

	if provider.created != 1 {
		t.Fatalf("created %d records, want 1", provider.created)
	}
	if len(claims.confirmed) != 1 || claims.confirmed[0] != claims.asked[0] {
		t.Errorf("confirmed %v, want the one claim it took (%v)", claims.confirmed, claims.asked)
	}
}

// A create that FAILED leaves its claim unconfirmed — which is the whole point.
//
// Confirming here would deduplicate every future firing against a record that
// does not exist, and the task would be lost with nothing to repair it. Left
// unconfirmed, the store's lease collects the claim and a redelivery applies it.
func TestAFailedCreateLeavesItsClaimUnconfirmed(t *testing.T) {
	claims := &scriptedClaims{grant: true}
	wanted := errors.New("the provider refused")

	_, err := ApplyActions(context.Background(),
		Executors{Provider: &refusingProvider{err: wanted}, Claims: claims}, createTaskEffect(t, "route_lead"))

	if !errors.Is(err, wanted) {
		t.Fatalf("err = %v, want the provider's own", err)
	}
	if len(claims.confirmed) != 0 {
		t.Errorf("confirmed %v after a create that never landed — a later firing would now fold against nothing", claims.confirmed)
	}
}

// A folded firing confirms nothing: it took no claim, so it has no record to
// vouch for, and confirming would vouch for somebody else's in-flight create.
func TestAFoldedFiringConfirmsNothing(t *testing.T) {
	claims := &scriptedClaims{grant: false}
	provider := &unitCountingProvider{}

	if _, err := ApplyActions(context.Background(),
		Executors{Provider: provider, Claims: claims}, createTaskEffect(t, "route_lead")); err != nil {
		t.Fatalf("ApplyActions: %v", err)
	}

	if provider.created != 0 {
		t.Fatal("a folded firing created a record")
	}
	if len(claims.confirmed) != 0 {
		t.Errorf("a folded firing confirmed %v", claims.confirmed)
	}
}

// A confirm that fails does not fail the firing. The record LANDED; re-running
// would create it twice, and the unconfirmed claim costs at most one duplicate
// after the lease — the smaller of the two wrongs.
func TestAFailedConfirmDoesNotFailAFiringWhoseRecordLanded(t *testing.T) {
	claims := &scriptedClaims{grant: true, confirmErr: errors.New("the claim row is unreachable")}
	provider := &unitCountingProvider{}

	if _, err := ApplyActions(context.Background(),
		Executors{Provider: provider, Claims: claims}, createTaskEffect(t, "route_lead")); err != nil {
		t.Fatalf("a failed confirm must not fail a firing whose create succeeded: %v", err)
	}
	if provider.created != 1 {
		t.Fatalf("created %d records, want 1", provider.created)
	}
}
