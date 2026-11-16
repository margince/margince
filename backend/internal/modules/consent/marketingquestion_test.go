// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/platform/mailcopy"
)

// A grant names its question by key, version and language, so the words under
// one version must never move. The gate in backend/gates holds the screen and
// mailcopy to the same words; this holds those words to their version.

// pinnedQuestion is the sha256 of each question body this build publishes,
// keyed key@version@locale. Like pinnedWording it holds this build only. The
// boot already refuses a published version whose body changed.
//
// gatekit:fixture the sha256 of each published question body
var pinnedQuestion = map[string]string{
	"marketing_question@6@de":    "9d465acdd71805ba27a8e46c7738e012d21334c0c682e3382493ec96d3650b78",
	"marketing_question@6@en":    "81c5b8d820503d80eab76f2936f7374906a3d940e6d54fc3f75cff6e62e7224d",
	"marketing_question@6@vi":    "09e932743c70bf1f4489a52d455de5d553a87d90e6822294cc438febc4d1f50c",
	"subscription_question@6@de": "98a4d7ed1a0a8be667409b73473d9c07ab81726426ddf8a59634349efe25a2a6",
	"subscription_question@6@en": "f8fab3f8b7b4fc5d6c51cf6ffe5f387f578b0d90cc847dbc0b9407c26f42e231",
	"subscription_question@6@vi": "6dc15e6c17bb477d6ba49ea99c2a7df3c1ccebe76e9d59095505e7fe26fabcc2",
}

// TestEveryPublishedQuestionIsPinnedToItsVersion fails when a question's words
// change and marketingQuestionVersion does not, and when a version is reused
// for words it did not publish.
func TestEveryPublishedQuestionIsPinnedToItsVersion(t *testing.T) {
	t.Parallel()

	keys := []string{TemplateMarketingQuestion, TemplateSubscriptionQuestion}
	seen := map[string]bool{}
	for _, key := range keys {
		for _, language := range mailcopy.Languages() {
			sum := sha256.Sum256([]byte(RenderQuestion(key, string(language))))
			got := hex.EncodeToString(sum[:])
			pin := fmt.Sprintf("%s@%d@%s", key, marketingQuestionVersion, language)
			seen[pin] = true
			want, pinned := pinnedQuestion[pin]
			switch {
			case !pinned:
				t.Errorf("%s has no pin. If you changed the words, bump marketingQuestionVersion "+
					"and replace every row with one for the new version; the new hash is %q", pin, got)
			case got != want:
				t.Errorf("%s words changed but marketingQuestionVersion did not.\n  pinned: %s\n"+
					"  now:    %s\nA grant under this version names the pinned words. Bump the "+
					"version and move every pin to it", pin, want, got)
			}
		}
	}
	if len(seen) < 6 {
		t.Fatalf("pinned %d question bodies, want two questions in three languages: the gate "+
			"has stopped seeing its subject", len(seen))
	}
	for pin := range pinnedQuestion {
		if !seen[pin] {
			t.Errorf("%s is pinned but this build publishes no such body. Remove the row: a "+
				"grant under an older version keeps its words in consent_text_version", pin)
		}
	}
}
