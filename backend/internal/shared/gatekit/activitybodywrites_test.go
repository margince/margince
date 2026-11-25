// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gatekit

import "testing"

func TestAWriterOfAnActivityTextIsCountedInEverySpelling(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		statement         string
		writers, unpaired int
	}{
		{`INSERT INTO activity (id, kind, body) VALUES ($1, $2, $3)`, 1, 1},
		{`INSERT INTO activity(kind,body,body_own_end) VALUES ($1,$2,$3)`, 1, 0},
		{`insert into public."activity" as a (id, "body") values ($1, $2)`, 1, 1},
		{`UPDATE activity SET body = $2 WHERE id = $1`, 1, 1},
		{`UPDATE activity a SET subject = $2, a.body=$3, body_own_end = $4 WHERE a.id = $1`, 1, 0},
		{`UPDATE ONLY activity SET body = NULL, raw = NULL`, 1, 1},
		{"UPDATE activity SET\n  -- The text goes with the words.\n  body = NULL,\n  body_own_end = NULL\n WHERE id = $1", 1, 0},
		{`WITH t AS (SELECT id FROM activity) UPDATE activity a SET body = NULL FROM t WHERE a.id = t.id`, 1, 1},
		{`WITH gone AS (UPDATE activity SET body = NULL, body_own_end = NULL WHERE id = $1 RETURNING id) SELECT id FROM gone`, 1, 0},
		{`INSERT INTO activity (id, kind) VALUES ($1, $2) ON CONFLICT (id) DO UPDATE SET body = EXCLUDED.body`, 1, 1},
		{`UPDATE activity SET body = CASE WHEN $2::text IS NULL THEN body ELSE nullif($2, '') END, body_own_end = $3 WHERE id = $1`, 1, 0},
		{`UPDATE activity SET subject = 'the body = gone, they said', body = $2`, 1, 1},
		{`UPDATE activity SET (body, subject) = ($2, $3) WHERE id = $1`, 1, 1},
		{`UPDATE activity SET body /* the text */ = $2 WHERE id = $1`, 1, 1},
		{`WITH a AS (INSERT INTO activity (id, body, body_own_end) VALUES ($1, $2, $3) RETURNING id) INSERT INTO comms_outbound (id, body) VALUES ($4, $5) ON CONFLICT (id) DO UPDATE SET body = EXCLUDED.body`, 1, 0},
		{`WITH a AS (INSERT INTO activity (id) VALUES ($1) ON CONFLICT (id) DO UPDATE SET body = EXCLUDED.body RETURNING id) SELECT id FROM a`, 1, 1},
		{`INSERT INTO activity (id, body /* the text */, body_own_end) VALUES ($1, $2, $3)`, 1, 0},
		{`UPDATE activity a SET ("body", body_own_end) = ($2, $3), language = NULL WHERE a.id = $1`, 1, 0},
		{`UPDATE activity SET subject = $2 WHERE id = $1`, 1, 1},
		{`UPDATE activity SET subject = coalesce($2, subject), body_own_end = $3 WHERE id = $1`, 1, 0},
		{`UPDATE activity SET subject = (SELECT body FROM note WHERE body = $2) WHERE id = $1`, 1, 1},
		{`INSERT INTO activity (id, kind) VALUES ($1, $2) ON CONFLICT (id) DO UPDATE SET subject = EXCLUDED.subject`, 1, 1},
	} {
		writers, unpaired := ActivityTextWrites(tc.statement)
		if writers != tc.writers || unpaired != tc.unpaired {
			t.Errorf("%s\n got %d writers, %d without the stored end; want %d and %d",
				tc.statement, writers, unpaired, tc.writers, tc.unpaired)
		}
	}
}

func TestAStatementThatOnlyReadsTheTextIsNotAWriter(t *testing.T) {
	t.Parallel()
	for _, statement := range []string{
		`UPDATE activity SET body_completeness = $2 WHERE id = $1 AND body = $3`,
		`UPDATE activity SET language = $2 FROM other o WHERE o.body = activity.body`,
		`UPDATE activity SET language = (SELECT subject FROM note WHERE body = $2) WHERE id = $1`,
		`INSERT INTO activity (id, kind, subject) VALUES ($1, $2, $3)`,
		`UPDATE activity_participant SET body = $2`,
		`INSERT INTO activity (id, kind, body_completeness) VALUES ($1, $2, $3)`,
		`INSERT INTO comms_outbound (id, body) VALUES ($1, $2)`,
		`SELECT body FROM activity WHERE id = $1`,
		`SELECT 'UPDATE activity SET body = NULL'`,
		"SELECT 1 -- INSERT INTO activity (id, body) VALUES ($1, $2)",
	} {
		if writers, _ := ActivityTextWrites(statement); writers != 0 {
			t.Errorf("counted as %d writers of an activity text, and it writes none: %s", writers, statement)
		}
	}
}
