// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The colleagues without a mailbox, as the Network tab and who_knows read them.
// Both surfaces go through contactSeatGaps and must report the same list and
// total. A list cut at its limit must say how many it left out.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedUnmailedSeats puts n invited colleagues on the Cc line of one message
// from the contact, dated with precision. Invited seats have no mailbox.
func seedUnmailedSeats(t *testing.T, e *integration.Env, contact ids.UUID, n int, precision string) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		var activity ids.UUID
		addresses := make([]string, 0, n)
		for i := range n {
			address := fmt.Sprintf("unmailed-%d-%s@example.test", i, contact)
			if _, err := tx.Exec(ctx, `
				INSERT INTO app_user (id, email, display_name, status) VALUES ($1, $2, $2, 'invited')`, ids.NewV7(), address); err != nil {
				return err
			}
			addresses = append(addresses, address)
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO activity (kind, occurred_at, occurred_precision, direction, source, captured_by)
			VALUES ('email', date_trunc('day', now() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC' - interval '12 hours',
			        $1, 'inbound', 'imap', 'connector:imap') RETURNING id`, precision).Scan(&activity); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity_participant (activity_id, contact_id, role) VALUES ($1, $2, 'from')`,
			activity, contact); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO activity_participant (activity_id, address, role)
			SELECT $1, address, 'cc' FROM unnest($2::text[]) AS address`, activity, addresses)
		return err
	}); err != nil {
		t.Fatalf("seeding colleagues without a mailbox: %v", err)
	}
}

func readContactNetwork(t *testing.T, e *integration.Env, contact ids.UUID) crmcontracts.ContactNetwork {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/contacts/"+contact.String()+"/network", nil).
		WithContext(e.Admin())
	NewContactGraphReads(e.Pool, e.DB()).GetContactNetwork(rec, req, crmcontracts.Id(contact))
	if rec.Code != http.StatusOK {
		t.Fatalf("network status = %d: %s", rec.Code, rec.Body.String())
	}
	var out crmcontracts.ContactNetwork
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decoding the network: %v", err)
	}
	return out
}

func TestBothNetworkSurfacesCountEveryColleagueWithoutAMailbox(t *testing.T) {
	cases := []struct {
		name, precision string
		seats           int
		wantListed      int
	}{
		{name: "a list past its limit names ten and counts twelve", precision: "instant", seats: 12, wantListed: 10},
		{name: "a list under its limit names and counts all three", precision: "day", seats: 3, wantListed: 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := integration.Setup(t)
			contact := seedGraphContact(t, e, "Outside Contact")
			seedUnmailedSeats(t, e, contact, tc.seats, tc.precision)

			network := readContactNetwork(t, e, contact)
			if len(network.CaptureGaps) != tc.wantListed || network.CaptureGapsTotal != tc.seats {
				t.Errorf("the Network tab lists %d of %d, want %d of %d",
					len(network.CaptureGaps), network.CaptureGapsTotal, tc.wantListed, tc.seats)
			}

			reading, err := whoKnowsLister(e.Pool, contacts.NewStore(e.DB()))(e.Admin(), contact)
			if err != nil {
				t.Fatalf("who_knows seam: %v", err)
			}
			if len(reading.CaptureGaps) != tc.wantListed || reading.CaptureGapsTotal != tc.seats {
				t.Errorf("who_knows lists %d of %d, want %d of %d",
					len(reading.CaptureGaps), reading.CaptureGapsTotal, tc.wantListed, tc.seats)
			}
			for i, gap := range reading.CaptureGaps {
				if gap.LastOnThreadPrecision != tc.precision || i >= len(network.CaptureGaps) ||
					network.CaptureGaps[i].LastOnThreadPrecision == nil || string(*network.CaptureGaps[i].LastOnThreadPrecision) != tc.precision {
					t.Errorf("gap %d: who_knows reads its time as %q, want %q as the Network tab does", i, gap.LastOnThreadPrecision, tc.precision)
				}
			}
		})
	}
}
