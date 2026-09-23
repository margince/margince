// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// A meeting's duration reaches the column that decides when it is over.
//
// The wire accepts duration_seconds, the mapping carries it (mailimportmapping_test)
// and the projection reads it back (activityprojection_test) — and neither of
// those touches the INSERT between them. While the store dropped the value,
// every one of those tests passed: the mapping asserts on the input struct, the
// projection asserts a column maps to a field, and the readers that care seed
// the column by statement rather than through this writer.
//
// So the link nothing held is the one that broke, and this holds it. It asks
// the DATABASE rather than the returned struct, because a writer that filled
// the response and not the row would satisfy anything that read its answer.
//
// What the column decides: MeetingIsOverSQL reads "over" as start PLUS
// duration. Empty, it reduces to start — so a meeting counts as a deal's next
// step only until it begins, and one in progress right now reports none.

import (
	"context"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestALoggedMeetingKeepsTheDurationItWasGiven(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)

	const hour = 3600
	subject, seconds := "Quarterly review", hour
	in, err := LogActivityInputFrom(crmcontracts.CreateActivityRequest{
		Kind: "meeting", Subject: &subject, Source: "ui", DurationSeconds: &seconds,
	})
	if err != nil {
		t.Fatalf("LogActivityInputFrom: %v", err)
	}
	activity, _, err := e.store(nil).LogActivity(ctx, in)
	if err != nil {
		t.Fatalf("LogActivity: %v", err)
	}

	var stored *int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT duration_seconds FROM activity WHERE id = $1`, ids.UUID(activity.Id),
	).Scan(&stored); err != nil {
		t.Fatalf("reading the stored duration: %v", err)
	}
	if stored == nil {
		t.Fatal("duration_seconds is NULL — the meeting was accepted and its length discarded, " +
			"so MeetingIsOverSQL reads it as over the moment it starts")
	}
	if *stored != hour {
		t.Errorf("duration_seconds = %d, want %d", *stored, hour)
	}
}
