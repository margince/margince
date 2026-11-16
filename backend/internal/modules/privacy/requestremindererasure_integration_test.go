// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// A mail may itself be titled "Erased", so the subject cannot mark a reminder
// that erasure already reached.
func TestErasingAMessageErasesTheReminderThatQuotesIt(t *testing.T) {
	for _, title := range []string{"Re: Angebot", erasedActivitySubject} {
		t.Run(title, func(t *testing.T) {
			env := setupSARIdentifiers(t)
			message := seedSubjectActivity(t, env)
			reminder := ids.NewV7()
			mustExec(context.Background(), t, env.owner, `INSERT INTO activity
				(id, kind, subject, body, system_text, occurred_at, source, captured_by, source_system, source_id, source_activity_id)
				VALUES ($1, 'task', $5, 'Anna Weber asked: "Please send the report."', $6, now(), 'manual',
				        'system:test', $2, $3, $4)`, reminder, provenance.EmailRequestSource, message.String(), message, title,
				&systemtext.Message{Version: 1, Subject: systemtext.Sentence("task.reply_to_email"),
					Body: systemtext.Sentence("request.asked", systemtext.Text("Anna Weber"), systemtext.Text("Please send the report."))})

			eraser := NewEraser(env.db).WithRawCapturePurger(&nilRawPurger{})
			if err := env.db.Tx(env.ctx, func(tx pgx.Tx) error {
				return eraser.purgeContentDerivedFrom(env.ctx, tx, message, theClockRanOut)
			}); err != nil {
				t.Fatal(err)
			}

			var subject string
			var bodyLeft, marked bool
			if err := env.owner.QueryRow(context.Background(),
				`SELECT subject, body IS NOT NULL, system_text = $2 FROM activity WHERE id = $1`, reminder, erasedActivityText()).
				Scan(&subject, &bodyLeft, &marked); err != nil {
				t.Fatal(err)
			}
			if subject != erasedActivitySubject || bodyLeft || !marked {
				t.Errorf("the reminder kept subject %q (body left: %v, system text erased: %v) after its message was "+
					"erased; it quotes the sender's words and must go with them", subject, bodyLeft, marked)
			}
		})
	}
}
