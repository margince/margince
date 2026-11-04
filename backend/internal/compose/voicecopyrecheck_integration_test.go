// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A voice sample follows its mail after the press. Each change runs through
// its production writer. The consumer reads the events that writer put in the
// outbox, so a writer that stopped emitting would leave the sample here.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// deliverOutboxFor hands every event staged for the activity to the consumer.
func deliverOutboxFor(t *testing.T, l learnedMail, activityID ids.UUID) int {
	t.Helper()
	var envelopes []events.Envelope
	if err := database.WithWorkspaceTx(l.env.e.Admin(), l.env.e.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `SELECT envelope FROM event_outbox
			WHERE envelope->'entity'->>'id' = $1 ORDER BY seq`, activityID.String())
		if err != nil {
			return err
		}
		var raw []byte
		_, err = pgx.ForEachRow(rows, []any{&raw}, func() error {
			var env events.Envelope
			if err := json.Unmarshal(raw, &env); err != nil {
				return err
			}
			envelopes = append(envelopes, env)
			return nil
		})
		return err
	}); err != nil {
		t.Fatalf("reading the outbox: %v", err)
	}
	gen := NewVoiceCopyRecheckGen(l.env.e.Pool)
	for _, env := range envelopes {
		if err := gen.HandleEvent(context.Background(), env); err != nil {
			t.Fatalf("rechecking after %s: %v", env.Type, err)
		}
	}
	return len(envelopes)
}

func TestAVoiceSampleFollowsItsMailOnceItStopsBeingLearnable(t *testing.T) {
	for name, change := range map[string]func(t *testing.T, l learnedMail){
		"limited to its participants": func(t *testing.T, l learnedMail) {
			decideThread(t, l.env.e, l.o.owner, threadOf(t, l.o, l.mail), false)
		},
		"marked confidential": func(t *testing.T, l learnedMail) {
			marked := "Vertraulich: Angebot für Pat"
			if _, err := l.env.e.Activities.UpdateActivity(l.env.e.As(l.o.owner, nil, sentMailOwnerPerms),
				ids.ActivityID{UUID: l.mail}, activities.UpdateActivityInput{Subject: &marked}); err != nil {
				t.Fatalf("marking the mail confidential: %v", err)
			}
		},
		"archived": func(t *testing.T, l learnedMail) {
			archiver := sentMailOwnerPerms
			archiver.Objects = map[string]principal.ObjectGrant{"activity": {Read: true, Update: true, Delete: true}}
			if _, err := l.env.e.Activities.ArchiveActivity(l.env.e.As(l.o.owner, nil, archiver),
				ids.ActivityID{UUID: l.mail}, nil); err != nil {
				t.Fatalf("archiving the mail: %v", err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			l := voiceLearnedFromMail(t)
			change(t, l)
			if deliverOutboxFor(t, l, l.mail) == 0 {
				t.Fatal("the change staged no event for the mail, so nothing could recheck it")
			}
			l.assertSampleErased(t)
		})
	}
}

// A mail still learnable keeps its sample through the same recheck.
func TestAnUnrelatedChangeKeepsTheVoiceSample(t *testing.T) {
	l := voiceLearnedFromMail(t)
	deliverOutboxFor(t, l, l.mail)
	if n := l.o.count(t, `SELECT count(*) FROM voice_corpus_source WHERE source_ref = $1
		AND content IS NOT NULL AND archived_at IS NULL`, "sent-mail:"+l.mail.String()); n != 1 {
		t.Error("the recheck emptied the sample of a mail that is still learnable")
	}
}

// A mailbox two seats hold is shared. Its sent mail carries the connecting
// seat as sender, but it is not that seat's own writing to learn from.
func TestASharedMailboxsSentMailTeachesNoVoice(t *testing.T) {
	o := setupOwed(t)
	o.e.WsExec(t, `UPDATE capture_connection SET account_label = $2 WHERE user_id = $1`, o.owner, "sales@ws.example")
	o.seat = "sales@ws.example"
	seatAddress(t, o.e, o.e.Rep2)
	o.e.WsExec(t, `UPDATE capture_connection SET account_label = $2 WHERE user_id = $1`, o.e.Rep2, "sales@ws.example")
	v, profileID := newSentMailVoice(t, o)
	shared := o.weWrite(t, owedMail{to: "pat@customer.example", subject: "Angebot", at: clocktest.Now(t).Add(-time.Hour),
		body: "Hallo Pat, anbei das Angebot vom Vertriebsteam."})
	if o.count(t, `SELECT count(*) FROM activity a JOIN activity_participant p ON p.activity_id = a.id
		WHERE a.id = $1 AND a.counterparty_outbound_attested AND a.audience = 'workspace'
		  AND p.role = 'from' AND p.user_id = $2`, shared, o.owner) != 1 {
		t.Fatal("the fixture's shared mail does not name the owner as its attested sender, so the shared rule is not what keeps it out")
	}
	learned := v.press(t, profileID)
	if learned.Read != 0 || learned.Added != 0 {
		t.Errorf("learned %+v from a mailbox two seats hold, want nothing", learned)
	}
	if got := stored(t, o, profileID); len(got) != 0 {
		t.Errorf("stored %s from the shared mailbox", strings.Join(mapKeys(got), ", "))
	}
}

func mapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
