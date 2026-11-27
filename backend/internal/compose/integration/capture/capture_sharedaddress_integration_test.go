// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const sharedAddress = "sales@myco.example"

// receivesAtSharedAddress lets the syncing seat learn the address from two
// deliveries, and records that each colleague's mailbox learned it too.
func receivesAtSharedAddress(t *testing.T, env captureEnv, colleagues ...ids.UUID) {
	t.Helper()
	env.sync(t,
		deliveredMail(sharedAddress, "", "buyer@customer.example", "shared-1@customer.example"),
		deliveredMail(sharedAddress, "", "other@customer.example", "shared-2@customer.example"))
	if got := ownAddresses(t, env, env.e.Rep1)[sharedAddress]; got != capturemod.IdentitySourceDeliveredTo {
		t.Fatalf("the seat holds %s with source %q, want it discovered", sharedAddress, got)
	}
	for _, colleague := range colleagues {
		ownerExec(t, env.e, `
			INSERT INTO capture_owner_identity (user_id, kind, value, source, created_by)
			VALUES ($1, 'address', $2, 'delivered_to', 'system:alias_discovery')`, colleague, sharedAddress)
	}
}

// statedSenders is the sender rows capture stored for one message: each
// address, and whether the row names a seat.
func statedSenders(t *testing.T, env captureEnv, sourceID string) (addresses []string, seats int) {
	t.Helper()
	err := database.WithWorkspaceTx(env.e.Admin(), env.e.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `
			SELECT coalesce(p.address, ''), p.user_id IS NOT NULL
			  FROM activity_participant p JOIN activity a ON a.id = p.activity_id
			 WHERE a.source_id = $1 AND p.role = 'from'`, sourceID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var address string
			var seat bool
			if err := rows.Scan(&address, &seat); err != nil {
				return err
			}
			if seat {
				seats++
			}
			addresses = append(addresses, address)
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatalf("reading who sent %s: %v", sourceID, err)
	}
	return addresses, seats
}

func TestMailFromAnAddressTwoSeatsReceiveAtKeepsThatAddressAsItsSender(t *testing.T) {
	t.Parallel()
	env := newCaptureEnv(t)
	receivesAtSharedAddress(t, env, env.e.Rep3)
	const reply = "shared-reply@myco.example"

	env.sync(t, emailCC(sharedAddress, "Sales", "buyer@customer.example", sharedAddress, reply))

	addresses, seats := statedSenders(t, env, reply)
	if len(addresses) != 1 || addresses[0] != sharedAddress || seats != 0 {
		t.Fatalf("stored senders = %v naming %d seats, want only %s and no seat", addresses, seats, sharedAddress)
	}
	author := listedAuthor(t, env.e, env.e.Rep1, reply)
	if author == nil || author.Address == nil || *author.Address != sharedAddress || author.UserId != nil || author.IsReader {
		t.Errorf("author = %+v, want the shared address and neither the reader nor any seat", author)
	}
	if n := countRows(t, env.e, `SELECT count(*) FROM contact_email WHERE email = '`+sharedAddress+`'`); n != 0 {
		t.Errorf("%d contacts were made for the shared address: it is internal", n)
	}
}

// The seat is the only one who can have written from an address it alone
// receives at, so no second end is stored for it.
func TestMailFromAnAddressOneSeatReceivesAtStoresNoOtherSender(t *testing.T) {
	t.Parallel()
	for name, colleagueLeft := range map[string]bool{
		"no colleague holds it":              false,
		"the colleague who held it has left": true,
	} {
		t.Run(name, func(t *testing.T) {
			env := newCaptureEnv(t)
			if colleagueLeft {
				receivesAtSharedAddress(t, env, env.e.Rep3)
				ownerExec(t, env.e, `UPDATE app_user SET status = 'deactivated', removed_at = now() WHERE id = $1`, env.e.Rep3)
			} else {
				receivesAtSharedAddress(t, env)
			}
			const own = "own-alias-mail@myco.example"

			env.sync(t, emailCC(sharedAddress, "Sales", "buyer@customer.example", sharedAddress, own))

			if addresses, _ := statedSenders(t, env, own); len(addresses) != 0 {
				t.Errorf("stored senders = %v, want none for the seat's own address", addresses)
			}
		})
	}
}
