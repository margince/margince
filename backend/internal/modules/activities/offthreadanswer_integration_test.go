// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Our mail on a new thread is found by the sender's address. It answers only
// when it went out the way their mail came in, and reached them.
func TestAnOffThreadReplyAnswersOnlyWhenDeliveredOnTheirChannel(t *testing.T) {
	t.Parallel()
	asked := clocktest.Now(t).Add(-48 * time.Hour).Truncate(time.Second)
	for name, tc := range map[string]struct {
		kind, asked, answered, delivery string
		want                            bool
	}{
		"delivered mail":              {kind: "email", want: true},
		"bounced mail":                {kind: "email", delivery: `'sent', now(), 'hard'`},
		"a message on its medium":     {kind: "message", asked: "whatsapp", answered: "whatsapp", want: true},
		"a message on another medium": {kind: "message", asked: "whatsapp", answered: "telegram"},
	} {
		t.Run(name, func(t *testing.T) {
			e := setupLoad(t)
			inbound, reply := ids.NewV7(), ids.NewV7()
			address := "asker-" + inbound.String() + "@buyer.test"
			e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, thread_key, channel_provider, source, captured_by)
				VALUES ($1, $2, 'inbound', 'Invoice', $3, $4, NULLIF($5, ''), 'seed', 'system')`, inbound, tc.kind, asked, "asked-"+inbound.String(), tc.asked)
			e.exec(t, `INSERT INTO activity_participant (activity_id, address, role) VALUES ($1, $2, 'from')`, inbound, address)
			e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, thread_key, channel_provider,
				counterparty_email, counterparty_outbound_attested, source, captured_by)
				VALUES ($1, $2, 'outbound', 'Re: Invoice', $3, $4, NULLIF($5, ''), $6, true, 'seed', 'system')`,
				reply, tc.kind, asked.Add(time.Hour), "answer-"+reply.String(), tc.answered, address)
			if tc.delivery != "" {
				e.exec(t, `INSERT INTO comms_outbound (id, activity_id, user_id, provider, message_id, recipients, subject, body,
					consent_purpose, sender_kind, status, bounced_at, bounce_kind)
					VALUES ($1, $2, $3, 'gmail', $4, '[]', 'Re: Invoice', 'Attached.', 'business_correspondence', 'user', `+tc.delivery+`)`,
					ids.NewV7(), reply, e.rep, ids.NewV7().String()+"@undelivered.test")
			}
			var answered bool
			if err := e.owner.QueryRow(e.as(), `SELECT `+answeredSQL("inbound", `'infinity'::timestamptz`)+`
				FROM activity inbound WHERE inbound.id = $1`, inbound).Scan(&answered); err != nil {
				t.Fatalf("reading whether the mail was answered: %v", err)
			}
			if answered != tc.want {
				t.Errorf("answered = %v, want %v", answered, tc.want)
			}
		})
	}
}
