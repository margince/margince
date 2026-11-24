// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// A rep reads the guard's "no connection" reason in their own language, from
// the web app's copy. The guard never reads a deal, so that sentence must not
// name one: a rep would go looking for a deal that would change nothing.

import (
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// tsNoConnection reads the value of consent.reason.no_connection, on its own
// line or the next.
var tsNoConnection = regexp.MustCompile(`"consent\.reason\.no_connection":\s*"([^"]*)"`)

// dealWords name deal-type evidence in the three languages the app ships.
var dealWords = []string{"deal", "opportunit", "verkaufschance", "geschäft", "cơ hội", "giao dịch"}

func TestTheNoConnectionCopyNamesNoDeal(t *testing.T) {
	t.Parallel()
	for _, locale := range []string{"en", "de", "vi"} {
		path := "../frontend/src/i18n/" + locale + ".ts"
		source, err := gatekit.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		found := tsNoConnection.FindSubmatch(source)
		if found == nil {
			t.Fatalf("%s has no consent.reason.no_connection: this gate reads a key that is gone", path)
		}
		sentence := strings.ToLower(string(found[1]))
		for _, word := range dealWords {
			if strings.Contains(sentence, word) {
				t.Errorf("%s: %q names %q, and the guard never reads a deal", path, found[1], word)
			}
		}
	}
}
