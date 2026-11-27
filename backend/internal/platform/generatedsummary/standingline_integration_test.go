// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package generatedsummary_test

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/generatedsummary"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A line is replaced in place, and touching it moves only its fingerprint.
func TestAStatusLineIsReplacedAndTouchedInPlace(t *testing.T) {
	t.Parallel()
	e := setup(t)
	record := generatedsummary.StandingRecord{EntityType: "contact", EntityID: e.contact(t).UUID}
	note := e.note(t)
	e.saveLine(t, record, "They asked for the offer.", note)
	e.saveLine(t, record, "They accepted the offer.", note)
	e.inTx(t, func(tx pgx.Tx) error { return generatedsummary.TouchStandingLineTx(e.ctx, tx, record, "read-again") })

	got := e.line(t, record)
	if got == nil || got.Text != "They accepted the offer." || got.Fingerprint != "read-again" {
		t.Fatalf("the line read back as %+v, want the second text under the touched fingerprint", got)
	}
}

// Forgetting a contact deletes its own line and every line written from an
// activity filed on it, and leaves a line about anybody else.
func TestForgettingAContactDeletesTheLinesThatMayQuoteIt(t *testing.T) {
	t.Parallel()
	e := setup(t)
	subject, bystander := e.contact(t), e.contact(t)
	theirMail, otherMail := e.note(t), e.note(t)
	e.inTx(t, func(tx pgx.Tx) error {
		_, err := tx.Exec(e.ctx, `INSERT INTO activity_link (activity_id, entity_type, contact_id) VALUES ($1, 'contact', $2)`,
			theirMail, subject)
		return err
	})
	own := generatedsummary.StandingRecord{EntityType: "contact", EntityID: subject.UUID}
	quoting := generatedsummary.StandingRecord{EntityType: "company", EntityID: e.company(t).UUID}
	other := generatedsummary.StandingRecord{EntityType: "contact", EntityID: bystander.UUID}
	e.saveLine(t, own, "About the subject.", otherMail)
	e.saveLine(t, quoting, "Quotes the subject's mail.", theirMail)
	e.saveLine(t, other, "About someone else.", otherMail)

	e.inTx(t, func(tx pgx.Tx) error { return generatedsummary.ForgetContacts(e.ctx, tx, subject) })

	if e.line(t, own) != nil || e.line(t, quoting) != nil {
		t.Error("a line about the forgotten contact, or quoting their mail, survived")
	}
	if e.line(t, other) == nil {
		t.Error("the bystander's line went with another contact")
	}
}

func (e env) note(t *testing.T) ids.UUID {
	t.Helper()
	return e.inserted(t, `
		INSERT INTO activity (kind, subject, body, occurred_at, source, captured_by)
		VALUES ('note', 'Status', 'Body', now(), 'manual', 'human:summary-test') RETURNING id`)
}

func (e env) saveLine(t *testing.T, record generatedsummary.StandingRecord, text string, evidence ...ids.UUID) {
	t.Helper()
	e.inTx(t, func(tx pgx.Tx) error {
		return generatedsummary.SaveStandingLineTx(e.ctx, tx, record, generatedsummary.StandingLine{
			Text: text, Evidence: evidence, Language: "en", Fingerprint: text, WrittenAt: writtenAt,
		})
	})
}

func (e env) line(t *testing.T, record generatedsummary.StandingRecord) *generatedsummary.StandingLine {
	t.Helper()
	var out *generatedsummary.StandingLine
	e.inTx(t, func(tx pgx.Tx) error {
		line, found, err := generatedsummary.ReadStandingLineTx(e.ctx, tx, record)
		if found {
			out = &line
		}
		return err
	})
	return out
}
