// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A voice build runs for the colleague who asked for it. The build row is kept
// by the system, so it always reaches an end. The voice is derived and
// activated by an agent under that colleague's live grants, so it stops when
// those grants no longer cover it.

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

func TestAVoiceBuildWorksAsItsRequestersAgentAndKeepsItsRowAsTheSystem(t *testing.T) {
	quote := "Colleague-path quote."
	env, build := seedVoiceBuild(t, quote, 6)
	worker := newVoiceBuildWorker(env.e.Pool, &scriptedBuildBrain{judgeScore: 0.9, quote: quote}, slog.New(slog.DiscardHandler))

	if err := worker.Work(context.Background(), voiceBuildJob(env, build)); err != nil {
		t.Fatalf("Work: %v", err)
	}
	finished, err := env.store.GetBuild(env.owner, env.profile.ID, build.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finished.Status != "succeeded" {
		t.Fatalf("build = %+v, want succeeded", finished)
	}
	if n := env.e.WsCount(t, `SELECT count(*) FROM audit_log
		WHERE entity_type = 'voice_build' AND entity_id = $1
		  AND actor_type = 'system' AND actor_id = 'system:voice_build' AND on_behalf_of = $2`,
		build.ID, env.e.Rep1); n == 0 {
		t.Error("no build-row change was kept by system:voice_build for Rep1; the claim is the system's bookkeeping")
	}
	if n := env.e.WsCount(t, `SELECT count(*) FROM audit_log
		WHERE entity_type = 'voice_profile_version'
		  AND actor_type = 'agent' AND actor_id = 'agent:voice-builder' AND on_behalf_of = $1`,
		env.e.Rep1); n != 1 {
		t.Errorf("%d versions written by agent:voice-builder for Rep1, want 1: the voice is the agent's work", n)
	}
}

// A requester whose grants no longer cover the profile gets a failed build with
// a reason, written by the system. Their corpus never reaches a model.
func TestAVoiceBuildForARequesterWithoutTheGrantFailsAndNeverCallsTheModel(t *testing.T) {
	quote := "Withdrawn-grant quote."
	env, build := seedVoiceBuild(t, quote, 6)
	env.e.WsExec(t, `DELETE FROM role_assignment WHERE user_id = $1`, env.e.Rep1)
	calls := 0
	worker := newVoiceBuildWorker(env.e.Pool, brainFunc(func(context.Context, model.Request) (model.Response, error) {
		calls++
		return model.Response{}, nil
	}), slog.New(slog.DiscardHandler))

	if err := worker.Work(context.Background(), voiceBuildJob(env, build)); err != nil {
		t.Fatalf("Work: %v", err)
	}
	finished, err := env.store.GetBuild(env.owner, env.profile.ID, build.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finished.Status != "failed" || finished.StatusCode == nil || *finished.StatusCode != "internal" {
		t.Fatalf("build = %+v, want failed/internal and never left running", finished)
	}
	if finished.StatusDetail == nil || !strings.Contains(*finished.StatusDetail, "may no longer change this voice profile") {
		t.Errorf("detail = %v, want the reason the requester can no longer build", finished.StatusDetail)
	}
	if calls != 0 {
		t.Errorf("the model was called %d times for a requester who may not build", calls)
	}
	if n := env.e.WsCount(t, `SELECT count(*) FROM audit_log
		WHERE entity_type = 'voice_build' AND entity_id = $1 AND actor_id = 'system:voice_build'
		  AND after->>'status' = 'failed'`, build.ID); n != 1 {
		t.Errorf("%d failures kept by system:voice_build, want 1", n)
	}
	if n := env.e.WsCount(t, `SELECT count(*) FROM voice_profile_version WHERE voice_profile_id = $1`, env.profile.ID); n != 0 {
		t.Errorf("%d versions written for a requester who may not build, want 0", n)
	}
}
