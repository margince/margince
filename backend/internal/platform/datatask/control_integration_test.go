// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package datatask

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestControlPreservesProgressAcrossPauseAndResume(t *testing.T) {
	ctx, pool, e, _ := executorFixture(t)
	before := progressFor(ctx, t, e)
	if before.RunID.IsZero() {
		t.Fatal("seeded task has no durable run identity")
	}
	if err := e.Control(ctx, before.TaskID, before.Step, "pause", before.Version); err != nil {
		t.Fatal(err)
	}
	paused := progressFor(ctx, t, e)
	if paused.Status != Paused || paused.RunID != before.RunID || paused.Version != before.Version+1 {
		t.Fatalf("pause replaced progress: %+v", paused)
	}
	if _, err := e.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	if got := progressFor(ctx, t, e); got.Batches != 0 || got.Version != paused.Version {
		t.Fatalf("paused task executed: %+v", got)
	}
	if err := e.Control(ctx, paused.TaskID, paused.Step, "retry", paused.Version); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("retry paused task: %v", err)
	}
	if err := e.Control(ctx, paused.TaskID, paused.Step, "resume", paused.Version); err != nil {
		t.Fatal(err)
	}
	resumed := progressFor(ctx, t, e)
	if resumed.Status != Pending || resumed.RunID != before.RunID || resumed.Version != paused.Version+1 {
		t.Fatalf("resume replaced progress: %+v", resumed)
	}
	for _, action := range []string{"pause", "resume"} {
		if systemActions(ctx, t, pool, "data_task."+action) != 1 {
			t.Fatalf("%s control missing its audit", action)
		}
	}
	if _, err := e.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	done := progressFor(ctx, t, e)
	if done.Status != Done || done.Changed != 1 {
		t.Fatalf("resumed task did not finish: %+v", done)
	}
	if err := e.Control(ctx, done.TaskID, done.Step, "pause", done.Version); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("paused completed task: %v", err)
	}
}

func TestEstimateRefusalLeavesPreviousEstimateAndAuditIntact(t *testing.T) {
	ctx, pool, e, _ := executorFixture(t)
	p := progressFor(ctx, t, e)
	e.tasks[0].Steps[0].Estimate = func(context.Context, Querier, int64) (int64, bool, error) {
		return estimateCap, true, nil
	}
	if err := e.Control(ctx, p.TaskID, p.Step, EstimateAction, p.Version); err != nil {
		t.Fatal(err)
	}
	before := progressFor(ctx, t, e)
	if before.Estimate == nil || *before.Estimate != estimateCap || !before.EstimateCapped || before.EstimatedAt == nil {
		t.Fatalf("bounded estimate=%+v", before)
	}
	for _, estimate := range []int64{-1, estimateCap + 1} {
		e.tasks[0].Steps[0].Estimate = func(context.Context, Querier, int64) (int64, bool, error) {
			return estimate, false, nil
		}
		if err := e.Control(ctx, p.TaskID, p.Step, EstimateAction, before.Version); err == nil {
			t.Fatalf("accepted out-of-bound estimate %d", estimate)
		}
	}
	refused := errors.New("estimate unavailable")
	e.tasks[0].Steps[0].Estimate = func(context.Context, Querier, int64) (int64, bool, error) {
		return 0, false, refused
	}
	if err := e.Control(ctx, p.TaskID, p.Step, EstimateAction, before.Version); !errors.Is(err, refused) {
		t.Fatalf("estimate failure lost: %v", err)
	}
	e.tasks[0].Requires = []string{"core/0"}
	if err := e.Control(ctx, p.TaskID, p.Step, EstimateAction, before.Version); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("estimated without prerequisite: %v", err)
	}
	after := progressFor(ctx, t, e)
	if after.Version != before.Version || after.Estimate == nil || *after.Estimate != *before.Estimate || !after.EstimateCapped {
		t.Fatalf("failed estimate changed progress: %+v", after)
	}
	if systemActions(ctx, t, pool, "data_task.estimate") != 1 {
		t.Fatal("failed estimate produced a successful control audit")
	}
}

func TestControlsRefuseUnknownAndUnauthorizedWork(t *testing.T) {
	ctx, _, e, _ := executorFixture(t)
	before := progressFor(ctx, t, e)
	for _, target := range [][2]string{{"core/0_unknown", before.Step}, {before.TaskID, "unknown"}} {
		if err := e.Control(ctx, target[0], target[1], "pause", before.Version); !errors.Is(err, apperrors.ErrNotFound) {
			t.Fatalf("unknown target %v: %v", target, err)
		}
	}
	denied := principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalHuman, ID: "reader"})
	agent := principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalAgent, ID: "agent"})
	for _, reader := range []context.Context{denied, agent} {
		if err := e.Control(reader, before.TaskID, before.Step, "pause", before.Version); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Fatalf("unauthorized control: %v", err)
		}
	}
	if _, err := e.List(denied); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("unauthorized report: %v", err)
	}
	e.tasks[0].Steps[0].Name = "unseeded"
	if err := e.Control(ctx, before.TaskID, "unseeded", "pause", 0); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("unseeded control: %v", err)
	}
	e.tasks[0].Steps[0].Name = before.Step
	after := progressFor(ctx, t, e)
	if after.Status != before.Status || after.Version != before.Version {
		t.Fatalf("refused control changed progress: %+v", after)
	}
	if err := e.Control(ctx, before.TaskID, before.Step, "destroy", before.Version); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("unknown action accepted: %v", err)
	}
}
