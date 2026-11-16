// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"maps"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestInvitationResponseMetricsUseTheResponseInstant(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name                         string
		starts, response, windowEnd  time.Duration
		answered                     int
		accepted, stamped, colleague bool
	}{
		{"answered before meeting but meeting inside window", 2 * time.Hour, time.Hour, 24 * time.Hour, 1, false, false, false},
		{"answered after meeting outside window", 2 * time.Hour, 3 * time.Hour, time.Hour, 0, false, false, false},
		{"answered as meeting starts", 2 * time.Hour, 2 * time.Hour, 24 * time.Hour, 0, false, false, false},
		{"answered after meeting inside window", 2 * time.Hour, 3 * time.Hour, 24 * time.Hour, 0, false, false, false},
		{"human accepted and answered after meeting", 2 * time.Hour, 3 * time.Hour, 24 * time.Hour, 1, true, false, false},
		{"seat stamped invitation", 2 * time.Hour, time.Hour, 24 * time.Hour, 0, false, true, false},
		{"human accepted seat stamped invitation", 2 * time.Hour, time.Hour, 24 * time.Hour, 1, true, true, false},
		{"human accepted colleague domain invitation", 2 * time.Hour, time.Hour, 24 * time.Hour, 0, true, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := setupLoad(t)
			from, to := requestInstant.Add(-time.Hour), requestInstant.Add(tc.windowEnd)
			store := metricsStore(e)
			if tc.colleague {
				_, domain, ok := strings.Cut(requestCounterparty, "@")
				if !ok {
					t.Fatal("fixture sender has no domain")
				}
				store = storeKnowing(e, domain)
			}
			before, err := store.ResponseWindow(e.as(), from, to)
			if err != nil {
				t.Fatal(err)
			}
			beforeSpread := invitationResponseSpread(t, e, store, to)
			invitation := seedEmailRequestFiledUnder(t, e, "Invitation: Metrics", "meeting", OwedVerdictAsksUs,
				requestCounterparty, []ActivityLinkInput{{EntityType: "contact", EntityID: e.buyer(t)}},
				mailParticipants{From: requestCounterparty})
			setCalendarMethod(t, e, invitation, "REQUEST")
			if tc.accepted {
				actor, ok := principal.Actor(e.as())
				if !ok {
					t.Fatal("missing reader")
				}
				actor.Permissions.Objects = maps.Clone(actor.Permissions.Objects)
				actor.Permissions.Objects["activity"] = principal.ObjectGrant{Read: true, Create: true}
				if _, _, err := storeKnowing(e).LogActivity(principal.WithActor(e.as(), actor), LogActivityInput{
					Kind: "task", Source: "manual", RequestActivityID: &invitation,
				}); err != nil {
					t.Fatal(err)
				}
			}
			if tc.stamped {
				e.exec(t, `UPDATE activity SET invitation_sender = $2 WHERE id = $1`, invitation, e.other)
			}
			meetingWithUID(t, e, invitation, requestInstant.Add(tc.starts))
			replyTo(t, e, invitation, "See you there.", requestInstant.Add(tc.response))
			got, err := store.ResponseWindow(e.as(), from, to)
			if err != nil {
				t.Fatal(err)
			}
			if got.Answered-before.Answered != tc.answered {
				t.Errorf("answered = %d, want %d", got.Answered-before.Answered, tc.answered)
			}
			if before.Answered == 0 && tc.answered == 1 && got.MedianMinutes != int(tc.response/time.Minute) {
				t.Errorf("median = %d minutes, want %v", got.MedianMinutes, tc.response)
			}
			spread := invitationResponseSpread(t, e, store, to)
			if spread.Answered-beforeSpread.Answered != tc.answered {
				t.Errorf("horizon answered = %d, want %d", spread.Answered-beforeSpread.Answered, tc.answered)
			}
			if tc.answered == 0 && spread.Slow != beforeSpread.Slow {
				t.Errorf("excluded invitation moved the horizon percentile: %v to %v", beforeSpread.Slow, spread.Slow)
			}
			if beforeSpread.Answered == 0 && tc.answered == 1 && spread.Slow != tc.response {
				t.Errorf("horizon percentile = %v, want %v", spread.Slow, tc.response)
			}
		})
	}
}

func invitationResponseSpread(t *testing.T, e *loadEnv, store *Store, asOf time.Time) waitingHorizonSpread {
	t.Helper()
	ctx := e.as()
	var spread waitingHorizonSpread
	if err := store.tx(ctx, func(tx pgx.Tx) error {
		var err error
		spread, err = store.waitingHorizonSpreadTx(ctx, tx, asOf, waitingHorizonSample)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return spread
}
