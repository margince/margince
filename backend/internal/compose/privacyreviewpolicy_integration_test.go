// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestErasureExplainsMissingReviewerAndRollsBack(t *testing.T) {
	e := integration.Setup(t)
	subject := e.SeedContact(t, "Anna Berg", nil)
	company := e.SeedCompany(t, "Review company", nil)
	e.WsExec(t, `UPDATE company SET description='Anna Berg is mentioned here' WHERE id=$1`, company)
	err := privacy.NewEraser(e.DB()).WithRawCapturePurger(RawCapturePurgerFor(e.DB())).EraseContact(e.Admin(), subject, "Subject requested erasure")
	var refusal *privacyReviewUnavailableError
	if !errors.As(err, &refusal) || !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("missing reviewer must explain a conflict: %v", err)
	}
	code, message := refusal.MessageFault()
	if code != "conflict" || !strings.Contains(message, "administrator") || !strings.Contains(message, "retry") {
		t.Fatalf("unhelpful refusal: %s %s", code, message)
	}
	if got := e.WsScalar(t, `SELECT full_name FROM contact WHERE id=$1`, subject); got != "Anna Berg" {
		t.Fatalf("partial erasure: %s", got)
	}
}

func TestErasureFlagsCompanyTemplatesWithoutChangingThem(t *testing.T) {
	e := integration.Setup(t)
	e.WsExec(t, `INSERT INTO role_assignment(user_id,role_id) SELECT $1,id FROM role WHERE key='admin'`, e.AdminUser)
	subject := e.SeedContact(t, "Anna Berg", nil)
	store := contacts.NewStore(InstallationDB(e.Pool)).WithSettings(StoredConfig(e.Pool))
	if _, err := store.SaveSignatureTemplate(e.Admin(), "<p>Anna Berg</p>"); err != nil {
		t.Fatal(err)
	}
	entity, err := store.CreateLegalEntity(e.Admin(), contacts.LegalEntityInput{DisplayName: new("Review company"), LegalName: new("Review company GmbH"), Country: new("at"), SignatureTemplate: new("<p>Anna Berg</p>")})
	if err != nil {
		t.Fatal(err)
	}
	if err := privacy.NewEraser(e.DB()).WithRawCapturePurger(RawCapturePurgerFor(e.DB())).EraseContact(e.Admin(), subject, "Subject requested erasure"); err != nil {
		t.Fatal(err)
	}
	if got := e.WsScalar(t, `SELECT signature_template FROM legal_entity WHERE id=$1`, entity.ID); got != "<p>Anna Berg</p>" {
		t.Fatal(got)
	}
	if got := e.WsScalar(t, `SELECT value #>> '{}' FROM setting WHERE key='contacts.signature_template'`); got != "<p>Anna Berg</p>" {
		t.Fatal(got)
	}
	for _, kind := range []string{"legal_entity", "signature_template"} {
		if n := e.WsCount(t, `SELECT count(*) FROM notice WHERE kind='privacy_review_required' AND target_type=$1 AND recipient_user_id=$2`, kind, e.AdminUser); n != 1 {
			t.Fatalf("%s review flags: %d", kind, n)
		}
	}
}

func TestErasureFlagsAnInactiveMembersSignatureWithoutOpeningTheControllersOwn(t *testing.T) {
	e := integration.Setup(t)
	e.WsExec(t, `INSERT INTO role_assignment(user_id,role_id) SELECT $1,id FROM role WHERE key='admin'`, e.AdminUser)
	subject, signature := e.SeedContact(t, "Anna Berg", nil), ids.NewV7()
	e.WsExec(t, `INSERT INTO email_signature(id,owner_id,body) VALUES($1,$2,'Anna Berg')`, signature, e.Rep1)
	e.WsExec(t, `UPDATE app_user SET status='deactivated' WHERE id=$1`, e.Rep1)
	if err := privacy.NewEraser(e.DB()).WithRawCapturePurger(RawCapturePurgerFor(e.DB())).EraseContact(e.Admin(), subject, "Subject requested erasure"); err != nil {
		t.Fatal(err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM notice WHERE kind='privacy_review_required' AND recipient_user_id=$1 AND target_type IS NULL AND body LIKE '%'||$2::uuid::text||'%' AND body LIKE '%'||$3::uuid::text||'%'`, e.AdminUser, signature, e.Rep1); n != 1 {
		t.Fatalf("inactive signature review count=%d", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM notice WHERE kind='privacy_review_required' AND recipient_user_id=$1`, e.Rep1); n != 0 {
		t.Fatal("notified departed owner")
	}
}

func TestMarkerCorrectionDefersAWriteConflictWithoutAbortingItsTransaction(t *testing.T) {
	e := integration.Setup(t)
	record := legacyWeeklyCopies(t, e, "")
	contact, company := e.SeedContact(t, "Ordinary contact", nil), e.SeedCompany(t, "Ordinary company", nil)
	ctx := principal.SystemActing(e.Admin(), "system:marker-test")
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(business pgx.Tx) error {
		if _, err := business.Exec(ctx, `UPDATE contact SET legal_hold=false WHERE id=$1`, contact); err != nil {
			return err
		}
		return database.WithWorkspaceTx(ctx, e.Pool, func(cleanup pgx.Tx) error {
			page, err := privacy.RedactMarkedWeeklyAuditPageTx(ctx, cleanup, ids.UUID{}, ids.NewV7(), 200, nil)
			if err != nil {
				return err
			}
			if page.Changed != 0 {
				t.Fatalf("changed %d records while CRM write holds a lock", page.Changed)
			}
			if _, err := business.Exec(ctx, `UPDATE company SET legal_hold=false WHERE id=$1`, company); err != nil {
				return err
			}
			var alive int
			if err := cleanup.QueryRow(ctx, `SELECT 1`).Scan(&alive); err != nil {
				return err
			}
			if alive != 1 {
				t.Fatal("cleanup transaction did not survive deferral")
			}
			return nil
		})
	}); err != nil {
		t.Fatal(err)
	}
	runMachineAddressTask(ctx, t, e, markedAuditRedactionDataTask())
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type='weekly_plan_commitment' AND entity_id=$1 AND after->>'label'='Private Contact'`, record); n != 0 {
		t.Fatalf("retry kept %d old images", n)
	}
}

func TestMarkerCorrectionDefersAReadLockOnARightsRequest(t *testing.T) {
	e := integration.Setup(t)
	request := legacyRequestAuditCopies(t, e)
	company := e.SeedCompany(t, "Ordinary company", nil)
	ctx := principal.SystemActing(e.Admin(), "system:marker-test")
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(business pgx.Tx) error {
		var locked ids.UUID
		if err := business.QueryRow(ctx, `SELECT id FROM data_subject_request WHERE id=$1 FOR UPDATE`, request).Scan(&locked); err != nil {
			return err
		}
		if err := database.WithWorkspaceTx(ctx, e.Pool, func(cleanup pgx.Tx) error {
			if _, err := cleanup.Exec(ctx, `SET LOCAL lock_timeout='200ms'`); err != nil {
				return err
			}
			page, err := privacy.RedactRetiredRequestAuditPageTx(ctx, cleanup, ids.UUID{}, ids.NewV7(), privacy.MarkerAuditPageSize, nil)
			if err != nil {
				return err
			}
			if page.Changed != 0 {
				t.Fatal("changed a request another transaction is using")
			}
			return nil
		}); err != nil {
			return err
		}
		_, err := business.Exec(ctx, `UPDATE company SET legal_hold=false WHERE id=$1`, company)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	runMachineAddressTask(ctx, t, e, markedAuditRedactionDataTask())
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type='data_subject_request' AND entity_id=$1 AND after->>'email'='private@example.test'`, request); n != 0 {
		t.Fatalf("retry kept %d images", n)
	}
}
