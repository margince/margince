// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Imported history does not rank as first-party testimony.
//
// The HubSpot migration ran through the public API as an administrator, so
// every one of its 34,521 activities carries a `human:` captured_by —
// truthfully, because an administrator did run the import. The retrieval ladder
// reads captured_by, so without this those rows scored T0: another company's
// CRM history ranked above what colleagues here actually wrote, inside the walk
// that answers questions about an account.
//
// THE RESERVED NAMESPACE IS WHAT SEPARATES THEM, and not `source_system IS NOT
// NULL`, which an earlier draft of this change used and which is wrong. That
// column is not exclusive to imports: activities/outboundmessage.go stamps
// `email` on a message this installation SENT, and activities/requesttask.go
// stamps its own reminder identity. Both are first-party writes, and both would
// have been demoted to captured-external by the looser predicate — so the fix
// for a mis-ranked timeline would itself have mis-ranked two more kinds of row.
//
// provenance.ReservedSourceSystemPrefix (`mirror:`) carries no such ambiguity.
// It exists precisely to name rows only an import may write, and every
// client-facing create path refuses it through provenance.Refuse. That is what
// the ladder reads.
//
// THE AUTHOR COLUMNS WOULD BE THE WRONG TEST, which is why the cases below
// cover a row that has none. The attribution repair skips an activity whose
// author it cannot resolve, so an imported row can carry no author at all — and
// testing for attribution rather than for provenance would hand exactly those
// rows full human-statement trust.
//
// DETERMINISM. Both notes take ONE timestamp, passed as a parameter rather than
// two separate `now()` calls, and the imported note is seeded FIRST so its
// lower ids.NewV7() wins sortAndTrim's id-ascending tie-break. So if the trust
// term were removed, the imported note would sort first and every case here
// would fail — the assertion can only pass on trust.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

func TestImportedHistoryRanksBelowWhatColleaguesWroteHere(t *testing.T) {
	for _, c := range []struct {
		name    string
		columns string
		values  string
		// seatArg is passed as $5 only by the case whose VALUES names it. A
		// literal-only case takes no fifth parameter, and handing pgx one it
		// cannot place is a "mismatched param and argument count" that looks
		// like a schema problem and is not.
		//
		// $1..$4 are fixed for every case — id, captured_by, occurred_at,
		// source_system — so the author column, when there is one, is always
		// $5. Written out here rather than derived, because a fixture that
		// rewrites its own placeholders is one column away from being wrong
		// silently.
		seatArg bool
	}{
		{
			name:    "the author holds a seat here",
			columns: ", source_author_id",
			values:  ", $5",
			seatArg: true,
		},
		{
			name:    "the author left before we had seats",
			columns: ", source_author_name",
			values:  ", 'Mutaz Suleiman'",
		},
		{
			// The case the author-column predicates missed. The repair reached
			// this row, could not resolve who wrote it, and skipped it — so it
			// carries no author at all and is still somebody else's history.
			name: "the author could not be resolved, so the repair skipped it",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := SetupSearch(t)

			// One contact, and a meeting anchored on them so the walk has
			// somewhere to start. The meeting itself is not what is ranked.
			contact := e.SeedID(t, `INSERT INTO contact (id, full_name, source, captured_by)
				VALUES ($1, 'Mara Kessler', 'manual', 'human:seed')`)
			anchor := seedMeeting(t, e, "Quarterly review")
			linkMeeting(t, e, anchor, "contact", "contact_id", contact)

			// BOTH notes name the same human writer. That is the whole
			// difficulty: captured_by cannot tell an imported row from one
			// somebody here typed.
			writer := "human:" + e.Rep1.String()

			// ONE timestamp for both, so the recency term is genuinely equal
			// rather than nearly so. Two `now()` calls in two autocommit
			// statements are not the same instant, and the newer row wins on
			// recency before trust is ever consulted.
			var when time.Time
			if err := e.Owner.QueryRow(context.Background(),
				`SELECT now() - interval '3 days'`).Scan(&when); err != nil {
				t.Fatalf("reading one timestamp for both notes: %v", err)
			}

			// THE IMPORTED NOTE FIRST, so its id sorts lower and wins the
			// id-ascending tie-break in sortAndTrim. Without the trust term it
			// would therefore rank ABOVE ours, and this test would fail.
			//
			// Its source_system is spelled from the kernel constant, so this
			// fixture cannot drift from the namespace the walk tests for.
			args := []any{writer, when, provenance.ReservedSourceSystemPrefix + "hubspot"}
			if c.seatArg {
				args = append(args, e.Rep1)
			}
			theirs := e.SeedID(t, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by,
			                                             source_system, source_id`+c.columns+`)
				VALUES ($1, 'note', 'Theirs: agreed the renewal terms', $3, 'hubspot_import', $2,
				        $4, 'imported-note-1'`+c.values+`)`, args...)
			e.SeedID(t, `INSERT INTO activity_link (id, activity_id, entity_type, contact_id)
				VALUES ($1, $2, 'contact', $3)`, theirs, contact)

			ours := e.SeedID(t, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
				VALUES ($1, 'note', 'Ours: agreed the renewal terms', $3, 'manual', $2)`, writer, when)
			e.SeedID(t, `INSERT INTO activity_link (id, activity_id, entity_type, contact_id)
				VALUES ($1, $2, 'contact', $3)`, ours, contact)

			assembled, err := prepFor(e.Admin(), t, e, anchor)
			if err != nil {
				t.Fatalf("walking the context: %v", err)
			}

			touches := summariesIn(assembled, "recent_touches")
			joined := strings.Join(touches, " | ")
			oursAt, theirsAt := -1, -1
			for i, s := range touches {
				switch {
				case strings.Contains(s, "Ours:"):
					oursAt = i
				case strings.Contains(s, "Theirs:"):
					theirsAt = i
				}
			}
			if oursAt < 0 || theirsAt < 0 {
				t.Fatalf("recent_touches = %q, want both notes — without both there is nothing to order", joined)
			}
			if oursAt > theirsAt {
				t.Errorf("recent_touches = %q: the imported note ranked above the one written here. "+
					"Same timestamp, same captured_by, and the imported note holds the LOWER id — "+
					"so it wins every tie-break there is and only the trust term can put ours first", joined)
			}
		})
	}
}

// TestAMessageThisInstallationSentStillRanksAsFirstParty is the regression for
// the predicate this change corrects, driven end to end rather than in a unit.
//
// Under `source_system IS NOT NULL` a natively sent email was indistinguishable
// from imported history: outboundmessage.go stamps `email` on it, so the walk
// demoted a message a colleague here sent to the same trust as another
// company's CRM export. The namespace predicate keeps it at T0.
//
// The shape mirrors the test above and inverts its expectation: the sent
// message is seeded FIRST, so it holds the lower id and wins the tie-break —
// meaning this passes whether or not trust applies. So the note it is ranked
// against is seeded with an agent's captured_by, which trust DOES demote. If
// the sent message were wrongly treated as imported, both would sit at T2, the
// id tie-break would decide, and the order would be unchanged — which is why
// the agent note carries a newer timestamp: at equal trust recency puts it
// first, and only the sent message keeping T0 can hold it back.
func TestAMessageThisInstallationSentStillRanksAsFirstParty(t *testing.T) {
	e := SetupSearch(t)

	contact := e.SeedID(t, `INSERT INTO contact (id, full_name, source, captured_by)
		VALUES ($1, 'Mara Kessler', 'manual', 'human:seed')`)
	anchor := seedMeeting(t, e, "Quarterly review")
	linkMeeting(t, e, anchor, "contact", "contact_id", contact)

	var older, newer time.Time
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT now() - interval '3 days', now() - interval '2 days'`).Scan(&older, &newer); err != nil {
		t.Fatalf("reading the two timestamps: %v", err)
	}

	// The message this installation SENT: source_system `email`, which the
	// rejected predicate would have read as imported.
	sent := e.SeedID(t, `INSERT INTO activity (id, kind, subject, occurred_at, direction, source, captured_by,
	                                           source_system, source_id)
		VALUES ($1, 'email', 'Ours: the renewal quote', $3, 'outbound', 'manual', $2, 'email', 'sent-1')`,
		"human:"+e.Rep1.String(), older)
	e.SeedID(t, `INSERT INTO activity_link (id, activity_id, entity_type, contact_id)
		VALUES ($1, $2, 'contact', $3)`, sent, contact)

	// An agent note, newer, which trust demotes to T1.
	agentNote := e.SeedID(t, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'note', 'Theirs: an agent inferred this', $3, 'manual', $2)`,
		"agent:"+e.Rep1.String(), newer)
	e.SeedID(t, `INSERT INTO activity_link (id, activity_id, entity_type, contact_id)
		VALUES ($1, $2, 'contact', $3)`, agentNote, contact)

	assembled, err := prepFor(e.Admin(), t, e, anchor)
	if err != nil {
		t.Fatalf("walking the context: %v", err)
	}

	touches := summariesIn(assembled, "recent_touches")
	joined := strings.Join(touches, " | ")
	sentAt, agentAt := -1, -1
	for i, s := range touches {
		switch {
		case strings.Contains(s, "Ours:"):
			sentAt = i
		case strings.Contains(s, "Theirs:"):
			agentAt = i
		}
	}
	if sentAt < 0 || agentAt < 0 {
		t.Fatalf("recent_touches = %q, want both rows", joined)
	}
	if sentAt > agentAt {
		t.Errorf("recent_touches = %q: a message this installation sent ranked below a newer agent "+
			"note. It carries source_system 'email', so a predicate testing only for a non-null "+
			"source_system reads it as imported and drops it to captured-external — which is the "+
			"bug this predicate exists to avoid", joined)
	}
}
