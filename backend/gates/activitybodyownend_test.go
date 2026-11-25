// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

package gates

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// bodyWriterFloor is the number of body-writing clauses the tree held when this
// gate was written. A reader that stops seeing statements reports fewer.
const bodyWriterFloor = 14

// Search cuts a mail body at activity.body_own_end, which Go computes from the
// text. A writer that changes the body and leaves the end cuts the new text at
// the old place. The census reads the Go statements outside test files.
func TestEveryWriterOfAnActivityBodySetsWhereTheSendersTextEnds(t *testing.T) {
	t.Parallel()
	writers := 0
	for _, path := range handWrittenGoSources(t) {
		where := filepath.ToSlash(path)
		if strings.HasSuffix(where, "_test.go") {
			continue
		}
		source, err := gatekit.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", where, err)
		}
		for _, statement := range gatekit.SQLStatementsIn(t, where, string(source)) {
			found, unpaired := gatekit.ActivityBodyWrites(statement)
			writers += found
			if unpaired > 0 {
				t.Errorf("%s writes activity.body and not body_own_end, so search cuts the new text where the old one ended. "+
					"Set it from mailquote.OwnEnd, or to NULL beside a cleared body:\n  %s",
					where, gatekit.FirstLineOf(strings.TrimSpace(statement)))
			}
		}
	}
	if writers < bodyWriterFloor {
		t.Fatalf("read %d writers of activity.body, under the %d the tree held: the census no longer sees them",
			writers, bodyWriterFloor)
	}
}
