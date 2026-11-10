// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind falsification H2

package gates

import (
	"strings"
	"testing"
)

// A downgrade guard may inspect rows and raise; calls and procedural writes still need review.
func readOnlyMigrationGuard(span string) bool {
	if !strings.HasPrefix(span, "$") {
		return false
	}
	delimiterEnd := strings.Index(span[1:], "$") + 2
	if delimiterEnd < 2 || len(span) < 2*delimiterEnd || !strings.HasSuffix(span, span[:delimiterEnd]) {
		return false
	}
	tokens, err := rewriteTokens(span[delimiterEnd : len(span)-delimiterEnd])
	if err != nil || len(tokens) < 15 {
		return false
	}
	words := make([]string, len(tokens))
	for i, token := range tokens {
		words[i] = token.text
	}
	if strings.Join(words[:5], " ") != "begin if exists ( select" {
		return false
	}
	end := 5
	for end < len(words) && words[end] != ")" {
		switch words[end] {
		case "(", ";", "into", "insert", "update", "delete", "merge", "execute", "call", "perform", "copy", "truncate", "do":
			return false
		}
		end++
	}
	if end == len(words) {
		return false
	}
	tail := strings.TrimSuffix(strings.Join(words[end+1:], " "), " ;")
	return tail == "then raise exception using errcode ; end if ; end"
}

func TestDowngradeReadGuardsCannotHideCallsOrWrites(t *testing.T) {
	t.Parallel()
	for _, scenario := range []struct {
		query, tail string
		safe        bool
	}{
		{"SELECT 1 FROM sample s JOIN member m ON m.id=s.member_id WHERE m.removed_at IS NOT NULL", "RAISE EXCEPTION 'Cannot reverse safely' USING ERRCODE='23514'", true},
		{"SELECT change_rows() FROM sample", "RAISE EXCEPTION 'No' USING ERRCODE='23514'", false},
		{"SELECT 1 INTO scratch FROM sample", "RAISE EXCEPTION 'No' USING ERRCODE='23514'", false},
		{"SELECT 1 FROM sample", "UPDATE sample SET active=false", false},
		{"SELECT 1 FROM sample", "RAISE NOTICE 'No'", false},
		{"SELECT 1 FROM sample", "RAISE EXCEPTION 'No' USING ERRCODE='23514'; END IF; DELETE FROM sample; IF true THEN RAISE EXCEPTION 'No' USING ERRCODE='23514'", false},
	} {
		sql := "DO $$ BEGIN IF EXISTS (" + scenario.query + ") THEN " + scenario.tail + "; END IF; END $$;"
		rewrites, err := migrationRewrites(sql)
		if err != nil {
			t.Fatal(err)
		}
		if (len(rewrites) == 0) != scenario.safe {
			t.Errorf("query=%s tail=%s: rewrites=%v", scenario.query, scenario.tail, rewrites)
		}
	}
}
