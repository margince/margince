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
// `source_system` is what separates them. It names the system a row came from,
// a native write never sets it, and the ladder reads it FIRST.
//
// THE AUTHOR COLUMNS WOULD BE THE WRONG TEST, which is why the cases below
// cover a row that has none. The repair skips an activity whose author it
// cannot resolve, so an imported row can carry no author at all — and testing
// for attribution rather than for provenance would hand exactly those rows full
// human-statement trust.
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
)

func TestImportedHistoryRanksBelowWhatColleaguesWroteHere(t *testing.T) {
	for _, c := range []struct {
		name    string
		columns string
		values  string
		// seatArg is passed as $4 only by the case whose VALUES names it. A
		// literal-only case takes no fourth parameter, and handing pgx one it
		// cannot place is a "mismatched param and argument count" that looks
		// like a schema problem and is not.
		seatArg bool
	}{
		{
			name:    "the author holds a seat here",
			columns: ", source_author_id",
			values:  ", $4",
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
			args := []any{writer, when}
			if c.seatArg {
				args = append(args, e.Rep1)
			}
			theirs := e.SeedID(t, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by,
			                                             source_system, source_id`+c.columns+`)
				VALUES ($1, 'note', 'Theirs: agreed the renewal terms', $3, 'hubspot_import', $2,
				        'hubspot', 'imported-note-1'`+c.values+`)`, args...)
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
