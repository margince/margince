// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// trustRace is a pipeline with two stages and an open transaction holding the
// ladder's trust lock, for a writer to wait on.
type trustRace struct {
	pipeline ids.PipelineID
	from, to ids.StageID
	holder   pgx.Tx
	pid      int32
}

func holdLadderTrust(ctx context.Context, t *testing.T, e *configEnv) *trustRace {
	t.Helper()
	from := openStage(t, e, SemanticOpen)
	var pipeline ids.PipelineID
	if err := e.owner.QueryRow(ctx, `SELECT pipeline_id FROM stage WHERE id = $1`, from).Scan(&pipeline); err != nil {
		t.Fatal(err)
	}
	probability := 10
	next, err := e.store.CreateStage(e.as(), CreateStageInput{
		PipelineID: pipeline, Name: "Proposal", Position: 1, Semantic: string(SemanticOpen), WinProbability: &probability,
	})
	if err != nil {
		t.Fatal(err)
	}
	holder, err := e.owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := holder.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("rolling back the lock holder: %v", err)
		}
	})
	race := &trustRace{pipeline: pipeline, from: from, to: ids.From[ids.StageKind](ids.UUID(next.Id)), holder: holder}
	if err := holder.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&race.pid); err != nil {
		t.Fatal(err)
	}
	if err := lockLadderTrust(ctx, holder, pipeline); err != nil {
		t.Fatal(err)
	}
	return race
}

// awaitWriter runs write until it waits on the holder, ends the holder with let,
// and answers what write returned. A write that never waits fails the test.
func awaitWriter(ctx context.Context, t *testing.T, e *configEnv, race *trustRace, write func() error, let func(context.Context) error) error {
	t.Helper()
	returned := make(chan struct{})
	var answer error
	go func() {
		defer close(returned)
		answer = write()
	}()
	testdb.AwaitQueuedBehind(t, e.owner, race.pid, 1, returned, "the ladder writer")
	if err := let(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-returned:
	case <-ctx.Done():
		t.Fatal("the writer never returned after the lock was released")
	}
	return answer
}

func (e *configEnv) asLadderAgent() context.Context {
	return principal.WithActor(e.as(), principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:ladder", UserID: e.admin,
		Scopes: principal.NewScopeSet(principal.ScopeRead, principal.ScopeWrite),
		Permissions: principal.Permissions{
			RoleKeys: []string{"admin"},
			Objects:  map[string]principal.ObjectGrant{"pipeline": {Create: true, Read: true, Update: true, Delete: true}},
		},
	})
}

// An agent's reshape waits for a transition being trusted, then reads that trust.
// Without the wait it would read the ladder as untrusted and commit beside it.
func TestAnAgentsReshapeWaitsForATrustBeingGrantedAndIsRefused(t *testing.T) {
	e := setupConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	race := holdLadderTrust(ctx, t, e)
	if _, err := race.holder.Exec(ctx, `
		INSERT INTO stage_progression_policy (pipeline_id, from_stage_id, to_stage_id, mode)
		VALUES ($1, $2, $3, 'auto')`, race.pipeline, race.from, race.to); err != nil {
		t.Fatal(err)
	}
	agent := e.asLadderAgent()
	probability := 20
	err := awaitWriter(ctx, t, e, race, func() error {
		_, err := e.store.CreateStage(agent, CreateStageInput{
			PipelineID: race.pipeline, Name: "Inserted", Position: 5, Semantic: string(SemanticOpen), WinProbability: &probability,
		})
		return err
	}, race.holder.Commit)
	var refusal *httperr.DetailedError
	if !errors.As(err, &refusal) || refusal.Code != "agent_reshapes_no_trusted_ladder" {
		t.Fatalf("the reshape answered %v, want the trusted-ladder refusal", err)
	}
}

// Trust being granted waits for an agent's reshape in the same way, whichever came first.
func TestATrustBeingGrantedWaitsForAReshapeInFlight(t *testing.T) {
	e := setupConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	race := holdLadderTrust(ctx, t, e)
	err := awaitWriter(ctx, t, e, race, func() error {
		_, err := e.store.SetTransitionPolicy(e.as(), SetTransitionPolicyInput{
			PipelineID: race.pipeline, FromStageID: race.from, ToStageID: race.to, Mode: ModeAuto,
		})
		return err
	}, race.holder.Rollback)
	if err != nil {
		t.Fatalf("the grant answered %v after the lock was released", err)
	}
}

// A policy write holds the trust lock while its foreign keys read the pipeline and
// the stages. So an agent's write waits for that lock holding neither, or the two
// wait on each other. Each writer here is asked while it waits.
func TestAnAgentWaitingForTheTrustLockHoldsNoLadderRow(t *testing.T) {
	probability := 20
	moved := 8
	label := "Renamed"
	for name, write := range map[string]func(*configEnv, *trustRace, ids.ExitCriterionID) error{
		"adding a stage": func(e *configEnv, r *trustRace, _ ids.ExitCriterionID) error {
			_, err := e.store.CreateStage(e.asLadderAgent(), CreateStageInput{
				PipelineID: r.pipeline, Name: "Inserted", Position: 5, Semantic: string(SemanticOpen), WinProbability: &probability,
			})
			return err
		},
		"moving a stage": func(e *configEnv, r *trustRace, _ ids.ExitCriterionID) error {
			_, err := e.store.UpdateStage(e.asLadderAgent(), r.to, UpdateStageInput{Position: &moved})
			return err
		},
		"removing a stage": func(e *configEnv, r *trustRace, _ ids.ExitCriterionID) error {
			return e.store.ArchiveStage(e.asLadderAgent(), r.to, nil)
		},
		"reordering the stages": func(e *configEnv, r *trustRace, _ ids.ExitCriterionID) error {
			_, err := e.store.ReorderStages(e.asLadderAgent(), r.pipeline, []ids.UUID{r.to.UUID, r.from.UUID}, nil)
			return err
		},
		"adding a criterion": func(e *configEnv, r *trustRace, _ ids.ExitCriterionID) error {
			_, err := e.store.CreateStageExitCriterion(e.asLadderAgent(), CreateCriterionInput{
				StageID: r.from, Key: "second", Label: "Second", Kind: string(CriterionCustom),
			})
			return err
		},
		"changing a criterion": func(e *configEnv, r *trustRace, criterion ids.ExitCriterionID) error {
			_, err := e.store.UpdateStageExitCriterion(e.asLadderAgent(), r.from, criterion, UpdateCriterionInput{Label: &label})
			return err
		},
		"removing a criterion": func(e *configEnv, r *trustRace, criterion ids.ExitCriterionID) error {
			return e.store.ArchiveStageExitCriterion(e.asLadderAgent(), r.from, criterion, nil)
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := setupConfigEnv(t)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			race := holdLadderTrust(ctx, t, e)
			created, err := e.store.CreateStageExitCriterion(e.as(), CreateCriterionInput{
				StageID: race.from, Key: "first", Label: "First", Kind: string(CriterionCustom),
			})
			if err != nil {
				t.Fatal(err)
			}
			criterion := ids.From[ids.ExitCriterionKind](ids.UUID(created.Id))
			returned := make(chan struct{})
			var answer error
			go func() {
				defer close(returned)
				answer = write(e, race, criterion)
			}()
			testdb.AwaitQueuedBehind(t, e.owner, race.pid, 1, returned, "the agent's ladder write")
			for table, id := range map[string]ids.UUID{"pipeline": race.pipeline.UUID, "stage": race.from.UUID} {
				if _, err := race.holder.Exec(ctx, `SELECT 1 FROM `+table+` WHERE id = $1 FOR KEY SHARE NOWAIT`, id); err != nil {
					t.Fatalf("the waiting write holds the %s row: %v", table, err)
				}
			}
			if _, err := race.holder.Exec(ctx, `SELECT 1 FROM stage WHERE id = $1 FOR KEY SHARE NOWAIT`, race.to.UUID); err != nil {
				t.Fatalf("the waiting write holds the other stage row: %v", err)
			}
			if err := race.holder.Rollback(ctx); err != nil {
				t.Fatal(err)
			}
			select {
			case <-returned:
			case <-ctx.Done():
				t.Fatal("the write never returned after the lock was released")
			}
			if answer != nil {
				t.Errorf("the write answered %v once the lock was released", answer)
			}
		})
	}
}
