// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The signature pass writes the name the way the signature prints it. A
// family-name-first signature fills last_name with the family name. The display
// name follows the printed order and is not rebuilt as "first last".

import (
	"context"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

const vietnameseMail = "Chào,\n\nCảm ơn.\n\nTrân trọng,\nLê Thị Mai\nGiám đốc\nAcme VN"

const vietnameseReply = `{"fields":[],"name":{"family_name":"Lê","given_names":"Mai","middle":"Thị","order":"family_first","evidence_snippet":"Lê Thị Mai","confidence":0.9}}`

type replyBrain struct{ text string }

func (b *replyBrain) Complete(context.Context, model.Request) (model.Response, error) {
	return model.Response{Text: b.text}, nil
}

type nameRow struct {
	First, Last *string
	Full        string
	Evidence    int
	Audited     int
	Events      int
	Read        bool
}

func seedNamedContact(t *testing.T, e *integration.Env, email, display, capturedBy string) ids.UUID {
	t.Helper()
	contact := seedEnrichContact(t, e, email, vietnameseMail)
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE contact SET full_name = $2, captured_by = $3 WHERE id = $1`, contact, display, capturedBy)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return contact
}

func runNamePass(t *testing.T, e *integration.Env, reply string) {
	t.Helper()
	enricher := NewCaptureEnricher(e.Pool, &replyBrain{text: reply}, slog.New(slog.DiscardHandler))
	if _, err := enricher.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS)); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func readNameRow(t *testing.T, e *integration.Env, contact ids.UUID) nameRow {
	t.Helper()
	var row nameRow
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		if err := tx.QueryRow(ctx, `SELECT first_name, last_name, full_name FROM contact WHERE id = $1`, contact).
			Scan(&row.First, &row.Last, &row.Full); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM contact_profile_field
			 WHERE contact_id = $1 AND field IN ('first_name', 'last_name')`, contact).Scan(&row.Evidence); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM audit_log
			 WHERE entity_type = 'contact' AND entity_id = $1 AND action = 'update' AND after ? 'full_name'`,
			contact).Scan(&row.Audited); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM event_outbox o JOIN audit_log a ON a.id = (o.envelope->'trace'->>'audit_log_id')::uuid
			 WHERE a.entity_id = $1 AND a.after ? 'full_name'`, contact).Scan(&row.Events); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM contact_signature_enrich_state WHERE contact_id = $1)`,
			contact).Scan(&row.Read)
	})
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func str(p *string) string {
	if p == nil {
		return "<null>"
	}
	return *p
}

func TestAFamilyNameFirstSignatureFillsLastNameWithTheFamilyName(t *testing.T) {
	e := integration.Setup(t)
	contact := seedNamedContact(t, e, "mai@acme.example", "Mai", "connector:gmail")

	runNamePass(t, e, vietnameseReply)

	row := readNameRow(t, e, contact)
	if str(row.First) != "Mai" || str(row.Last) != "Lê" {
		t.Fatalf("first %s last %s, want Mai / Lê", str(row.First), str(row.Last))
	}
	if row.Full != "Lê Thị Mai" {
		t.Errorf("display = %q, want the name as printed", row.Full)
	}
	if row.Evidence != 2 || row.Audited != 1 || row.Events != 1 {
		t.Errorf("evidence %d, name audit rows %d, events %d; want 2, 1, 1", row.Evidence, row.Audited, row.Events)
	}
}

func TestTheDisplayRefreshDoesNotReverseAPrintedName(t *testing.T) {
	e := integration.Setup(t)
	contact := seedNamedContact(t, e, "mai2@acme.example", "Mai Lê", "connector:gmail")

	runNamePass(t, e, vietnameseReply)

	var stale []ids.ContactID
	var refreshed bool
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		var err error
		if stale, err = contacts.StaleDisplayNamesTx(context.Background(), tx, 100); err != nil {
			return err
		}
		refreshed, err = contacts.RefreshDisplayNameTx(context.Background(), tx, ids.ContactID{UUID: contact})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(stale) != 0 || refreshed {
		t.Errorf("stale %v, refreshed %v; the refresh offered to rewrite a display that already holds the printed name", stale, refreshed)
	}
	if row := readNameRow(t, e, contact); row.Full != "Lê Thị Mai" {
		t.Errorf("display = %q, want Lê Thị Mai", row.Full)
	}
}

func TestAnUnclearOrderWritesNoNameAndStillMarksTheMailRead(t *testing.T) {
	e := integration.Setup(t)
	contact := seedNamedContact(t, e, "mai3@acme.example", "Mai", "connector:gmail")

	runNamePass(t, e, `{"fields":[],"name":{"family_name":"","given_names":"","middle":"","order":"unclear","evidence_snippet":"Lê Thị Mai","confidence":0.9}}`)

	row := readNameRow(t, e, contact)
	if row.First != nil || row.Last != nil || row.Full != "Mai" || row.Evidence != 0 {
		t.Errorf("first %s last %s display %q evidence %d; an unclear order must write nothing",
			str(row.First), str(row.Last), row.Full, row.Evidence)
	}
	if !row.Read {
		t.Error("the mail was not marked read, so the contact is offered to the model again every pass")
	}
}

func TestADisplayNameAPersonOwnsIsNotReplacedBySignature(t *testing.T) {
	e := integration.Setup(t)
	contact := seedNamedContact(t, e, "mai4@acme.example", "Mai", "user:colleague")

	runNamePass(t, e, vietnameseReply)

	if row := readNameRow(t, e, contact); row.Full != "Mai" || row.Audited != 0 {
		t.Errorf("display = %q with %d name audit rows; a name a person owns must stay", row.Full, row.Audited)
	}
}

func TestACorrectedNameIsNotOverwrittenBySignature(t *testing.T) {
	e := integration.Setup(t)
	contact := seedNamedContact(t, e, "mai5@acme.example", "Mai", "connector:gmail")
	for _, field := range []string{contacts.FieldFirstName, contacts.FieldLastName} {
		corrected := "Mai"
		if err := ai.NewFeedbackStore(InstallationDB(e.Pool)).Record(e.Admin(), ai.RecordInput{
			SubjectType: "contact", SubjectID: contact, ClaimKind: ai.ClaimProfileField,
			ClaimPath: ai.ProfileFieldClaimPath(field), Verdict: ai.VerdictCorrected, CorrectedValue: &corrected,
		}); err != nil {
			t.Fatalf("record the correction: %v", err)
		}
	}

	runNamePass(t, e, vietnameseReply)

	row := readNameRow(t, e, contact)
	if row.First != nil || row.Last != nil || row.Full != "Mai" {
		t.Errorf("first %s last %s display %q; a corrected field must not be rewritten", str(row.First), str(row.Last), row.Full)
	}
}
