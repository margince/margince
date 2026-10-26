// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
)

const probeSource = extension.SourceName(ingressProbeSystem)

func TestABatchSavesItsPositionAfterEveryRecordIsAnswered(t *testing.T) {
	e := setupIngress(t)
	ctx := context.Background()
	member := extension.UserID(e.member.String())

	before, err := e.ingestingRuntime().IngestPosition(ctx, probeSource)
	if err != nil || before != "" {
		t.Fatalf("IngestPosition before any batch = %q, %v; want empty", before, err)
	}

	recs := []extension.Record{
		aProviderRecord("ws-7:1", "a@outside.test"),
		aProviderRecord("ws-7:2", "b@outside.test"),
	}
	results, err := e.ingestingRuntime().IngestBatch(ctx, member, probeSource, recs, "page-2")
	if err != nil || len(results) != 2 {
		t.Fatalf("IngestBatch = %d results, %v; want 2", len(results), err)
	}

	// A new Runtime is what a restarted job tick holds.
	saved, err := e.ingestingRuntime().IngestPosition(ctx, probeSource)
	if err != nil || saved != "page-2" {
		t.Fatalf("IngestPosition after restart = %q, %v; want page-2", saved, err)
	}
}

func TestAFailedBatchKeepsThePreviousPosition(t *testing.T) {
	e := setupIngress(t)
	ctx := context.Background()
	member := extension.UserID(e.member.String())
	first := []extension.Record{aProviderRecord("ws-7:1", "a@outside.test")}
	if _, err := e.ingestingRuntime().IngestBatch(ctx, member, probeSource, first, "page-1"); err != nil {
		t.Fatalf("first batch: %v", err)
	}

	// The probe source lands activities only, so the second record is refused
	// after the first has landed.
	refused := extension.Record{System: ingressProbeSystem, Key: "co-1", Company: &extension.CompanyFields{DisplayName: "Acme"}}
	second := []extension.Record{aProviderRecord("ws-7:2", "b@outside.test"), refused}
	results, err := e.ingestingRuntime().IngestBatch(ctx, member, probeSource, second, "page-2")
	if !errors.Is(err, extension.ErrIngressNotDeclared) {
		t.Fatalf("IngestBatch error = %v, want ErrIngressNotDeclared", err)
	}
	if len(results) != 1 {
		t.Errorf("results = %d, want the one record answered before the refusal", len(results))
	}
	saved, err := e.ingestingRuntime().IngestPosition(ctx, probeSource)
	if err != nil || saved != "page-1" {
		t.Errorf("position = %q, %v; want page-1, so the half-done batch is offered again", saved, err)
	}
}

func TestABatchOrPositionReadIsRefusedWhenMalformedUndeclaredAttendedOrExpired(t *testing.T) {
	e := setupIngress(t)
	ctx := context.Background()
	member := extension.UserID(e.member.String())
	recs := []extension.Record{aProviderRecord("ws-7:1", "a@outside.test")}

	if _, err := e.ingestingRuntime().IngestBatch(ctx, member, probeSource, recs, ""); !errors.Is(err, extension.ErrInvalid) {
		t.Errorf("a batch with no position answered %v, want ErrInvalid", err)
	}
	const other = extension.SourceName("not-declared")
	if _, err := e.ingestingRuntime().IngestBatch(ctx, member, other, nil, "p"); !errors.Is(err, extension.ErrIngressNotDeclared) {
		t.Errorf("an undeclared source answered %v, want ErrIngressNotDeclared", err)
	}
	if _, err := e.ingestingRuntime().IngestPosition(ctx, other); !errors.Is(err, extension.ErrIngressNotDeclared) {
		t.Errorf("IngestPosition on an undeclared source answered %v, want ErrIngressNotDeclared", err)
	}

	attended := runtimeFor(e.callCtx(e.WS), ingressUnit, "1.0.0", "invocation/probe", boundExtensionRuntime())
	if _, err := attended.IngestBatch(ctx, member, probeSource, recs, "p"); !errors.Is(err, extension.ErrAttendedIngest) {
		t.Errorf("an attended batch answered %v, want ErrAttendedIngest", err)
	}

	expired := e.ingestingRuntime()
	expired.release()
	if _, err := expired.IngestPosition(ctx, probeSource); !errors.Is(err, extension.ErrRuntimeExpired) {
		t.Errorf("IngestPosition on a released runtime answered %v, want ErrRuntimeExpired", err)
	}
	if _, err := expired.IngestBatch(ctx, member, probeSource, nil, "p"); !errors.Is(err, extension.ErrRuntimeExpired) {
		t.Errorf("an empty batch on a released runtime answered %v, want ErrRuntimeExpired", err)
	}
}
