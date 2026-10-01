// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The done lane's count, against a read that really was cut short.
//
// The cap is a LIMIT in the arm's own statement, so whether a read came back
// full is a fact about the database rather than about the assembler. A suite
// that hands the assembler a "this arm was capped" map proves the flag travels
// and nothing about the read that sets it, which is the half that can silently
// stop being true.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// readCapRows is one row past the arm's limit, which is what makes the read
// come back full. It is spelled here rather than imported because magic keeps
// its cap unexported; the assertion below fails loudly if the two drift.
const readCapRows = 5001

// A JOB BIGGER THAN ONE READ REPORTS A FLOOR, NOT A TOTAL.
//
// The rows past the cap are indistinguishable from rows that do not exist, so
// a line grouped out of a full read states the most it could see. Saying that
// number flat understates how far a machine went, which is the direction a
// reader of this page cannot afford to be misled in.
func TestALineFromAReadThatFilledItsCapSaysTheCountIsAFloor(t *testing.T) {
	e := integration.Setup(t)
	since := time.Now().Add(-time.Hour)
	contact := e.SeedContact(t, "Anna Keller", &e.Rep1)

	// One contact, many rows inside the window: the cap counts AUDIT ROWS, so a
	// job that revisited one record fills it exactly as one over many would.
	if _, err := e.Pool.Exec(context.Background(), `
		INSERT INTO audit_log (id, occurred_at, action, entity_type, entity_id, actor_type, actor_id, after)
		SELECT uuidv7(), now() - (n || ' milliseconds')::interval, 'update', 'contact', $1, 'agent', 'agent:deepread',
		       '{"cohort_linked": 0, "cohort_promoted": 1}'::jsonb
		  FROM generate_series(1, $2) AS n`, contact, readCapRows); err != nil {
		t.Fatalf("seeding a job bigger than one read: %v", err)
	}

	receipt, err := newMagicService(e.Pool, approvalsServiceWithEffects(e.Pool), time.Now).
		Read(e.As(e.Rep1, []ids.UUID{e.Team1}, integration.RepPerms), &since, 20)
	if err != nil {
		t.Fatalf("reading the receipt: %v", err)
	}

	var line crmcontracts.MagicLine
	var found bool
	for _, candidate := range receipt.Done {
		if candidate.Entity != nil && ids.UUID(candidate.Entity.Id) == contact {
			line, found = candidate, true
			break
		}
	}
	if !found {
		t.Fatal("the contact the job worked on has no line on the receipt")
	}
	if line.CountIsFloor == nil || !*line.CountIsFloor {
		t.Error("a line grouped out of a read that filled its cap states its count as exact; " +
			"either the cap moved past the row count seeded here or the read no longer reports being cut")
	}
}
