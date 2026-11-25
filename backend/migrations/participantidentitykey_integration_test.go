// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// participantIdentity is the four columns the participant key digests.
type participantIdentity struct {
	name                            string
	user, contact, address, account *string
}

func seedEvidenceActivity(t *testing.T, conn *pgx.Conn, subject string) string {
	t.Helper()
	return scalar(t, conn, `INSERT INTO activity (kind, subject, occurred_at, source, captured_by)
		VALUES ('call', $1, '2026-01-01T12:00:00Z', 'manual', 'human:x') RETURNING id`, subject)
}

func insertParticipantIdentity(conn *pgx.Conn, activity string, identity participantIdentity) error {
	_, err := conn.Exec(context.Background(), `INSERT INTO activity_participant
		(activity_id, role, user_id, contact_id, address, channel_user_id) VALUES ($1, 'to', $2, $3, $4, $5)`,
		activity, identity.user, identity.contact, identity.address, identity.account)
	return err
}

// The digest input joins four columns, so the cases are pairs a careless join folds into one.
// They are a null beside an empty text, and a text holding the separator.
func TestAParticipantKeyTellsEveryIdentityApart(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	activity := seedEvidenceActivity(t, conn, "Identity key")
	shared := seedAppUserForConnection(t, conn, "shared-id@identitykey.test")
	scalar(t, conn, `INSERT INTO contact (id, full_name, source, captured_by)
		VALUES ($1, 'Shares the id of a user', 'manual', 'human:x') RETURNING id`, shared)
	text := func(value string) *string { return &value }
	identities := []participantIdentity{
		{name: "a user", user: &shared},
		{name: "a contact with the id of that user", contact: &shared},
		{name: "both", user: &shared, contact: &shared},
		{name: "an address", address: text("a")},
		{name: "an account", account: text("a")},
		{name: "an empty address", address: text("")},
		{name: "an empty account", account: text("")},
		{name: "an address and an empty account", address: text("a"), account: text("")},
		{name: "an empty address and an account", address: text(""), account: text("a")},
		{name: "an address and an account", address: text("a"), account: text("b")},
		{name: "an address holding the separator", address: text("a|1:b")},
		{name: "an address that reads as a length and an account", address: text("1:a|1:b")},
		{name: "an account holding the separator", account: text("|1:a")},
		{name: "an address ending in the separator", address: text("a|")},
	}
	for _, identity := range identities {
		if err := insertParticipantIdentity(conn, activity, identity); err != nil {
			t.Fatalf("%s was refused beside the identities before it: %v", identity.name, err)
		}
	}
	for _, identity := range identities {
		err := insertParticipantIdentity(conn, activity, identity)
		var refused *pgconn.PgError
		if !errors.As(err, &refused) || refused.Code != "23505" || refused.ConstraintName != "uq_activity_participant_identity" {
			t.Errorf("%s was stored a second time on one activity and role: %v", identity.name, err)
		}
	}
	other := seedEvidenceActivity(t, conn, "Another activity")
	if err := insertParticipantIdentity(conn, other, identities[0]); err != nil {
		t.Errorf("the key refused one identity on a second activity: %v", err)
	}
}
