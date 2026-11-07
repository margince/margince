// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func latestDerivedEvent(t *testing.T, e *integration.Env, kind string, id ids.UUID, field string) events.Envelope {
	t.Helper()
	var encoded []byte
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT envelope FROM event_outbox
 WHERE envelope->>'type'=@kind AND envelope->'entity'->>'id'=@id
 AND (@field::text='' OR envelope->'payload'->'changed_fields' ? @field::text)
 ORDER BY seq DESC LIMIT 1`, pgx.NamedArgs{"kind": kind, "id": id.String(), "field": field}).Scan(&encoded)
	}); err != nil {
		t.Fatal(err)
	}
	var env events.Envelope
	if err := json.Unmarshal(encoded, &env); err != nil {
		t.Fatal(err)
	}
	return env
}

func TestAHistoryMintedContactDoesNotTurnItsLiveMailIntoHistory(t *testing.T) {
	b := setupBackfillWire(t)
	if _, err := capture.NewSettings(NewSettingsStore(b.env.Pool), StoredConfig(b.env.Pool)).Update(b.env.Admin(), capture.SettingsPatch{MailSharing: new(true), SignatureEnrich: new(true)}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.registry.SetMailPosture(b.human, "gmail", "classified", false); err != nil {
		t.Fatal(err)
	}
	if _, err := b.registry.SetSignatureEnrichment(b.human, "gmail", new(true)); err != nil {
		t.Fatal(err)
	}
	run := strandedBackfill(t, b)
	seat := seatAddress(t, b.env, b.env.Rep1)
	write := func(history bool) ids.UUID {
		t.Helper()
		ctx := threadConnectorCtx(b.env, b.env.Rep1)
		if history {
			ctx = storekit.WithBatch(ctx, run)
		}
		days, subject := -1, "Current order"
		if history {
			days = -365
			subject = "Historical order"
		}
		raw := owedMail{from: "alice@cohort-origin.test", to: seat, subject: subject, body: "Best regards,\nAlice Buyer\nDirector", at: clocktest.Now(t).Add(time.Duration(days) * 24 * time.Hour), messageID: ids.NewV7().String() + "@cohort-origin.test"}.raw()
		parsed, err := mailmap.Parse(raw, seat)
		if err != nil {
			t.Fatal(err)
		}
		ref, err := newCaptureSink(b.env.Pool, CaptureConfig{}).Upsert(ctx, parsed.ToRecord("gmail", raw))
		if err != nil {
			t.Fatal(err)
		}
		return ref.ID
	}
	historical, live := write(true), write(false)
	contact, err := b.env.Contacts.CreateContact(storekit.WithBatch(b.env.Admin(), run), contacts.CreateContactInput{FullName: "Alice Buyer", Source: "manual", Emails: []contacts.ContactEmailInput{{Email: "alice@cohort-origin.test", EmailType: "work", IsPrimary: true}}})
	if err != nil {
		t.Fatal(err)
	}
	system := principal.SystemActing(principal.WithWorkspaceID(context.Background(), b.env.WS), "agent:enrich")
	if err := NewConfidentialityVerdictEngine(b.env.Pool, &answerEveryThreadBrain{}, b.handlers.log).RunWorkspace(system, 10); err != nil {
		t.Fatal(err)
	}
	created := latestDerivedEvent(t, b.env, "contact.created", ids.UUID(contact.Id), "")
	if err := NewCohortPromoteGen(b.env.Pool, b.env.Contacts, b.handlers.log).HandleEvent(system, created); err != nil {
		t.Fatal(err)
	}
	inserter, err := jobs.NewInserter(b.env.Pool, b.handlers.log)
	if err != nil {
		t.Fatal(err)
	}
	trigger := NewCaptureEnrichTrigger(b.env.Pool, inserter, b.handlers.log)
	for _, test := range []struct {
		id      ids.UUID
		history bool
	}{{historical, true}, {live, false}} {
		env := latestDerivedEvent(t, b.env, "activity.updated", test.id, "relinked")
		if events.IsImported(env) {
			t.Fatal("a cohort repair changed the existing bulk-import side-effect policy")
		}
		if err := database.WithWorkspaceTx(system, b.env.Pool, func(tx pgx.Tx) error {
			origin, err := graphHistoryForEventTx(system, tx, env)
			if err != nil {
				return err
			}
			if (origin != nil) != test.history {
				t.Fatalf("mail %s origin=%+v history=%t", test.id, origin, test.history)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		trigger.now = func() time.Time { return env.OccurredAt }
		if err := trigger.HandleEvent(context.Background(), env); err != nil {
			t.Fatal(err)
		}
	}
	work, err := newContactsStore(InstallationDB(b.env.Pool)).PendingSignatureWork(system, 100)
	if err != nil || len(work) != 1 || work[0].ContactID.UUID != ids.UUID(contact.Id) {
		t.Fatalf("live cohort repair did not retain priority: %+v %v", work, err)
	}
}

func TestAContactRelinkDoesNotReopenAnotherSendersSignature(t *testing.T) {
	h := setupHistory(t)
	integration.ApplyRiverSchema(t)
	mail := h.writes(t, 0, signed("Director"))
	h.brain.answers[mail] = statesTitle("Director")
	h.pass(t)
	alias, err := h.e.Contacts.CreateContact(h.e.Admin(), contacts.CreateContactInput{FullName: "Another Buyer", Source: "manual", Emails: []contacts.ContactEmailInput{{Email: "another@customer.example", EmailType: "work", IsPrimary: true}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.e.Activities.RelinkActivity(h.e.Admin(), ids.From[ids.ActivityKind](mail), activities.RelinkActivityInput{EntityType: "contact", EntityID: ids.UUID(alias.Id)}); err != nil {
		t.Fatal(err)
	}
	event := latestDerivedEvent(t, h.e, "activity.updated", mail, "relinked")
	log := slog.New(slog.DiscardHandler)
	inserter, err := jobs.NewInserter(h.e.Pool, log)
	if err != nil {
		t.Fatal(err)
	}
	trigger := NewCaptureEnrichTrigger(h.e.Pool, inserter, log)
	trigger.now = func() time.Time { return event.OccurredAt }
	if err := trigger.HandleEvent(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	system := principal.SystemActing(principal.WithWorkspaceID(context.Background(), h.e.WS), "agent:enrich")
	work, err := newContactsStore(InstallationDB(h.e.Pool)).PendingSignatureWork(system, 100)
	if err != nil || len(work) != 0 {
		t.Fatalf("relink requested unrelated signatures: %+v %v", work, err)
	}
	if got := h.e.WsCount(t, `SELECT count(*) FROM contact_signature_enrich_state WHERE contact_id=@contact AND activity_id=@mail`, pgx.NamedArgs{"contact": h.contact, "mail": mail}); got != 1 {
		t.Fatal("relink cleared an existing contact's watermark")
	}
}
