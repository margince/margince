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
	"marketing_question@5@en":    "e857e5468b71a88ad027e8a9a819392d686258513424c1c75781ff27a3856d7d",
	"marketing_question@5@de":    "4b157ad993538ea86f98410395239c0578da8cba22c4377ea9c845648e9dc08e",
	"marketing_question@5@vi":    "8d3fafee47b1f955bfea7390e8232252a9c6a7e9c8940e85984bda36823f6b9f",
	"subscription_question@5@en": "8e75d8851231582ef3754845c914f3155194ce99ef1b2724b72ca8a9047aa970",
	"subscription_question@5@de": "65679282ca4695990ad37950a95607fd6520458f0421815ef5349f53815ef60a",
	"subscription_question@5@vi": "6dfa5d398a194e9c93e69a4102d08bc0ea8e28c3e48a2b6d54f39e0e20b94dcb",
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
