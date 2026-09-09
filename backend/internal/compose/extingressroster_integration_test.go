// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A group chat's roster over the WHOLE path: a unit hands the core a record
// naming who else was in the room, and the installation gains the rows saying
// so — and no reader it did not already have.
//
// It runs through the real ingress rather than the conversion alone, because the
// two things worth pinning happen at opposite ends of it: the roster crosses the
// published surface at the top, and the account resolves against
// person_channel_identity in the sink's own transaction at the bottom.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension"
)

// aGroupChatRecord is one message on the probe transport with a roster on it.
//
// A MESSAGE on the declared provider, because that pairing is what a chat is:
// the kind is what makes the parties recordable at all, and the transport is
// what an account id is meaningful against.
func aGroupChatRecord(key string, roster ...extension.Participant) extension.Record {
	return extension.Record{
		System: ingressProbeSystem,
		Key:    key,
		Activity: extension.ActivityFields{
			Kind:            extension.ActivityKindMessage,
			ChannelProvider: ingressProbeProvider,
			Subject:         "the group",
			Body:            "adding legal so we are all on the same page",
			OccurredAt:      time.Date(2026, 9, 1, 9, 30, 0, 0, time.UTC),
			Direction:       extension.DirectionInbound,
		},
		ThreadKey: "probe-chat:ws-7:group-1",
		Counterparty: extension.Counterparty{
			DisplayName: "A Sender", Direction: extension.DirectionInbound,
			ChannelIdentity: extension.ChannelIdentity{
				Provider: ingressProbeProvider, ChannelUserID: "acct-93", DisplayName: "A Sender",
			},
		},
		Participants: roster,
		Raw:          []byte(`{"id":2048,"type":"group"}`),
	}
}

// seedContactBoundToAccount gives the installation a contact already bound to
// one channel account — the state the resolution below reads.
//
// It runs through the same workspace-bound transaction the reads do (it commits
// either way), because person_channel_identity is a tenant table and a seed
// outside one lands nowhere while looking exactly like a seed that worked.
func seedContactBoundToAccount(t *testing.T, e *ingressEnv, account string) ids.UUID {
	t.Helper()
	person := ids.NewV7()
	e.readAsWorkspace(t, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`INSERT INTO person (id, full_name, owner_id, source, captured_by) VALUES ($1, 'Priya Raman', $2, 'manual', 'human:test')`,
			person, e.member); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO person_channel_identity (person_id, provider, channel_user_id, source, captured_by)
			VALUES ($1, $2, $3, 'capture', 'human:test')`, person, ingressProbeProvider, account)
		return err
	})
	return person
}

func anAttendee(account, email, name string) extension.Participant {
	return extension.Participant{
		Account: account, Email: email, Name: name,
		Role: extension.ParticipantRoleAttendee,
	}
}

// The path end to end. Before this the roster had nowhere to go: the published
// record carried no participants field at all, so a captured group chat named
// its sender and the member and nobody else in the room.
func TestAUnitsGroupRosterLandsAsParticipants(t *testing.T) {
	e := setupIngress(t)
	registerProbeTransport(t, e)
	rt := e.ingestingRuntime()

	result, err := rt.Ingest(context.Background(), extension.UserID(e.member.String()),
		aGroupChatRecord("ws-7:2048",
			anAttendee("acct-51", "", "Sam Okonkwo"),
			anAttendee("acct-77", "legal@example.test", "Priya Raman")))
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if result.Disposition != extension.DispositionAccepted {
		t.Fatalf("disposition = %q, want accepted", result.Disposition)
	}
	activityID := ids.MustParse(result.Ref.ID)

	type party struct {
		account string
		address *string
		user    *ids.UUID
		person  *ids.UUID
	}
	var landed []party
	e.readAsWorkspace(t, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT channel_user_id, address, user_id, person_id
			  FROM activity_participant
			 WHERE activity_id = $1 AND channel_user_id IS NOT NULL AND role NOT IN ('from', 'to')
			 ORDER BY channel_user_id`, activityID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p party
			if err := rows.Scan(&p.account, &p.address, &p.user, &p.person); err != nil {
				return err
			}
			landed = append(landed, p)
		}
		return rows.Err()
	})

	if len(landed) != 2 {
		t.Fatalf("the roster landed %d row(s), want the two the unit named", len(landed))
	}
	if landed[0].account != "acct-51" || landed[0].address != nil {
		t.Errorf("the account-only party landed as %+v, want its account and no address", landed[0])
	}
	if landed[1].account != "acct-77" || landed[1].address == nil || *landed[1].address != "legal@example.test" {
		t.Errorf("the party carrying both landed as %+v, want both kept", landed[1])
	}
	// THE GRANT THAT IS NOT MADE. A unit's roster is a remote system's text, so
	// neither party may reach a seat — a landed user_id here is a read handed
	// out on a stranger's say-so.
	for _, p := range landed {
		if p.user != nil {
			t.Errorf("%q bound seat %s — a unit naming somebody must not make them a reader", p.account, p.user)
		}
	}
}

// The resolution an account CAN do, taken in the sink's own transaction against
// the binding the installation already holds. This is what puts a group chat on
// the right contact's timeline and into "who on our team knows this person".
func TestARosterAccountResolvesToTheContactItIsBoundTo(t *testing.T) {
	e := setupIngress(t)
	registerProbeTransport(t, e)
	person := seedContactBoundToAccount(t, e, "acct-77")

	result, err := e.ingestingRuntime().Ingest(context.Background(), extension.UserID(e.member.String()),
		aGroupChatRecord("ws-7:2049", anAttendee("acct-77", "", "Priya Raman")))
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	var personID *ids.UUID
	e.readAsWorkspace(t, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT person_id FROM activity_participant
			  WHERE activity_id = $1 AND channel_user_id = 'acct-77'`,
			ids.MustParse(result.Ref.ID)).Scan(&personID)
	})
	if personID == nil || *personID != person {
		t.Fatalf("the roster row resolved to %v, want the contact the account is bound to (%s)", personID, person)
	}
}

// A record naming no ROSTER lands exactly as it did before the field
// existed: the two parties the message is already between (stamped by
// stampCaptureParticipants, role from/to) and nothing more. The ordinary
// two-party message is the overwhelming majority of what a chat unit sends,
// and it must not have gained an EXTRA row.
func TestATwoPartyMessageStillNamesNoRoster(t *testing.T) {
	e := setupIngress(t)
	registerProbeTransport(t, e)
	result, err := e.ingestingRuntime().Ingest(context.Background(), extension.UserID(e.member.String()),
		aGroupChatRecord("ws-7:2050"))
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if got := e.countAsWorkspace(t,
		`SELECT count(*) FROM activity_participant
		  WHERE activity_id = $1 AND channel_user_id IS NOT NULL AND role NOT IN ('from', 'to')`,
		ids.MustParse(result.Ref.ID)); got != 0 {
		t.Fatalf("a message naming nobody left %d roster row(s)", got)
	}
}
