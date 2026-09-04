// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// A captured message is filed under the contacts who were ON it, not only the
// one the ladder judged.
//
// Mail reaches its records through the counterparty: one address, one ensure,
// one link. A contact merely cc'd was stamped as a participant and filed
// nowhere, so the message never reached their timeline — and, because consent
// reads an INBOUND activity linked to the person as the Art 6(1)(f) qualifying
// event, the send gate refused mail to a contact whose own replies were sitting
// in the workspace unlinked.
//
// The audience half is what these tests are really guarding. A link says where
// a message is FILED; the audience says who may READ it, and filing a contact's
// own mail under them must not move the second. The birth ladder decides the
// audience before any of this runs, so what is proven here is that the filing
// leaves that decision alone.

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// peopleFiledUnder answers the people one captured activity is filed under.
func peopleFiledUnder(t *testing.T, e *integration.SearchEnv, sourceID string) []ids.UUID {
	t.Helper()
	var out []ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `
			SELECT l.person_id FROM activity_link l
			  JOIN activity a ON a.id = l.activity_id
			 WHERE a.source_id = $1 AND l.person_id IS NOT NULL
			 ORDER BY l.person_id`, sourceID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id ids.UUID
			if err := rows.Scan(&id); err != nil {
				return err
			}
			out = append(out, id)
		}
		return rows.Err()
	}); err != nil {
		t.Fatalf("reading what the message was filed under: %v", err)
	}
	return out
}

// filedUnder reports whether the filing names this person.
func filedUnder(filed []ids.UUID, want ids.UUID) bool {
	for _, p := range filed {
		if p == want {
			return true
		}
	}
	return false
}

// A contact on the Cc line is filed under, the way the To party always was.
//
// This is the defect in its original shape: an inbound message whose Cc is an
// existing contact reached that contact's record not at all, so nothing that
// walks activity_link — their timeline, and the consent gate's qualifying
// event — could see a message they were plainly on.
func TestACcdContactIsFiledUnderTheirOwnRecord(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync

	// Through the real people store: the resolution under test reads
	// person_email, and a row a test invents is not the row production writes.
	store := people.NewStore(e.DB())
	cc, err := store.EnsurePersonByEmail(personCreator(e), "Cc Contact", "cc@partner.example", "manual")
	if err != nil {
		t.Fatalf("seeding the cc'd contact: %v", err)
	}

	sync(t, emailCC("sender@partner.example", "Sender", captureOwner, "cc@partner.example", "cc1@partner.example"))

	filed := peopleFiledUnder(t, e, "cc1@partner.example")
	if !filedUnder(filed, cc) {
		t.Fatalf("message filed under %v, want the cc'd contact %s among them — "+
			"a contact copied on a message reaches it through no link at all, "+
			"so their timeline misses it and the consent gate finds no qualifying event",
			filed, cc)
	}
}

// A SUPPRESSED sender is filed under nobody, whoever is copied on it.
//
// The ladder judged this sender infrastructure. Filing the message anyway would
// put a bulk sender's mail on a contact's timeline and hand it the Art 6(1)(f)
// qualifying event that judgement just refused.
func TestASuppressedMessageIsFiledUnderNobodyItCopies(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync

	store := people.NewStore(e.DB())
	if _, err := store.EnsurePersonByEmail(personCreator(e), "Cc Contact", "cc@partner.example", "manual"); err != nil {
		t.Fatalf("seeding the cc'd contact: %v", err)
	}

	// DocuSign is exact infrastructure, so the ladder suppresses it.
	sync(t, emailCC("dse@eu.docusign.net", "DocuSign EU", captureOwner, "cc@partner.example", "sup1@docusign.net"))

	if filed := peopleFiledUnder(t, e, "sup1@docusign.net"); len(filed) != 0 {
		t.Fatalf("a suppressed message is filed under %v, want nobody — the ladder judged this sender, "+
			"and filing it puts a bulk sender's mail on a contact's timeline and hands it a qualifying event "+
			"the judgement just refused", filed)
	}
}

// The filing does not widen who may READ the message.
//
// This is the invariant the whole change turns on. A message the sender marked
// confidential is born held to its participants, and a cc'd contact gaining a
// link says nothing about that marking — the link records where the message is
// FILED, and the audience records who may read it.
//
// The sender's marker is used rather than a suppression because the two are
// independent: a suppressed message is filed under nobody, so it writes no
// links and would pass this no matter how the audience were wired. Here the
// links ARE written, so the assertion is about a message that was actually
// filed.
func TestFilingAConfidentialMessageDoesNotWidenWhoMayReadIt(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync

	store := people.NewStore(e.DB())
	cc, err := store.EnsurePersonByEmail(personCreator(e), "Cc Contact", "cc@partner.example", "manual")
	if err != nil {
		t.Fatalf("seeding the cc'd contact: %v", err)
	}

	sync(t, markedEmailCC("sender@partner.example", captureOwner, "cc@partner.example",
		"conf1@partner.example", "[Vertraulich] Aufhebungsvertrag"))

	// The filing happened: without it this test would prove nothing about the
	// audience, because a message with no links keeps its birth audience anyway.
	filed := peopleFiledUnder(t, e, "conf1@partner.example")
	if !filedUnder(filed, cc) {
		t.Fatalf("message filed under %v, want the cc'd contact %s among them — "+
			"the audience assertion below only means something once the links exist", filed, cc)
	}
	audience, reason := audienceOf(t, e, activityIDOf(t, e, "conf1@partner.example"))
	if audience != "participants" || reason != "explicitly_confidential" {
		t.Errorf("a message its sender marked confidential is audience=%q reason=%q after being filed, "+
			"want participants/explicitly_confidential — filing says where a message lives, never who may "+
			"read it, and widening here publishes a thread the sender asked us not to share",
			audience, reason)
	}
}

// markedEmailCC is the confidential shape with a third party copied — the
// NDA thread this whole change came from: colleagues writing, an outside
// contact on Cc, and a marker in the subject.
func markedEmailCC(from, to, cc, msgID, subject string) []byte {
	return []byte(strings.Join([]string{
		"From: " + from,
		"To: " + to,
		"Cc: " + cc,
		"Subject: " + subject,
		"Date: Wed, 04 Jun 2026 08:00:00 +0000",
		"Message-ID: <" + msgID + ">",
		"Content-Type: text/plain",
		"",
		"hello",
		"",
	}, "\r\n"))
}
