// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func endWorkLease(t *testing.T, env *integration.Env, lease *database.SessionLease) {
	t.Helper()
	var ended bool
	err := env.Pool.QueryRow(t.Context(), `SELECT pg_terminate_backend(@pid, 5000)`, pgx.StrictNamedArgs{"pid": lease.PID()}).Scan(&ended)
	if err != nil || !ended {
		t.Fatalf("ending worker session: %v, ended=%v", err, ended)
	}
}

func TestVoiceBuildRecoversAsSoonAsItsSessionDies(t *testing.T) {
	env, build := seedVoiceBuild(t, "Recoverable quote.", 6)
	ctx := env.workerCtx(t)
	lease, held, err := env.e.DB().TryWorkLease(ctx, "voice_build", build.ID, discardLogger())
	if err != nil || !held {
		t.Fatalf("lease: held=%v err=%v", held, err)
	}
	defer lease.Close(ctx)
	input, claimed, err := env.store.ClaimBuild(ctx, env.profile.ID, build.ID, time.Hour)
	if err != nil || !claimed {
		t.Fatalf("claim: claimed=%v err=%v", claimed, err)
	}
	worker := newVoiceBuildWorker(env.e.Pool, nil, discardLogger())
	if err := worker.Work(context.Background(), voiceBuildJob(env, build)); err != nil {
		t.Fatal(err)
	}
	before, err := env.store.GetBuild(env.owner, env.profile.ID, build.ID)
	if err != nil || before.Status != "running" || before.Attempt != input.Build.Attempt {
		t.Fatalf("live owner changed: %+v, %v", before, err)
	}
	due, err := env.store.DueDeferredBuilds(ctx)
	if err != nil || len(due) != 0 {
		t.Fatalf("live owner was re-offered: %+v, %v", due, err)
	}
	endWorkLease(t, env.e, lease)
	due, err = env.store.DueDeferredBuilds(ctx)
	if err != nil || len(due) != 1 || due[0].BuildID != build.ID {
		t.Fatalf("lost owner not re-offered: %+v, %v", due, err)
	}
	if err := worker.Work(context.Background(), voiceBuildJob(env, build)); err != nil {
		t.Fatal(err)
	}
	after, err := env.store.GetBuild(env.owner, env.profile.ID, build.ID)
	if err != nil || after.Status != "failed" || after.Attempt != before.Attempt+1 {
		t.Fatalf("replacement did not finish: %+v, %v", after, err)
	}
}

func TestVoiceStagesAreFencedToTheClaimGeneration(t *testing.T) {
	env, build := seedVoiceBuild(t, "Fenced quote.", 6)
	ctx := env.workerCtx(t)
	old, claimed, err := env.store.ClaimBuild(ctx, env.profile.ID, build.ID, time.Hour)
	if err != nil || !claimed {
		t.Fatalf("old claim: %v %v", claimed, err)
	}
	held, err := env.e.DB().WithWorkLease(ctx, "voice_build", build.ID, discardLogger(), func(owned context.Context) error {
		next, claimed, err := env.store.ClaimBuild(owned, env.profile.ID, build.ID, time.Hour)
		if err != nil || !claimed {
			t.Fatalf("replacement claim: %v %v", claimed, err)
		}
		if !claimTime(next).After(claimTime(old)) {
			t.Fatal("replacement did not advance its fence")
		}
		if err := env.store.SetBuildStage(ctx, build.ID, claimTime(old), "activate"); !errors.Is(err, ai.ErrVoiceBuildNotRunning) {
			t.Fatalf("stale stage: %v", err)
		}
		return env.store.SetBuildStage(owned, build.ID, claimTime(next), "extract")
	})
	if err != nil || !held {
		t.Fatalf("replacement: %v %v", held, err)
	}
}

func TestDocumentRetryUsesSessionDeathInsteadOfTheOldTimer(t *testing.T) {
	env := integration.Setup(t)
	ctx := env.Admin()
	pipeline, stage, _ := integration.DealFixture(t, env)
	deal := env.SeedDeal(t, "Document recovery", pipeline, stage, &env.Rep1)
	blob := blobstore.NewMemory()
	store := activities.NewStore(env.DB()).WithBlobstore(blob)
	attachment, err := store.UploadAttachment(ctx, activities.AttachmentInput{EntityType: "deal", EntityID: deal, Filename: "note.txt", ContentType: "text/plain", Content: strings.NewReader("A recoverable document.")})
	if err != nil {
		t.Fatal(err)
	}
	attachmentID := ids.UUID(attachment.Id)
	reading, _, err := store.StartExtractionReadQueued(ctx, attachmentID, "human:"+env.AdminUser.String(), nil)
	if err != nil {
		t.Fatal(err)
	}
	lease, held, err := env.DB().TryWorkLease(ctx, "document_extract", reading.ID, slog.Default())
	if err != nil || !held {
		t.Fatalf("lease: %v %v", held, err)
	}
	defer lease.Close(ctx)
	old, err := store.BeginExtractionRead(ctx, reading.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	enqueued := 0
	enqueue := func(context.Context, pgx.Tx, activities.ExtractionRead) error { enqueued++; return nil }
	if _, _, err := store.StartExtractionReadQueued(ctx, attachmentID, "human:"+env.AdminUser.String(), enqueue); err != nil {
		t.Fatal(err)
	}
	if enqueued != 0 {
		t.Fatal("live owner was requeued")
	}
	endWorkLease(t, env, lease)
	if _, _, err := store.StartExtractionReadQueued(ctx, attachmentID, "human:"+env.AdminUser.String(), enqueue); err != nil {
		t.Fatal(err)
	}
	if enqueued != 1 {
		t.Fatalf("replacement offers=%d", enqueued)
	}
	worker := newDocumentExtractWorker(env.Pool, nil, blob, discardLogger())
	if err := worker.Work(context.Background(), &river.Job[DocumentExtractArgs]{Args: DocumentExtractArgs{Workspace: env.WS, AttachmentID: attachmentID, ExtractionReadID: reading.ID, RequestedBy: env.AdminUser.String()}}); err != nil {
		t.Fatal(err)
	}
	finished, err := store.GetExtractionRead(ctx, attachmentID, reading.ID)
	if err != nil || finished.Status != activities.ExtractionReadFailed || finished.Attempt != old.Attempt+1 {
		t.Fatalf("replacement did not finish: %+v %v", finished, err)
	}
	err = store.FinishExtractionRead(ctx, reading.ID, activities.ExtractionReadOutcome{ClaimedAt: claimStart(old), Status: activities.ExtractionReadFailed, Detail: "Stale worker"})
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("stale worker wrote its result: %v", err)
	}
}

func TestSessionLockedJobsInsertPastAbandonedRiverRows(t *testing.T) {
	env := integration.Setup(t)
	integration.ApplyRiverSchema(t)
	runner, err := jobs.NewInserter(env.Pool, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range []river.JobArgs{
		CaptureSyncArgs{Workspace: env.WS, ConnectionID: ids.NewV7().String(), Provider: "gmail"},
		DocumentExtractArgs{Workspace: env.WS, AttachmentID: ids.NewV7(), ExtractionReadID: ids.NewV7(), RequestedBy: env.AdminUser.String()},
		VoiceBuildArgs{Workspace: env.WS, ProfileID: ids.NewV7().String(), BuildID: ids.NewV7().String(), RequestedBy: env.AdminUser.String()},
	} {
		t.Run(args.Kind(), func(t *testing.T) {
			opts := &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: activeSweepStates}}
			if err := runner.Enqueue(t.Context(), args, opts); err != nil {
				t.Fatal(err)
			}
			if _, err := env.Pool.Exec(t.Context(), `UPDATE river_job SET state = 'running', attempted_at = now() WHERE kind = @kind`, pgx.StrictNamedArgs{"kind": args.Kind()}); err != nil {
				t.Fatal(err)
			}
			if err := runner.Enqueue(t.Context(), args, opts); err != nil {
				t.Fatal(err)
			}
			var count int
			err := env.Pool.QueryRow(t.Context(), `SELECT count(*) FROM river_job WHERE kind = @kind`, pgx.StrictNamedArgs{"kind": args.Kind()}).Scan(&count)
			if err != nil || count != 2 {
				t.Fatalf("replacement rows=%d, err=%v", count, err)
			}
		})
	}
}
