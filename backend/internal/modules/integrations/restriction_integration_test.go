// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integrations

// A paid data provider is never asked about a contact under a restriction of
// processing (GDPR Art. 18), and the sweep picks the contact up after the lift.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

// restrict plants the stop the consent module writes. A module never imports
// a sibling, so the row is written by hand in the shape consent writes it.
func (e *runsEnv) restrict(t *testing.T, contact ids.ContactID) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO communication_suppression (contact_id, kind, source, captured_by, decided_by_level, recorded_through)
		VALUES ($1, 'processing_restriction', 'privacy request', 'human:x', 'subject', 'colleague')`, contact); err != nil {
		t.Fatal(err)
	}
}

func TestNoProviderIsAskedAboutARestrictedContactUntilTheLift(t *testing.T) {
	e := setupRuns(t, runsConfig{})
	e.restrict(t, e.mine)

	_, err := e.store.QueueRun(e.ctx, provider.QueueInput{
		ContactID: e.mine.String(), Provider: e.provider, Trigger: provider.TriggerManual,
	})
	if !errors.Is(err, ErrSubjectRestricted) {
		t.Fatalf("a run on a restricted contact answered %v, want ErrSubjectRestricted", err)
	}
	var runs int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM provider_run WHERE contact_id = $1`, e.mine).Scan(&runs); err != nil {
		t.Fatal(err)
	}
	if runs != 0 {
		t.Fatalf("a refused run left %d provider_run row(s)", runs)
	}
	if e.sweepSelects(t)[e.mine] {
		t.Fatal("the backfill sweep selected a restricted contact")
	}

	if _, err := e.owner.Exec(context.Background(),
		`UPDATE communication_suppression SET revoked_at = now() WHERE contact_id = $1`, e.mine); err != nil {
		t.Fatal(err)
	}
	if !e.sweepSelects(t)[e.mine] {
		t.Fatal("after the lift the sweep still passes the contact over")
	}
}

// A run queued before the restriction is held at the lease: nothing is sent,
// and the run stays queued for after the lift.
func TestARunQueuedBeforeARestrictionIsNotSubmitted(t *testing.T) {
	e := setupRuns(t, runsConfig{})
	sealCredential(t, e)
	run := queueFor(t, e, e.mine.String())
	e.restrict(t, e.mine)

	before := e.fake.Calls()
	if err := e.store.ExecuteSubmit(e.ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	if e.fake.Calls() != before {
		t.Fatal("the provider was called about a restricted contact")
	}
	if state, _, _ := runRow(t, e, run.ID); state != string(provider.RunQueued) {
		t.Fatalf("the run is %s, want queued until the lift", state)
	}
}
