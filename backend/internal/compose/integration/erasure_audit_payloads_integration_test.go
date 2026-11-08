// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestErasureDestroysTheAuditAndOutboxCopiesWrittenByContactCreation(t *testing.T) {
	for _, held := range []bool{false, true} {
		t.Run(map[bool]string{false: "erased", true: "legal hold"}[held], func(t *testing.T) {
			e := Setup(t)
			ctx := e.Admin()
			contact, err := e.Contacts.CreateContact(ctx, contacts.CreateContactInput{
				FullName: "Erasure Audit Subject", Source: "manual",
				Emails: []contacts.ContactEmailInput{{Email: "audit-erasure@example.test", EmailType: "work", IsPrimary: true}},
			})
			if err != nil {
				t.Fatal(err)
			}
			id := ids.UUID(contact.Id)
			if held {
				if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
					_, err := tx.Exec(ctx, `UPDATE contact SET legal_hold=true WHERE id=$1`, id)
					return err
				}); err != nil {
					t.Fatal(err)
				}
			}
			err = compose.ComposedEraser(e.Pool, nil, nil).EraseContact(ctx, id, "erase Erasure Audit Subject")
			if (held && !errors.Is(err, apperrors.ErrConflict)) || (!held && err != nil) {
				t.Fatalf("erasure under legal hold=%v: %v", held, err)
			}
			if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
				var namedAudit, namedEvent, metadata bool
				err := tx.QueryRow(ctx, `SELECT
				 EXISTS(SELECT 1 FROM audit_log WHERE entity_type='contact' AND entity_id=@id
				        AND concat(before::text, after::text, evidence::text) LIKE '%Erasure Audit Subject%'),
				 EXISTS(SELECT 1 FROM event_outbox WHERE envelope->'entity'->>'id'=@id::text
				        AND envelope::text LIKE '%Erasure Audit Subject%'),
				 EXISTS(SELECT 1 FROM audit_log WHERE entity_type='contact' AND entity_id=@id AND action='create')`,
					pgx.StrictNamedArgs{"id": id}).Scan(&namedAudit, &namedEvent, &metadata)
				if err != nil {
					return err
				}
				if namedAudit != held || namedEvent != held || !metadata {
					t.Errorf("held=%v: named audit=%v named event=%v creation metadata=%v", held, namedAudit, namedEvent, metadata)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
