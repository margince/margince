// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gatekit_test

// The reader, from the other end. A census inherits exactly what this can see,
// so a statement shape it stops reading is a clean tree reported over the very
// thing the census exists to find.

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

func TestTheStatementReaderSeesEachShapeAStatementIsWrittenIn(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "a backticked literal",
			source: "package p\nvar q = `SELECT id FROM person`\n",
			want:   "SELECT id FROM person",
		},
		{
			// The defect this reader exists for: source text keeps the escape,
			// so a pattern asking for whitespace matches nothing.
			name:   "an interpreted literal, whose whitespace is an escape",
			source: "package p\nvar q = \"SELECT id\\nFROM person\"",
			want:   "SELECT id\nFROM person",
		},
		{
			name:   "a chain of literals is one statement",
			source: "package p\nvar q = \"SELECT id \" + \"FROM person\"\n",
			want:   "SELECT id FROM person",
		},
		{
			// The chain's readable half folds, and the half it cannot read is
			// still WALKED — a statement hidden inside it would otherwise leave
			// with it.
			name:   "a literal inside a chain's unreadable half is still read",
			source: "package p\nimport \"fmt\"\nvar q = \"WITH x AS (\" + fmt.Sprintf(\"SELECT id FROM person\") + \")\"\n",
			want:   "SELECT id FROM person",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			statements := gatekit.SQLStatementsIn(t, "probe.go", tc.source)
			found := false
			for _, statement := range statements {
				if strings.Contains(statement, tc.want) {
					found = true
				}
			}
			if !found {
				t.Errorf("the reader did not produce %q; it read %q", tc.want, statements)
			}
		})
	}
}
