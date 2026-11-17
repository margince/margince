// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestEveryDeliveryScrubClosesTheReplyAMemberStillHasQueued(t *testing.T) {
	for name, scrub := range deliveryScrubs {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			tx := subjectColumnsTx(ctx, t)

			ws, user, activity, connection := ids.NewV7(), ids.NewV7(), ids.NewV7(), ids.NewV7()
			mustExec(ctx, t, tx, `INSERT INTO workspace (id) VALUES ($1)`, ws)
			mustExec(ctx, t, tx,
				`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Member')`,
				user, "member-"+user.String()+"@anon.test")
			mustExec(ctx, t, tx,
				`INSERT INTO activity (id, kind, occurred_at, source, captured_by)
				 VALUES ($1, 'note', now(), 'manual', 'user:'||$2::text)`, activity, user)
			mustExec(ctx, t, tx,
				`INSERT INTO personal_channel_connection (id, user_id, enabled) VALUES ($1, $2, true)`, connection, user)
			mustExec(ctx, t, tx,
				`INSERT INTO personal_channel_reply (connection_id, activity_id, delivery_key, network, thread_id,
				                                     recipient_account, reply_to, body)
				 VALUES ($1, $2, 'erasure-key', 'whatsapp', 'group-1', '+84900000000', 'wamid-1', 'See you at ten')`,
				connection, activity)

			if err := scrub(ctx, tx, []ids.UUID{activity}, noPayloads{t: t}); err != nil {
				t.Fatalf("scrubbing the deliveries: %v", err)
			}

			var state string
			var held int
			if err := tx.QueryRow(ctx, `
				SELECT state, (body IS NOT NULL)::int + (recipient_account IS NOT NULL)::int
				       + (reply_to IS NOT NULL)::int + (thread_id <> '')::int
				  FROM personal_channel_reply WHERE activity_id = $1`, activity).Scan(&state, &held); err != nil {
				t.Fatal(err)
			}
			if state != "failed" || held != 0 {
				t.Fatalf("after the erasure the reply is %q and holds %d of body, recipient, reply-to and chat; "+
					"want it closed with none, so a client's next pull cannot deliver it to the subject", state, held)
			}
		})
	}
}
