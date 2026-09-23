// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// An approval card is written in the installation's base language.
//
// The census in compose proves every language HAS the sentence; only a real
// stager against a real settings row proves the right one is picked. Stagers
// resolve the language one of two ways — on the transaction they already hold,
// or through the pool — so one of each is driven through its real entry point
// and its stored summary read back.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// holdAndReadSummary stages one held message through the production notifier
// and answers the summary its card was stored with.
func holdAndReadSummary(ctx context.Context, t *testing.T, e *Env) string {
	t.Helper()
	notifier := compose.NewScheduledSendHeldNotifier(approvals.NewService(e.DB()))
	notice := activities.HeldNotice{
		ScheduledSendID: ids.NewV7(),
		ScheduledBy:     e.Rep1,
		Reason:          activities.HeldSendRefused,
		Subject:         "Angebot Q3",
		ScheduledAt:     time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC),
	}
	var summary string
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		if err := notifier.NotifyHeldInTx(ctx, tx, notice); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `
			SELECT summary FROM approval
			 WHERE kind = $1 AND proposed_change->>'scheduled_send_id' = $2`,
			approvals.KindScheduledSendHeld, notice.ScheduledSendID.String()).Scan(&summary)
	}); err != nil {
		t.Fatalf("staging the held message's card: %v", err)
	}
	return summary
}

func TestAHeldSendCardIsWrittenInTheInstallationsBaseLanguage(t *testing.T) {
	e := Setup(t)
	ctx := e.Admin()

	setBaseLanguage(ctx, t, e.Pool, "de")
	german := `"Angebot Q3" wurde nicht gesendet: eine Prüfung hat den Versand zum Sendezeitpunkt abgelehnt`
	if got := holdAndReadSummary(ctx, t, e); got != german {
		t.Errorf("the installation is set to German and the card reads %q, want %q", got, german)
	}

	// The other direction catches a resolver that happens to return a
	// constant: a test asserting only German passes against code that always
	// writes German.
	setBaseLanguage(ctx, t, e.Pool, "en")
	english := `"Angebot Q3" was not sent: a gate refused it at send time`
	if got := holdAndReadSummary(ctx, t, e); got != english {
		t.Errorf("the installation was changed to English and the card reads %q, want %q", got, english)
	}
}

// seedLinkedInLookalike files one LinkedIn connection whose name folds onto a
// contact's without equalling it, the match that goes to a human.
func seedLinkedInLookalike(ctx context.Context, t *testing.T, e *Env, company ids.UUID, connection, folded, contactName string) {
	t.Helper()
	created, err := e.Contacts.CreateContact(ctx, contacts.CreateContactInput{FullName: contactName, Source: "manual"})
	if err != nil {
		t.Fatalf("seeding the contact %s: %v", contactName, err)
	}
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO relationship (kind, contact_id, company_id, source, captured_by)
			VALUES ('employment', $1, $2, 'manual', 'human:test')`, ids.UUID(created.Id), company); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO linkedin_connection
			    (owner_user_id, full_name, normalized_name, company_name, normalized_company, matched_company_id, source)
			VALUES ($1, $2, $3, 'Acme GmbH', 'acme', $4, 'csv_export')`, e.Rep1, connection, folded, company)
		return err
	}); err != nil {
		t.Fatalf("seeding the connection %s: %v", connection, err)
	}
}

// stageLinkedInAndReadSummary runs the production pool-path stager and answers
// the summary stored for the contact named.
func stageLinkedInAndReadSummary(ctx context.Context, t *testing.T, e *Env, contactName string) string {
	t.Helper()
	store := contacts.NewStore(e.DB())
	if _, err := store.MatchLinkedInConnections(ctx, e.Rep1); err != nil {
		t.Fatalf("matching the connections: %v", err)
	}
	if _, err := compose.StageLinkedInMatches(ctx, e.Pool, approvals.NewService(e.DB()), store); err != nil {
		t.Fatalf("staging the matches: %v", err)
	}
	var summary string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT summary FROM approval
			 WHERE kind = 'linkedin_match' AND proposed_change->>'contact_name' = $1`, contactName).Scan(&summary)
	}); err != nil {
		t.Fatalf("reading the staged match for %s: %v", contactName, err)
	}
	return summary
}

// The pool path, which opens its own transaction to read the setting. Two
// lookalikes, so the English staging writes a new card rather than joining the
// German one.
func TestALinkedInMatchCardIsWrittenInTheInstallationsBaseLanguage(t *testing.T) {
	e := Setup(t)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, AdminPerms)
	var company ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			INSERT INTO company (display_name, source, captured_by)
			VALUES ('Acme GmbH', 'manual', 'human:test') RETURNING id`).Scan(&company)
	}); err != nil {
		t.Fatalf("seeding the account: %v", err)
	}

	setBaseLanguage(ctx, t, e.Pool, "de")
	seedLinkedInLookalike(ctx, t, e, company, "Andreas Müller", "andreas muller", "Andreas Muller")
	german := "Andreas Müller bei Acme GmbH scheint Andreas Muller zu sein"
	if got := stageLinkedInAndReadSummary(ctx, t, e, "Andreas Muller"); got != german {
		t.Errorf("the installation is set to German and the card reads %q, want %q", got, german)
	}

	setBaseLanguage(ctx, t, e.Pool, "en")
	seedLinkedInLookalike(ctx, t, e, company, "Björn Lindström", "bjorn lindstrom", "Bjorn Lindstrom")
	english := "Björn Lindström at Acme GmbH looks like Bjorn Lindstrom"
	if got := stageLinkedInAndReadSummary(ctx, t, e, "Bjorn Lindstrom"); got != english {
		t.Errorf("the installation was changed to English and the card reads %q, want %q", got, english)
	}
}
