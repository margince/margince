// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestBothPrivacyActsClearProjectedParticipantDomains(t *testing.T) {
	acts := map[string]func(context.Context, pgx.Tx, ids.ContactID, []string) error{
		"erasure": func(ctx context.Context, tx pgx.Tx, subject ids.ContactID, emails []string) error {
			return scrubSubjectFromParticipants(ctx, tx, subject, emails, nil)
		},
		"retention": func(ctx context.Context, tx pgx.Tx, subject ids.ContactID, emails []string) error {
			return scrubContactGraphTraces(ctx, tx, subject.UUID, emails, nil, "Subject", nil)
		},
	}
	for name, act := range acts {
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()
			tx := subjectColumnsTx(ctx, t)
			workspace, user, activity := ids.NewV7(), ids.NewV7(), ids.NewV7()
			subject := ids.New[ids.ContactKind]()
			const address = "subject@sub.contact.test"
			mustExec(ctx, t, tx, `INSERT INTO workspace(id) VALUES (@id)`, pgx.NamedArgs{"id": workspace})
			mustExec(ctx, t, tx, `INSERT INTO app_user(id,email,display_name) VALUES (@id,@email,'Colleague')`, pgx.NamedArgs{"id": user, "email": user.String() + "@own.test"})
			mustExec(ctx, t, tx, `INSERT INTO contact(id,full_name,source,captured_by) VALUES (@id,'Subject','manual','test:privacy')`, pgx.NamedArgs{"id": subject})
			mustExec(ctx, t, tx, `INSERT INTO activity(id,kind,source,captured_by) VALUES (@id,'email','manual','test:privacy')`, pgx.NamedArgs{"id": activity})
			folded := owndomain.StoredOfAddress(address)
			mustExec(ctx, t, tx, `INSERT INTO activity_participant(activity_id,user_id,contact_id,role,address,address_domain_folded)
    VALUES (@activity,@user,@subject,'from',@address,@folded)`, pgx.NamedArgs{
				"activity": activity, "user": user, "subject": subject, "address": address, "folded": folded,
			})
			if err := act(ctx, tx, subject, []string{address}); err != nil {
				t.Fatal(err)
			}
			var cleared bool
			if err := tx.QueryRow(ctx, `SELECT user_id IS NOT NULL AND contact_id IS NULL AND address IS NULL
    AND address_domain_folded IS NULL FROM activity_participant WHERE activity_id=@activity`, pgx.NamedArgs{"activity": activity}).Scan(&cleared); err != nil {
				t.Fatal(err)
			}
			if !cleared {
				t.Fatal("privacy act retained derived participant identity")
			}
		})
	}
}
