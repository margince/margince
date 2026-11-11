// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestOlderHistoricalMailCanFillATitleAfterANewerMailWasAlreadyRead(t *testing.T) {
	b := setupBackfillWire(t)
	if _, err := capture.NewSettings(NewSettingsStore(b.env.Pool), StoredConfig(b.env.Pool)).Update(b.env.Admin(), capture.SettingsPatch{MailSharing: new(true), SharedPostureAllowed: new(true), SignatureEnrich: new(true)}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.registry.SetMailPosture(b.human, "gmail", "shared", false); err != nil {
		t.Fatal(err)
	}
	if _, err := b.registry.SetSignatureEnrichment(b.human, "gmail", new(true)); err != nil {
		t.Fatal(err)
	}
	run := strandedBackfill(t, b)
	contact, err := b.env.Contacts.CreateContact(b.env.Admin(), contacts.CreateContactInput{FullName: "Alice Buyer", Source: "manual", Emails: []contacts.ContactEmailInput{{Email: "alice@older.test", EmailType: "work", IsPrimary: true}}})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.From[ids.ContactKind](ids.UUID(contact.Id))
	seat := seatAddress(t, b.env, b.env.Rep1)
	source := storekit.WithBatch(threadConnectorCtx(b.env, b.env.Rep1), run)
	write := func(days int, body string) {
		t.Helper()
		raw := owedMail{from: "alice@older.test", to: seat, subject: "Product order", body: body, at: clocktest.Now(t).Add(time.Duration(days) * 24 * time.Hour), messageID: ids.NewV7().String() + "@older.test"}.raw()
		parsed, err := mailmap.Parse(raw, seat)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := newCaptureSink(b.env.Pool, CaptureConfig{}).Upsert(source, parsed.ToRecord("gmail", raw)); err != nil {
			t.Fatal(err)
		}
		repair := principal.SystemActing(principal.WithWorkspaceID(context.Background(), b.env.WS), "system:contact_cohort_repair")
		if err := NewConfidentialityVerdictEngine(b.env.Pool, &answerEveryThreadBrain{}, b.handlers.log).RunWorkspace(repair, 10); err != nil {
			t.Fatal(err)
		}
		if err := database.WithWorkspaceTx(repair, b.env.Pool, func(tx pgx.Tx) error { _, err := b.env.Contacts.PromoteContactCohortTx(repair, tx, id); return err }); err != nil {
			t.Fatal(err)
		}
	}
	write(-1, "Best regards,\nAlice Buyer")
	system := principal.SystemActing(principal.WithWorkspaceID(context.Background(), b.env.WS), "agent:enrich")
	if err := database.WithWorkspaceTx(system, b.env.Pool, func(tx pgx.Tx) error { return contacts.QueueSignatureWorkTx(system, tx, []ids.ContactID{id}, nil) }); err != nil {
		t.Fatal(err)
	}
	// The newer mail states no title, so the model finds none in it.
	brain := &signatureScriptBrain{}
	reader := NewCaptureEnricher(b.env.Pool, brain, b.handlers.log)
	if _, err := reader.RunPendingWorkspace(system); err != nil {
		t.Fatal(err)
	}
	if got := b.env.WsCount(t, `SELECT count(*) FROM contact_signature_enrich_state WHERE contact_id=@contact AND activity_id IS NOT NULL`, pgx.NamedArgs{"contact": id}); got != 1 {
		t.Fatal("newer mail was not read")
	}
	brain.fields = statesTitle("Director")
	write(-2, "Best regards,\nAlice Buyer\nDirector")
	all := historyGraphEvents(t, b, run)
	if len(all) != 2 {
		t.Fatalf("captured events=%d", len(all))
	}
	inserter, err := jobs.NewInserter(b.env.Pool, b.handlers.log)
	if err != nil {
		t.Fatal(err)
	}
	trigger := NewCaptureEnrichTrigger(b.env.Pool, inserter, b.handlers.log)
	trigger.now = func() time.Time { return all[1].OccurredAt }
	if err := trigger.HandleEvent(context.Background(), all[1]); err != nil {
		t.Fatal(err)
	}
	if _, err := b.registry.CancelBackfill(b.human, "gmail", ids.From[ids.UserKind](b.env.Rep1)); err != nil {
		t.Fatal(err)
	}
	if _, err := releaseTerminalImportWork(system, b.env.Pool); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.RunPendingWorkspace(system); err != nil {
		t.Fatal(err)
	}
	if got := b.env.WsScalar(t, `SELECT COALESCE(title,'') FROM contact WHERE id=@contact`, pgx.NamedArgs{"contact": id}); got != "Director" {
		t.Fatalf("older newly imported signature left title=%q", got)
	}
}
