// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"fmt"
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
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func latestSignatureOpening(t *testing.T, b *backfillWireEnv, id ids.UUID) events.Envelope {
	t.Helper()
	var encoded []byte
	if err := database.WithWorkspaceTx(b.env.Admin(), b.env.Pool, func(tx pgx.Tx) error {
		args := []any{id.String()}
		return tx.QueryRow(context.Background(), fmt.Sprintf(`SELECT envelope FROM event_outbox
   WHERE envelope->>'type'='activity.updated' AND envelope->'entity'->>'id'=$%d
   AND envelope->'payload'->'changed_fields'->>'audience'='workspace' ORDER BY seq DESC LIMIT 1`, len(args)), args...).Scan(&encoded)
	}); err != nil {
		t.Fatal(err)
	}
	var event events.Envelope
	if err := json.Unmarshal(encoded, &event); err != nil {
		t.Fatal(err)
	}
	return event
}

func TestAutomaticOpeningKeepsItsHistoryOriginAndAnOwnerCanRequestALiveRead(t *testing.T) {
	b := setupBackfillWire(t)
	if _, err := capture.NewSettings(NewSettingsStore(b.env.Pool), StoredConfig(b.env.Pool)).Update(b.env.Admin(), capture.SettingsPatch{MailSharing: new(true)}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.registry.SetMailPosture(b.human, "gmail", "classified", false); err != nil {
		t.Fatal(err)
	}
	seat := seatAddress(t, b.env, b.env.Rep1)
	run := strandedBackfill(t, b)
	source := storekit.WithBatch(threadConnectorCtx(b.env, b.env.Rep1), run)
	contact, err := b.env.Contacts.CreateContact(b.env.Admin(), contacts.CreateContactInput{FullName: "Alice Buyer", Source: "manual", Emails: []contacts.ContactEmailInput{{Email: "alice@opening.test", EmailType: "work", IsPrimary: true}}})
	if err != nil {
		t.Fatal(err)
	}
	raw := owedMail{from: "Alice Buyer <alice@opening.test>", to: seat, subject: "Product order", body: "Best regards,\nAlice Buyer\nDirector", at: clocktest.Now(t).Add(-24 * time.Hour), messageID: ids.NewV7().String() + "@opening.test"}.raw()
	parsed, err := mailmap.Parse(raw, seat)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := newCaptureSink(b.env.Pool, CaptureConfig{}).Upsert(source, parsed.ToRecord("gmail", raw))
	if err != nil {
		t.Fatal(err)
	}
	if got := activityAudience(t, b.env, ref.ID); got != "participants" {
		t.Fatalf("classified mail was born %q", got)
	}
	brain := &answerEveryThreadBrain{}
	engine := NewConfidentialityVerdictEngine(b.env.Pool, brain, b.handlers.log)
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), b.env.WS), 10); err != nil {
		t.Fatal(err)
	}
	if got := activityAudience(t, b.env, ref.ID); got != "workspace" {
		t.Fatalf("ordinary classifier left mail %q", got)
	}
	systemRepair := principal.SystemActing(b.env.Admin(), "system:contact_cohort_repair")
	if err := database.WithWorkspaceTx(systemRepair, b.env.Pool, func(tx pgx.Tx) error {
		_, err := b.env.Contacts.PromoteContactCohortTx(systemRepair, tx, ids.From[ids.ContactKind](ids.UUID(contact.Id)))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	opening := latestSignatureOpening(t, b, ref.ID)
	if opening.Actor.Type != string(principal.PrincipalSystem) {
		t.Fatalf("opening actor=%s", opening.Actor.Type)
	}
	inserter, err := jobs.NewInserter(b.env.Pool, b.handlers.log)
	if err != nil {
		t.Fatal(err)
	}
	trigger := NewCaptureEnrichTrigger(b.env.Pool, inserter, b.handlers.log)
	trigger.now = func() time.Time { return opening.OccurredAt.Add(2 * captureEnrichFreshWindow) }
	if err := trigger.HandleEvent(context.Background(), opening); err != nil {
		t.Fatal(err)
	}
	args := []any{ids.UUID(contact.Id), run}
	if got := b.env.WsCount(t, fmt.Sprintf(`SELECT count(*) FROM contact_signature_enrich_state WHERE contact_id=$%d AND deferred_run_id=$%d AND requested_generation>processed_generation`, len(args)-1, len(args)), args...); got != 1 {
		t.Fatalf("automatic history work was not deferred: %d", got)
	}
	system := principal.SystemActing(principal.WithWorkspaceID(context.Background(), b.env.WS), "agent:enrich")
	store := newContactsStore(InstallationDB(b.env.Pool))
	candidates, err := store.SignatureCandidatesForContacts(system, []ids.ContactID{ids.From[ids.ContactKind](ids.UUID(contact.Id))}, 100, true)
	if err != nil || len(candidates) != 0 {
		t.Fatalf("nightly reader bypassed history deferral: %+v %v", candidates, err)
	}
	assertAutomaticHistoryGraph(source, t, b, run, ids.UUID(contact.Id), seat, opening)
	args = []any{ref.ID}
	thread := b.env.WsScalar(t, fmt.Sprintf(`SELECT thread_key FROM activity WHERE id=$%d`, len(args)), args...)
	setter := NewThreadAudienceSetter(b.env.Pool)
	if _, err := setter.Decide(purgeCtx(b.env, b.env.Rep1), thread, false); err != nil {
		t.Fatal(err)
	}
	if _, err := setter.Decide(purgeCtx(b.env, b.env.Rep1), thread, true); err != nil {
		t.Fatal(err)
	}
	humanOpening := latestSignatureOpening(t, b, ref.ID)
	if humanOpening.Actor.Type != string(principal.PrincipalHuman) {
		t.Fatalf("owner opening actor=%s", humanOpening.Actor.Type)
	}
	trigger.now = func() time.Time { return humanOpening.OccurredAt }
	if err := trigger.HandleEvent(context.Background(), humanOpening); err != nil {
		t.Fatal(err)
	}
	work, err := store.PendingSignatureWork(system, 100)
	if err != nil || len(work) != 1 {
		t.Fatalf("owner request was held behind history: %+v %v", work, err)
	}
}

func assertAutomaticHistoryGraph(source context.Context, t *testing.T, b *backfillWireEnv, run, contact ids.UUID, seat string, opening events.Envelope) {
	t.Helper()
	gen := NewGraphEdgeConsumer(b.env.Pool)
	system := principal.SystemActing(principal.WithWorkspaceID(context.Background(), b.env.WS), "system:graph_edge_reconcile")
	if err := database.WithWorkspaceTx(system, b.env.Pool, func(tx pgx.Tx) error {
		history, err := graphHistoryForEventTx(system, tx, opening)
		if err != nil {
			return err
		}
		if history == nil || history.RunID != run || !history.Active || history.SourceXID == "" {
			t.Fatalf("automatic opening lost origin: %+v", history)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := gen.HandleEvent(context.Background(), opening); err != nil {
		t.Fatal(err)
	}
	raw := owedMail{from: "alice@opening.test", to: seat, subject: "Another product order", body: signed("Director"), at: clocktest.Now(t), messageID: ids.NewV7().String() + "@opening.test"}.raw()
	parsed, err := mailmap.Parse(raw, seat)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := newCaptureSink(b.env.Pool, CaptureConfig{}).Upsert(source, parsed.ToRecord("gmail", raw))
	if err != nil {
		t.Fatal(err)
	}
	if err := NewConfidentialityVerdictEngine(b.env.Pool, &answerEveryThreadBrain{}, b.handlers.log).RunWorkspace(system, 10); err != nil {
		t.Fatal(err)
	}
	repair := principal.SystemActing(system, "system:contact_cohort_repair")
	if err := database.WithWorkspaceTx(repair, b.env.Pool, func(tx pgx.Tx) error {
		_, err := b.env.Contacts.PromoteContactCohortTx(repair, tx, ids.From[ids.ContactKind](contact))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	second := latestSignatureOpening(t, b, ref.ID)
	if err := gen.HandleEvent(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	if got := b.env.WsCount(t, `SELECT count(*) FROM graph_interaction_edge WHERE contact_id=@contact AND count_total=1
 AND requested_generation>processed_generation AND deferred_run_id=@run`, pgx.NamedArgs{"contact": contact, "run": run}); got != 1 {
		t.Fatalf("automatic opening refolded the growing history: %d", got)
	}
}
