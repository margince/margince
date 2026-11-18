// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// TestARefusalRecordedAfterSelectionKeepsItsRecord acts on a contact and a lead
// that hold a refusal inside its floor, as one recorded after the pass selected
// them. Each retention action refuses, and the records keep their names.
func TestARefusalRecordedAfterSelectionKeepsItsRecord(t *testing.T) {
	t.Parallel()
	ctx := principal.WithActor(context.Background(),
		principal.Principal{Type: principal.PrincipalSystem, ID: "system:retention"})
	tx := subjectColumnsTx(ctx, t)
	contact := keptStopSubject(ctx, t, tx)
	lead := ids.NewV7()
	mustExec(ctx, t, tx, `INSERT INTO lead (id, full_name, email, source, captured_by)
		VALUES ($1, 'Kept Lead', 'kept-lead@anon.test', 'test', 'human:x')`, lead)
	mustExec(ctx, t, tx, `
		INSERT INTO communication_suppression (contact_id, lead_id, kind, source, captured_by, decided_by_level, kept_until)
		VALUES ($1, NULL, 'marketing_objection', 'operator_ui', 'human:x', 'user', now() + interval '1 year'),
		       (NULL, $2, 'marketing_objection', 'operator_ui', 'human:x', 'user', now() + interval '1 year')`,
		contact, lead)

	svc := &RetentionService{eraser: &Eraser{payloads: noPayloads{t: t}}}
	for action, act := range map[string]func() error{
		"contact/anonymize": func() error { return svc.anonymizeContact(ctx, tx, contact.UUID) },
		"lead/anonymize":    func() error { return svc.anonymizeLead(ctx, tx, lead) },
		"lead/archive":      func() error { return svc.archiveLead(ctx, tx, lead) },
	} {
		if err := act(); !errors.Is(err, errKeptByFloor) {
			t.Errorf("%s on a record with a kept refusal answered %v, want it kept by the floor", action, err)
		}
	}

	var contactName, leadName string
	var leadArchived bool
	if err := tx.QueryRow(ctx, `SELECT full_name FROM contact WHERE id = $1`, contact).Scan(&contactName); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT full_name, archived_at IS NOT NULL FROM lead WHERE id = $1`, lead).
		Scan(&leadName, &leadArchived); err != nil {
		t.Fatal(err)
	}
	if contactName != "Ines Subject" || leadName != "Kept Lead" || leadArchived {
		t.Errorf("retention changed records the floor keeps: contact %q, lead %q archived %v", contactName, leadName, leadArchived)
	}
}
