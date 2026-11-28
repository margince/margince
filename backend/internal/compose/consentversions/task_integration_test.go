// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consentversions_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/consentversions"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type consentVersionSeed struct{ Lead, Withdrawn bool }

func seedConsentVersion(t *testing.T, e *integration.Env, seed consentVersionSeed) ids.UUID {
	t.Helper()
	subject := e.SeedContact(t, "Consent subject", nil)
	column := "contact_id"
	if seed.Lead {
		subject = ids.NewV7()
		e.WsExec(t, `INSERT INTO lead (id, source, captured_by) VALUES ($1, 'manual', 'human:test')`, subject)
		column = "lead_id"
	}
	e.WsExec(t, `INSERT INTO consent_purpose (id, key, label, requires_double_opt_in) VALUES ($1, 'business_correspondence', 'Business correspondence', false) ON CONFLICT (key) DO NOTHING`, ids.NewV7())
	purpose := ids.MustParse(e.WsScalar(t, `SELECT id::text FROM consent_purpose WHERE key = 'business_correspondence'`))
	id, at := ids.NewV7(), clocktest.Now(t).UTC()
	state := "granted"
	var text, version *string
	if seed.Withdrawn {
		state = "withdrawn"
	} else {
		text, version = new("The wording shown"), new("policy-1")
	}
	e.WsExec(t, `INSERT INTO contact_consent (id, `+column+`, purpose_id, state, source, captured_at)
   VALUES ($1, $2, $3, $4, 'test', $5)`, id, subject, purpose, state, at)
	e.WsExec(t, `INSERT INTO consent_event (`+column+`, purpose_id, new_state, source, captured_at, captured_by, policy_text, policy_version)
   VALUES ($1, $2, $3, 'test', $4, 'human:test', $5, $6)`, subject, purpose, state, at, text, version)
	return id
}

func consentVersion(t *testing.T, e *integration.Env, id ids.UUID) string {
	t.Helper()
	return e.WsScalar(t, `SELECT COALESCE(policy_version, '<unknown>') FROM contact_consent WHERE id = $1`, id)
}

func TestConsentVersionTaskUsesOnlyTheMatchingLatestProof(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	contact, lead := seedConsentVersion(t, e, consentVersionSeed{}), seedConsentVersion(t, e, consentVersionSeed{Lead: true})
	withdrawal := seedConsentVersion(t, e, consentVersionSeed{Withdrawn: true})
	untouched := seedConsentVersion(t, e, consentVersionSeed{})
	e.WsExec(t, `UPDATE contact_consent SET policy_version = 'already-recorded' WHERE id = $1`, untouched)
	var unknown []ids.UUID
	for _, change := range []string{"source = 'different'", "lawful_basis = 'contract'", "captured_at = captured_at + interval '1 second'", "state = 'withdrawn'"} {
		id := seedConsentVersion(t, e, consentVersionSeed{})
		e.WsExec(t, `UPDATE contact_consent SET `+change+` WHERE id = $1`, id)
		unknown = append(unknown, id)
	}
	tied := seedConsentVersion(t, e, consentVersionSeed{Lead: true})
	e.WsExec(t, `INSERT INTO consent_event (id, lead_id, purpose_id, new_state, source, captured_at, captured_by, policy_text, policy_version)
  SELECT 'ffffffff-ffff-4fff-bfff-ffffffffffff', lead_id, purpose_id, state, source, captured_at, 'human:test', 'Later wording', 'policy-2'
    FROM contact_consent WHERE id = $1`, tied)
	stale := seedConsentVersion(t, e, consentVersionSeed{})
	e.WsExec(t, `INSERT INTO consent_event (contact_id, purpose_id, new_state, source, captured_at, captured_by)
  SELECT contact_id, purpose_id, 'withdrawn', source, captured_at + interval '1 second', 'human:test'
    FROM contact_consent WHERE id = $1`, stale)
	unknown = append(unknown, stale, withdrawal)
	beforeProofs, beforeEvents := e.WsCount(t, `SELECT count(*) FROM consent_event`), e.WsCount(t, `SELECT count(*) FROM event_outbox`)
	task := consentversions.DataTask()
	task.Steps[0].Limit, task.Steps[0].MinLimit = 1, 1
	engine, now := consentVersionEngine(ctx, t, e, task)
	for range 20 {
		out, err := engine.Execute(ctx)
		if err != nil {
			t.Fatal(err)
		}
		*now = now.Add(out.Wait)
	}
	for _, id := range []ids.UUID{contact, lead} {
		if got := consentVersion(t, e, id); got != "policy-1" {
			t.Errorf("proven version = %q", got)
		}
	}
	if got := consentVersion(t, e, tied); got != "policy-2" {
		t.Errorf("equal-time latest version = %q", got)
	}
	if got := consentVersion(t, e, untouched); got != "already-recorded" {
		t.Errorf("existing version changed to %q", got)
	}
	for _, id := range unknown {
		if got := consentVersion(t, e, id); got != "<unknown>" {
			t.Errorf("unproven version invented: %q", got)
		}
	}
	var replay datatask.BatchOut
	if err := database.WithTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		replay, err = task.Steps[0].Batch(ctx, tx, datatask.BatchIn{Limit: 100, RunID: ids.NewV7()})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if replay.Changed != 0 {
		t.Errorf("replay changed %d rows", replay.Changed)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM consent_event`); got != beforeProofs {
		t.Errorf("repair wrote proofs: %d vs %d", got, beforeProofs)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM event_outbox`); got != beforeEvents {
		t.Errorf("repair emitted events: %d vs %d", got, beforeEvents)
	}
}

func TestConsentVersionTaskRollsBackWithItsExecutorTransaction(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	id := seedConsentVersion(t, e, consentVersionSeed{})
	abort := errors.New("abort the batch")
	task := consentversions.DataTask()
	batch := task.Steps[0].Batch
	task.Steps[0].Batch = func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
		out, err := batch(ctx, tx, in)
		if err != nil {
			return out, err
		}
		if out.Changed != 1 {
			t.Errorf("batch changed %d rows", out.Changed)
		}
		return out, abort
	}
	engine, _ := consentVersionEngine(ctx, t, e, task)
	if _, err := engine.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	progress, err := engine.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(progress) != 1 || progress[0].Changed != 0 || progress[0].Scanned != 0 || progress[0].Batches != 0 || progress[0].ConsecutiveFailures != 1 {
		t.Fatalf("failed batch committed progress: %+v", progress)
	}
	if got := consentVersion(t, e, id); got != "<unknown>" {
		t.Fatalf("rolled-back version persisted: %q", got)
	}
}

func TestConsentVersionTaskBoundsItsEstimateAndPagesPastUnknownProof(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	for range 3 {
		seedConsentVersion(t, e, consentVersionSeed{Withdrawn: true})
	}
	seedConsentVersion(t, e, consentVersionSeed{Lead: true})
	step := consentversions.DataTask().Steps[0]
	err := database.WithTx(ctx, e.Pool, func(tx pgx.Tx) error {
		count, more, err := step.Estimate(ctx, tx, 2)
		if err != nil {
			return err
		}
		if count != 2 || !more {
			t.Errorf("estimate = %d, more %v; want capped at 2", count, more)
		}
		in := datatask.BatchIn{Limit: 1, RunID: ids.NewV7()}
		scanned, changed, done := 0, 0, false
		for range 10 {
			out, err := step.Batch(ctx, tx, in)
			if err != nil {
				return err
			}
			if out.Scanned > 1 {
				t.Errorf("one-row page inspected %d states", out.Scanned)
			}
			scanned, changed = scanned+out.Scanned, changed+out.Changed
			if out.Done {
				done = true
				break
			}
			in.Cursor = out.Cursor
		}
		if !done || scanned != 4 || changed != 1 {
			t.Errorf("paging: done %v, scanned %d, changed %d", done, scanned, changed)
		}
		if _, err := step.Batch(ctx, tx, datatask.BatchIn{Limit: 1, Cursor: []byte("bad cursor")}); err == nil {
			t.Error("malformed cursor accepted")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func consentVersionEngine(ctx context.Context, t *testing.T, e *integration.Env, task datatask.Task) (*datatask.Engine, *time.Time) {
	t.Helper()
	now := clocktest.Now(t)
	engine, err := datatask.New(e.Pool, []datatask.Task{task}, "test", func() time.Time { return now }, func(context.Context, pgx.Tx, datatask.Progress) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	return engine, &now
}
