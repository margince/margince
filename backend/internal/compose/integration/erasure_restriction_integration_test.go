// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The erasure's second outcome (A165/ADR-0114): a Handelsbrief inside its
// statutory window is RESTRICTED rather than destroyed, the controller can see
// what is held and why, and the suspended erasure completes when the window
// closes — retain-only posture or not.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// restrictionFixture is one subject with a 400-day-old email about a won deal
// (a Handelsbrief) and a same-age note (ordinary), plus the delivery behind
// the email — the second copy of its addressing and substance.
type restrictionFixture struct {
	person, email, note, delivery, deal ids.UUID
}

func seedRestrictionFixture(t *testing.T, e *Env) restrictionFixture {
	t.Helper()
	f := restrictionFixture{person: ids.NewV7(), email: ids.NewV7(), note: ids.NewV7(), delivery: ids.NewV7()}
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		if _, err := tx.Exec(ctx,
			`INSERT INTO person (id, full_name, first_name, source, captured_by)
			 VALUES ($1, 'Held Subject', 'Held', 'manual', 'human:x')`, f.person); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO person_email (person_id, email, source, captured_by)
			 VALUES ($1, 'held@example.test', 'manual', 'human:x')`, f.person); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO activity (id, kind, subject, body, raw, counterparty_email, occurred_at, source, captured_by)
			 VALUES ($1, 'email', 'Angebot 2026-0042', 'Our offer, as discussed.', '{"provider":"payload"}'::jsonb,
			         'held@example.test', now() - interval '400 days', 'manual', 'human:x')`, f.email); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO activity (id, kind, subject, body, occurred_at, source, captured_by)
			 VALUES ($1, 'note', 'Internal jotting', 'Chase them next week.', now() - interval '400 days', 'manual', 'human:x')`,
			f.note); err != nil {
			return err
		}
		for _, a := range []ids.UUID{f.email, f.note} {
			if _, err := tx.Exec(ctx,
				`INSERT INTO activity_link (activity_id, entity_type, person_id)
				 VALUES ($1, 'person', $2)`, a, f.person); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO comms_outbound (id, activity_id, user_id, provider, message_id,
			                            recipients, cc, subject, body, consent_purpose,
			                            list_unsubscribe, status, sent_at, provider_message_id,
			                            bounced_at, bounce_kind, bounce_recipient)
			VALUES ($1, $2, $3, 'gmail', $4, jsonb_build_array('held@example.test'::text), '[]'::jsonb,
			        'Angebot 2026-0042', 'Our offer, as discussed.', 'transactional',
			        '<https://app.test/unsubscribe?tok=held>', 'sent', now() - interval '400 days', 'receipt-' || $4,
			        now() - interval '399 days', 'hard', 'held@example.test')`,
			f.delivery, f.email, e.Rep1, f.delivery.String()+"@margince.test")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	f.deal = e.SeedWonDealLinkedTo(t, f.email)
	return f
}

// TestErasureRestrictsAHandelsbriefInsteadOfDestroyingIt pins the split: the
// note is erased, the Handelsbrief is held — its substance intact, its
// identifiers gone, its deadline pinned, its evidence written — and both the
// tombstone and the outbox event say so.
func TestErasureRestrictsAHandelsbriefInsteadOfDestroyingIt(t *testing.T) {
	e := Setup(t)
	f := seedRestrictionFixture(t, e)

	if err := privacy.NewEraser(e.DB()).ErasePerson(e.Admin(), f.person, "test"); err != nil {
		t.Fatalf("erasing the subject → %v", err)
	}

	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		var subject, class, reason string
		var body, counterparty *string
		var raw []byte
		var restricted, archived, windowAhead bool
		var redacted []string
		if err := tx.QueryRow(ctx, `
			SELECT subject, body, raw, counterparty_email, restricted_at IS NOT NULL, archived_at IS NOT NULL,
			       restricted_until > restricted_at, coalesce(retention_class, ''), coalesce(restricted_reason, ''), redacted_fields
			  FROM activity WHERE id = $1`, f.email).Scan(&subject, &body, &raw, &counterparty, &restricted, &archived,
			&windowAhead, &class, &reason, &redacted); err != nil {
			return err
		}
		if subject != "Angebot 2026-0042" || body == nil || *body != "Our offer, as discussed." {
			return fmt.Errorf("the Handelsbrief's substance was destroyed: subject=%q body=%v", subject, body)
		}
		if raw != nil || counterparty != nil {
			return fmt.Errorf("the Handelsbrief kept the identifiers that ARE the subject: raw=%s counterparty=%v", raw, counterparty)
		}
		if !restricted || !archived || !windowAhead || class != "commercial_correspondence" || reason != "commercial_correspondence" {
			return fmt.Errorf("the Handelsbrief is not held as restricted: restricted=%v archived=%v window=%v class=%q reason=%q",
				restricted, archived, windowAhead, class, reason)
		}
		if fmt.Sprint(redacted) != "[raw counterparty_email]" {
			return fmt.Errorf("redacted_fields = %v, want the two identifier columns", redacted)
		}
		var evidence int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM activity_retention_evidence WHERE activity_id = $1 AND basis = 'deal_won' AND deal_id = $2`,
			f.email, f.deal).Scan(&evidence); err != nil {
			return err
		}
		if evidence != 1 {
			return fmt.Errorf("a pre-stamp Handelsbrief was restricted with %d evidence rows, want 1", evidence)
		}
		var noteSubject string
		var noteBody *string
		if err := tx.QueryRow(ctx, `SELECT subject, body FROM activity WHERE id = $1`, f.note).Scan(&noteSubject, &noteBody); err != nil {
			return err
		}
		if noteSubject != "Erased Subject" || noteBody != nil {
			return fmt.Errorf("the ordinary note was not erased: subject=%q body=%v", noteSubject, noteBody)
		}
		// The delivery: addressing gone, substance kept, and it says which.
		var recipients, deliverySubject, deliveryBody string
		var unsubscribe *string
		var deliveryRedacted []string
		var bounceRecipient *string
		if err := tx.QueryRow(ctx, `SELECT recipients::text, subject, body, list_unsubscribe, bounce_recipient, redacted_fields FROM comms_outbound WHERE id = $1`,
			f.delivery).Scan(&recipients, &deliverySubject, &deliveryBody, &unsubscribe, &bounceRecipient, &deliveryRedacted); err != nil {
			return err
		}
		if recipients != "[]" || unsubscribe != nil || deliverySubject != "Angebot 2026-0042" || deliveryBody != "Our offer, as discussed." {
			return fmt.Errorf("delivery not redacted per datum: recipients=%s unsubscribe=%v subject=%q body=%q",
				recipients, unsubscribe, deliverySubject, deliveryBody)
		}
		// The bounce's named recipient is addressing too: the redaction that
		// removes WHO a held delivery named must not leave the answer behind
		// on the bounce stamp.
		if bounceRecipient != nil {
			return fmt.Errorf("bounce_recipient survived the restriction: %v", *bounceRecipient)
		}
		if fmt.Sprint(deliveryRedacted) != "[bounce_recipient recipients list_unsubscribe]" {
			return fmt.Errorf("delivery redacted_fields = %v", deliveryRedacted)
		}
		// The proof and the announcement, in the same transaction.
		var tombstones, events int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE action = 'restrict' AND entity_type = 'activity' AND entity_id = $1
			  AND evidence->>'class' = 'commercial_correspondence' AND evidence->'deal_ids' ? $2`,
			f.email, f.deal.String()).Scan(&tombstones); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'retention.restricted' AND envelope->'entity'->>'id' = $1::text`,
			f.email).Scan(&events); err != nil {
			return err
		}
		if tombstones != 1 || events != 1 {
			return fmt.Errorf("restrict tombstones = %d, retention.restricted events = %d, want 1 and 1", tombstones, events)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	assertRestrictedListOverTheWire(t, e, f)
}

// assertRestrictedListOverTheWire reads the controller's list as JSON — the
// wire shape, not the store struct — so the assertion that no correspondence
// leaks is about what a client actually receives.
func assertRestrictedListOverTheWire(t *testing.T, e *Env, f restrictionFixture) {
	t.Helper()
	handlers := privacy.NewHandlers(e.DB(), nil)
	call := func(ctx context.Context) (int, map[string]any) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/v1/retention/restrictions", nil).WithContext(ctx)
		handlers.ListRestrictedActivities(rec, req, crmcontracts.ListRestrictedActivitiesParams{})
		var body map[string]any
		raw, err := io.ReadAll(rec.Body)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("list body is not JSON: %v — %s", err, raw)
		}
		return rec.Code, body
	}

	// The management role holds retention_policy all-false: withheld.
	if status, _ := call(retentionAdminCtx(e.WS, principal.ObjectGrant{})); status != http.StatusForbidden {
		t.Fatalf("a role without the retention authority read the restricted list: %d", status)
	}

	status, body := call(retentionAdminCtx(e.WS, principal.ObjectGrant{Read: true}))
	if status != http.StatusOK {
		t.Fatalf("listing restricted records → %d: %v", status, body)
	}
	data, ok := body["data"].([]any)
	if !ok || len(data) != 1 {
		t.Fatalf("restricted list = %v, want exactly the held Handelsbrief", body["data"])
	}
	row, ok := data[0].(map[string]any)
	if !ok {
		t.Fatalf("row is not an object: %v", data[0])
	}
	if row["activity_id"] != f.email.String() || row["kind"] != "email" {
		t.Errorf("row names %v/%v, want the held email", row["activity_id"], row["kind"])
	}
	if row["reason"] != "commercial_correspondence · §257 HGB / §147 AO" {
		t.Errorf("reason = %v", row["reason"])
	}
	for _, leak := range []string{"subject", "body", "counterparty_email", "raw"} {
		if _, present := row[leak]; present {
			t.Errorf("the restricted list carries %q — the correspondence is restricted precisely so it is not read", leak)
		}
	}
	deals := fmt.Sprint(row["deals"])
	if deals != fmt.Sprintf("[map[id:%s name:Floor fixture deal]]", f.deal) {
		t.Errorf("deals = %s, want the frozen name of the qualifying deal", deals)
	}
	if fmt.Sprint(row["redacted_fields"]) != "[raw counterparty_email]" {
		t.Errorf("redacted_fields = %v", row["redacted_fields"])
	}
}

// TestExpiredRestrictionCompletesTheSuspendedErasureUnderRetainOnly is A165 §2's
// last sentence: when the window closes the record is erased without anybody
// asking again, and the retain-only posture — which suspends the operator's
// storage-limitation ladder — does not suspend an Art. 17 request the engine
// itself held. The guard forbids moving a deadline nearer, so the test steps
// around it on its own database to close the window; that is a clock shift,
// not a second version of production.
func TestExpiredRestrictionCompletesTheSuspendedErasureUnderRetainOnly(t *testing.T) {
	e := Setup(t)
	f := seedRestrictionFixture(t, e)
	if err := privacy.NewEraser(e.DB()).ErasePerson(e.Admin(), f.person, "test"); err != nil {
		t.Fatalf("erasing the subject → %v", err)
	}
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := settings.SeedValue(context.Background(), tx, privacy.RetainOnly, true)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	svc := compose.NewRetentionServiceFor(e.DB(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))

	// A window still open is left exactly as it is by the pass.
	if err := svc.EvaluateInstallation(RetentionPassCtx(e.WS)); err != nil {
		t.Fatal(err)
	}
	if !restrictedRowHolds(t, e, f.email) {
		t.Fatal("the pass touched a held record whose window is still open")
	}

	owner := OwnerConn(t)
	ctx := context.Background()
	for _, stmt := range []string{
		`ALTER TABLE activity DISABLE TRIGGER activity_refuse_restricted_mutation`,
		`UPDATE activity SET restricted_until = restricted_at + interval '1 millisecond' WHERE id = '` + f.email.String() + `'`,
		`ALTER TABLE activity ENABLE TRIGGER activity_refuse_restricted_mutation`,
	} {
		if _, err := owner.Exec(ctx, stmt); err != nil {
			t.Fatalf("closing the window: %v", err)
		}
	}
	if err := svc.EvaluateInstallation(RetentionPassCtx(e.WS)); err != nil {
		t.Fatal(err)
	}

	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		var subject, body *string
		var stillRestricted bool
		var redacted []string
		if err := tx.QueryRow(ctx, `SELECT subject, body, restricted_at IS NOT NULL, redacted_fields FROM activity WHERE id = $1`,
			f.email).Scan(&subject, &body, &stillRestricted, &redacted); err != nil {
			return err
		}
		if subject != nil || body != nil || stillRestricted {
			return fmt.Errorf("the expired restriction did not complete the erasure: subject=%v body=%v restricted=%v", subject, body, stillRestricted)
		}
		if fmt.Sprint(redacted) != "[raw counterparty_email subject body]" {
			return fmt.Errorf("redacted_fields = %v", redacted)
		}
		var deliveryBody string
		if err := tx.QueryRow(ctx, `SELECT body FROM comms_outbound WHERE id = $1`, f.delivery).Scan(&deliveryBody); err != nil {
			return err
		}
		if deliveryBody != "" {
			return fmt.Errorf("the delivery kept its substance after the window closed: %q", deliveryBody)
		}
		var expired, erased int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE action = 'expire' AND entity_id = $1`, f.email).Scan(&expired); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'retention.applied' AND envelope->'entity'->>'id' = $1::text
			  AND envelope->'payload'->>'reason' = 'restriction_expired'`, f.email).Scan(&erased); err != nil {
			return err
		}
		if expired != 1 || erased != 1 {
			return fmt.Errorf("expire tombstones = %d, retention.applied events = %d, want 1 and 1", expired, erased)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// And the list no longer names it.
	handlers := privacy.NewHandlers(e.DB(), nil)
	rec := httptest.NewRecorder()
	handlers.ListRestrictedActivities(rec, httptest.NewRequest(http.MethodGet, "/v1/retention/restrictions", nil).
		WithContext(retentionAdminCtx(e.WS, principal.ObjectGrant{Read: true})), crmcontracts.ListRestrictedActivitiesParams{})
	var listed struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&listed); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("listing after expiry → %d %v", rec.Code, err)
	}
	if len(listed.Data) != 0 {
		t.Fatalf("an erased record is still listed as held: %s", listed.Data)
	}
}

// TestARestrictedRowRefusesEveryOrdinaryWrite pins the data-layer guard from
// the module's side: a write to a held record fails as a constraint refusal,
// which the erasure engine never attempts (its selectors exclude restricted
// rows) and which a caller must not retry.
func TestARestrictedRowRefusesEveryOrdinaryWrite(t *testing.T) {
	e := Setup(t)
	f := seedRestrictionFixture(t, e)
	if err := privacy.NewEraser(e.DB()).ErasePerson(e.Admin(), f.person, "test"); err != nil {
		t.Fatalf("erasing the subject → %v", err)
	}
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `UPDATE activity SET body = 'rewritten' WHERE id = $1`, f.email)
		return err
	})
	var pgErr interface{ SQLState() string }
	if !errors.As(err, &pgErr) || pgErr.SQLState() != "23514" {
		t.Fatalf("a write to a restricted row was not refused by the guard: %v", err)
	}
	// Erasing the subject again is idempotent over the held row: the restrict
	// step selects only unrestricted rows, so nothing is written twice and
	// nothing fails on the guard.
	if err := privacy.NewEraser(e.DB()).ErasePerson(e.Admin(), f.person, "test"); err != nil && !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("a second erasure over a held record failed: %v", err)
	}
}

// TestARestrictedRowLeavesEveryOrdinaryReadPath is A165 §2 from the reader's
// side, for the UNBOUNDED principal — the admin the link-walk spares is the
// one the availability test must still stop. The single-row read, the
// timeline including archived rows, and the record history all answer as if
// the row were gone; the Art. 15 package alone still reaches it; and a write
// — through the store's own guarded path or a raw statement alike — surfaces
// as the 423 the contract promises, with the deadline attached: the store's
// write path used to read the row LiveOnly before ever reaching the guard,
// so an authorized writer with the id in front of them was told the record
// did not exist; lockActivityForWrite now asks whether a row LiveOnly
// cannot see is gone or held before answering either way.
func TestARestrictedRowLeavesEveryOrdinaryReadPath(t *testing.T) {
	e := Setup(t)
	f := seedRestrictionFixture(t, e)
	admin := e.Admin()
	if err := privacy.NewEraser(e.DB()).ErasePerson(admin, f.person, "test"); err != nil {
		t.Fatalf("erasing the subject → %v", err)
	}
	held := ids.From[ids.ActivityKind](f.email)

	if _, err := e.Activities.GetActivity(admin, held, storekit.IncludeArchived); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a held record is readable by id, archived included: %v", err)
	}
	listed, _, err := e.Activities.ListActivities(admin, activities.ListActivitiesInput{IncludeArchived: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range listed {
		if ids.UUID(a.Id) == f.email {
			t.Errorf("a held record is on the timeline when archived rows are included")
		}
	}
	if _, err := privacy.ListRecordHistory(admin, e.DB(), privacy.RecordHistoryFilter{EntityType: "activity", EntityID: f.email}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a held record's history (which carries its pre-redaction images) is readable: %v", err)
	}

	// The store's own write path reaches the guard now: lockActivityForWrite
	// answers a genuinely missing row as before, but retries a row LiveOnly
	// cannot see once it confirms the row is HELD, so the write below reaches
	// activity_refuse_restricted_mutation and the transport answers the
	// refusal as the contract's 423 with the deadline, never as a 404 that
	// tells an authorized writer their record does not exist.
	subject := "rewritten"
	_, updateErr := e.Activities.UpdateActivity(admin, held, activities.UpdateActivityInput{Subject: &subject})
	if updateFault, ok := httperr.Classify(updateErr); !ok || updateFault.Status != http.StatusLocked || updateFault.Code != "locked" {
		t.Errorf("the store's write path answered %v (fault %+v), want 423 locked", updateErr, updateFault)
	}
	err = database.WithWorkspaceTx(admin, e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `UPDATE activity SET subject = 'rewritten' WHERE id = $1`, f.email)
		return err
	})
	fault, ok := httperr.Classify(err)
	if !ok || fault.Status != http.StatusLocked || fault.Code != "locked" {
		t.Fatalf("a write to a held record → %v (fault %+v), want 423 locked", err, fault)
	}
	if fault.Details["retain_until"] == nil {
		t.Errorf("the 423 does not say when the hold lifts: %+v", fault)
	}
}

func restrictedRowHolds(t *testing.T, e *Env, id ids.UUID) bool {
	t.Helper()
	var held bool
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT restricted_at IS NOT NULL AND body IS NOT NULL FROM activity WHERE id = $1`, id).Scan(&held)
	}); err != nil {
		t.Fatal(err)
	}
	return held
}

// controllerCtx is a named administrator holding the retention authority —
// the principal both overrides require. It uses a SEEDED user rather than a
// fresh id because a decision is attributed to a person the installation can
// name, and an id with no app_user row behind it is refused by design.
func controllerCtx(e *Env, grant principal.ObjectGrant) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.Rep1.String(), UserID: e.Rep1,
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"retention_policy": grant},
			RowScope: principal.RowScopeAll,
		},
	})
}

// TestAControllerReleasesAHeldRecordByErasingIt is A165 §4's release: the
// classification is a proxy for a legal judgement that belongs to the
// controller, so a record held wrongly can be let go — and letting go means
// ERASING, because the Art. 17 request the hold suspended is still
// outstanding. The reason is what makes it a decision rather than a toggle.
func TestAControllerReleasesAHeldRecordByErasingIt(t *testing.T) {
	e := Setup(t)
	f := seedRestrictionFixture(t, e)
	eraser := privacy.NewEraser(e.DB()).WithRawCapturePurger(compose.RawCapturePurgerFor(e.DB()))
	if err := eraser.ErasePerson(e.Admin(), f.person, "test"); err != nil {
		t.Fatalf("erasing the subject → %v", err)
	}
	const stated = "reviewed: a marketing enquiry, no transaction behind it"
	reason, err := privacy.ParseStatedReason(stated)
	if err != nil {
		t.Fatal(err)
	}

	// A role that may READ the list may not decide about it.
	if err := eraser.ReleaseRestriction(controllerCtx(e, principal.ObjectGrant{Read: true}), f.email, reason); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a read-only retention grant released a held record: %v", err)
	}
	// A reason nobody stated is refused before any transaction opens.
	if _, err := privacy.ParseStatedReason("   "); err == nil {
		t.Fatal("a blank reason was accepted — an unexplained decision is exactly what DEPACK-AC-5a forbids")
	}

	controller := controllerCtx(e, principal.ObjectGrant{Read: true, Update: true})
	if err := eraser.ReleaseRestriction(controller, f.email, reason); err != nil {
		t.Fatalf("releasing → %v", err)
	}

	err = database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		var subject, body *string
		var stillHeld bool
		if err := tx.QueryRow(ctx, `SELECT subject, body, restricted_at IS NOT NULL FROM activity WHERE id = $1`,
			f.email).Scan(&subject, &body, &stillHeld); err != nil {
			return err
		}
		if subject != nil || body != nil || stillHeld {
			return fmt.Errorf("the release did not erase: subject=%v body=%v held=%v", subject, body, stillHeld)
		}
		var deliveryBody string
		if err := tx.QueryRow(ctx, `SELECT body FROM comms_outbound WHERE id = $1`, f.delivery).Scan(&deliveryBody); err != nil {
			return err
		}
		if deliveryBody != "" {
			return fmt.Errorf("the delivery kept the substance the release destroyed: %q", deliveryBody)
		}
		// The decision is on the record, attributed and explained.
		var audited int
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM audit_log
			 WHERE action = 'release' AND entity_id = $1
			   AND evidence->>'reason' = $2 AND evidence->>'decided_by_name' = 'Rep'`,
			f.email, stated).Scan(&audited); err != nil {
			return err
		}
		var announced int
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM event_outbox
			 WHERE envelope->>'type' = 'retention.restricted'
			   AND envelope->'payload'->>'action' = 'release'
			   AND envelope->'entity'->>'id' = $1::text`, f.email).Scan(&announced); err != nil {
			return err
		}
		if audited != 1 || announced != 1 {
			return fmt.Errorf("release tombstones = %d, events = %d, want 1 and 1", audited, announced)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Releasing again has nothing left to release, and says so as a conflict
	// rather than as a record that was never there.
	if err := eraser.ReleaseRestriction(controller, f.email, reason); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("a second release → %v, want a conflict", err)
	}
	if err := eraser.ReleaseRestriction(controller, ids.NewV7(), reason); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("releasing a record that does not exist → %v, want not-found", err)
	}
}

// TestALegalHoldOutranksAControllerRelease pins the rule the sweep already
// keeps and the release used to skip: a litigation hold says somebody must
// keep this record, and it outranks the subject's Art. 17 request — and a
// controller's decision — until it is lifted. Destroying held evidence is
// spoliation, and it is the one thing neither path may do.
func TestALegalHoldOutranksAControllerRelease(t *testing.T) {
	e := Setup(t)
	f := seedRestrictionFixture(t, e)
	eraser := privacy.NewEraser(e.DB()).WithRawCapturePurger(compose.RawCapturePurgerFor(e.DB()))
	if err := eraser.ErasePerson(e.Admin(), f.person, "test"); err != nil {
		t.Fatalf("erasing the subject → %v", err)
	}
	// Counsel places the hold on the deal the correspondence hangs off, after
	// the record is already held under the statutory floor.
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `UPDATE deal SET legal_hold = true WHERE id = $1`, f.deal)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	reason, err := privacy.ParseStatedReason("reviewed: not commercial correspondence")
	if err != nil {
		t.Fatal(err)
	}
	controller := controllerCtx(e, principal.ObjectGrant{Read: true, Update: true})
	if err := eraser.ReleaseRestriction(controller, f.email, reason); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("a release under a legal hold → %v, want a conflict", err)
	}
	// Refused means the record is untouched, not half-erased.
	var body *string
	var stillHeld bool
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT body, restricted_at IS NOT NULL FROM activity WHERE id = $1`, f.email).Scan(&body, &stillHeld)
	}); err != nil {
		t.Fatal(err)
	}
	if body == nil || !stillHeld {
		t.Fatalf("the refused release destroyed evidence anyway: body=%v held=%v", body, stillHeld)
	}
	// Lifting the hold returns the decision to the controller.
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `UPDATE deal SET legal_hold = false WHERE id = $1`, f.deal)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := eraser.ReleaseRestriction(controller, f.email, reason); err != nil {
		t.Fatalf("releasing once the hold is lifted → %v", err)
	}
}

// TestAControllerPinsCorrespondenceTheDerivationCannotSee is A165 §4's other
// direction, and DEPACK-AC-5h's named case: supplier correspondence qualifies
// under §257 HGB and has no deal in this product to hang off, so no automatic
// rule here would find it. The controller says so, with a reason, and the
// record is held on exactly the same terms as a derived one.
func TestAControllerPinsCorrespondenceTheDerivationCannotSee(t *testing.T) {
	e := Setup(t)
	supplierMail := ids.NewV7()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`INSERT INTO activity (id, kind, subject, body, counterparty_email, occurred_at, source, captured_by)
			 VALUES ($1, 'email', 'Lieferschein 88-2026', 'Delivery note attached.', 'supplier@parts.test',
			         now() - interval '30 days', 'manual', 'human:x')`, supplierMail)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	eraser := privacy.NewEraser(e.DB())
	reason, err := privacy.ParseStatedReason("supplier correspondence: §257 HGB, no deal in the CRM")
	if err != nil {
		t.Fatal(err)
	}
	controller := controllerCtx(e, principal.ObjectGrant{Read: true, Update: true})
	if err := eraser.PinToFloor(controller, supplierMail, reason); err != nil {
		t.Fatalf("pinning → %v", err)
	}

	err = database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		var body *string
		var counterparty *string
		var held, windowAhead bool
		var class string
		if err := tx.QueryRow(ctx, `
			SELECT body, counterparty_email, restricted_at IS NOT NULL, restricted_until > now(), coalesce(retention_class, '')
			  FROM activity WHERE id = $1`, supplierMail).Scan(&body, &counterparty, &held, &windowAhead, &class); err != nil {
			return err
		}
		// The substance stays — that is what the obligation keeps — and the
		// identifier that names the counterparty goes.
		if body == nil || *body != "Delivery note attached." || counterparty != nil {
			return fmt.Errorf("the pinned record is not redacted per datum: body=%v counterparty=%v", body, counterparty)
		}
		if !held || !windowAhead || class != "commercial_correspondence" {
			return fmt.Errorf("the pin did not hold the record: held=%v window=%v class=%q", held, windowAhead, class)
		}
		// The evidence names WHO decided and why — a pin names no deal, so the
		// attribution is what substantiates it.
		var attributed int
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM activity_retention_evidence
			 WHERE activity_id = $1 AND basis = 'controller_pin' AND deal_id IS NULL
			   AND decided_by = $2 AND decided_by_name = 'Rep' AND length(btrim(reason)) > 0`,
			supplierMail, e.Rep1).Scan(&attributed); err != nil {
			return err
		}
		if attributed != 1 {
			return fmt.Errorf("the pin left %d attributed evidence rows, want 1", attributed)
		}
		var announced int
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM event_outbox
			 WHERE envelope->>'type' = 'retention.restricted'
			   AND envelope->'payload'->>'action' = 'pin'
			   AND envelope->'entity'->>'id' = $1::text`, supplierMail).Scan(&announced); err != nil {
			return err
		}
		if announced != 1 {
			return fmt.Errorf("pin events = %d, want 1", announced)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// A pin is not a way to extend a window already running.
	if err := eraser.PinToFloor(controller, supplierMail, reason); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("pinning an already-held record → %v, want a conflict", err)
	}
	// And the record is now on the controller's list, held with no deal named.
	page, err := privacy.ListRestrictedActivities(controller, e.DB(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Records) != 1 || page.Records[0].ActivityID != supplierMail || len(page.Records[0].Deals) != 0 {
		t.Errorf("the pinned record is not listed as held with no transaction: %+v", page.Records)
	}
}

// pinnedMailWithAnOriginal seeds the lane that reaches an erasure with NO
// Art. 17 request in it: a captured supplier mail, the verbatim provider
// original behind it, and a controller's pin to the statutory floor.
//
// The original matters because the pin path has no prior purge. A restriction
// derived from an erasure request had its raw row deleted at request time by
// purgeDerivedTraces, which is why the gap this fixture drives reads clean at a
// glance — it is only reachable from PinToFloor.
func pinnedMailWithAnOriginal(t *testing.T, e *Env) (activity ids.UUID, sourceID string) {
	t.Helper()
	activity, sourceID = ids.NewV7(), "supplier-"+ids.NewV7().String()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity (id, kind, subject, body, counterparty_email, occurred_at,
			                      source, source_system, source_id, captured_by)
			VALUES ($1, 'email', 'Lieferschein 88-2026', 'Delivery note attached.', 'supplier@parts.test',
			        now() - interval '30 days', 'capture_email', 'imap', $2, 'human:x')`,
			activity, sourceID); err != nil {
			return err
		}
		// The verbatim original, joined on the pair every erasure here keeps.
		_, err := tx.Exec(ctx, `
			INSERT INTO raw_capture (source_system, source_id, payload)
			VALUES ('imap', $1, $2::jsonb)`,
			sourceID, `{"subject":"Lieferschein 88-2026","body":"Delivery note attached."}`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	eraser := privacy.NewEraser(e.DB()).WithRawCapturePurger(compose.RawCapturePurgerFor(e.DB()))
	reason, err := privacy.ParseStatedReason("supplier correspondence: §257 HGB, no deal in the CRM")
	if err != nil {
		t.Fatal(err)
	}
	if err := eraser.PinToFloor(controllerCtx(e, principal.ObjectGrant{Read: true, Update: true}), activity, reason); err != nil {
		t.Fatalf("pinning → %v", err)
	}
	return activity, sourceID
}

// originalsBehind counts the provider originals still standing for a source id.
func originalsBehind(t *testing.T, e *Env, sourceID string) int {
	t.Helper()
	var n int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM raw_capture WHERE source_system = 'imap' AND source_id = $1`,
			sourceID).Scan(&n)
	}); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestAFloorExpiringDestroysTheProviderOriginalToo — the chain #2803 describes,
// with no erasure request anywhere in it.
//
// A controller pins an ordinary captured message to its statutory floor. The
// pin deliberately KEEPS the body — that is what the obligation preserves — so
// nothing has purged the raw row at any point. The window closes, the sweep
// lifts the restriction and destroys subject/body/raw/counterparty_email while
// keeping source_system/source_id, and the verbatim original stood, joined on
// exactly that pair, for an Art. 15 export to serve back.
//
// The lift arm and the erase arm had drifted in the direction the file's own
// comment forbids: "a record must not be more thoroughly erased by the clock
// than by a controller's decision".
func TestAFloorExpiringDestroysTheProviderOriginalToo(t *testing.T) {
	e := Setup(t)
	activity, sourceID := pinnedMailWithAnOriginal(t, e)
	if originalsBehind(t, e, sourceID) != 1 {
		t.Fatal("the pin did not leave the original standing, so this case would pass for the wrong reason")
	}

	owner, ctx := OwnerConn(t), context.Background()
	for _, stmt := range []string{
		`ALTER TABLE activity DISABLE TRIGGER activity_refuse_restricted_mutation`,
		`UPDATE activity SET restricted_until = restricted_at + interval '1 millisecond' WHERE id = '` + activity.String() + `'`,
		`ALTER TABLE activity ENABLE TRIGGER activity_refuse_restricted_mutation`,
	} {
		if _, err := owner.Exec(ctx, stmt); err != nil {
			t.Fatalf("closing the window: %v", err)
		}
	}
	svc := compose.NewRetentionServiceFor(e.DB(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := svc.EvaluateInstallation(RetentionPassCtx(e.WS)); err != nil {
		t.Fatal(err)
	}

	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		var body *string
		if err := tx.QueryRow(ctx, `SELECT body FROM activity WHERE id = $1`, activity).Scan(&body); err != nil {
			return err
		}
		if body != nil {
			return fmt.Errorf("the lift left the body %q", *body)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got := originalsBehind(t, e, sourceID); got != 0 {
		t.Errorf("%d provider original(s) survive the floor expiring — the parsed copy is gone and an Art. 15 export still serves the verbatim one, joined on the pair the lift keeps", got)
	}
}

// TestAControllerReleaseDestroysTheProviderOriginalToo — the same gap on the
// path a person takes rather than the clock.
//
// The file's invariant runs both ways: a release is the controller completing
// the erasure the restriction suspended, so it must reach everything the sweep
// reaches.
func TestAControllerReleaseDestroysTheProviderOriginalToo(t *testing.T) {
	e := Setup(t)
	activity, sourceID := pinnedMailWithAnOriginal(t, e)

	eraser := privacy.NewEraser(e.DB()).WithRawCapturePurger(compose.RawCapturePurgerFor(e.DB()))
	reason, err := privacy.ParseStatedReason("the supplier obligation ended: contract closed out")
	if err != nil {
		t.Fatal(err)
	}
	if err := eraser.ReleaseRestriction(controllerCtx(e, principal.ObjectGrant{Read: true, Update: true, Delete: true}), activity, reason); err != nil {
		t.Fatalf("releasing → %v", err)
	}
	if got := originalsBehind(t, e, sourceID); got != 0 {
		t.Errorf("%d provider original(s) survive a controller's release", got)
	}
}

// TestAnEraserWithNoPurgerRefusesRatherThanErasingHalf — the seam is not
// optional, and the refusal is what says so.
//
// There is no second path that ages raw_capture out: the Art. 17 cascade's
// purge is scoped to a PERSON where a retention window is scoped to time. So an
// unwired purger is not a degraded mode that something else corrects later — it
// is an erasure that reports success over an intact original, which is the one
// outcome worse than refusing.
func TestAnEraserWithNoPurgerRefusesRatherThanErasingHalf(t *testing.T) {
	e := Setup(t)
	activity, sourceID := pinnedMailWithAnOriginal(t, e)

	unwired := privacy.NewEraser(e.DB())
	reason, err := privacy.ParseStatedReason("the supplier obligation ended: contract closed out")
	if err != nil {
		t.Fatal(err)
	}
	err = unwired.ReleaseRestriction(controllerCtx(e, principal.ObjectGrant{Read: true, Update: true, Delete: true}), activity, reason)
	if !errors.Is(err, privacy.ErrRetentionSeamMissing) {
		t.Errorf("a release through an eraser with no raw-capture purger answered %v, want the missing-seam refusal", err)
	}
	// And it refused rather than half-erasing: the original stands because
	// nothing was destroyed, not because the purge was skipped.
	if got := originalsBehind(t, e, sourceID); got != 1 {
		t.Errorf("%d provider original(s) after a refused release, want the record untouched", got)
	}
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		var held bool
		if err := tx.QueryRow(context.Background(),
			`SELECT restricted_at IS NOT NULL FROM activity WHERE id = $1`, activity).Scan(&held); err != nil {
			return err
		}
		if !held {
			return fmt.Errorf("the refused release still lifted the restriction")
		}
		return nil
	}); err != nil {
		t.Error(err)
	}
}
