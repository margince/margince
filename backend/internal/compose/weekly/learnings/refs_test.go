// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package learnings

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/promptref"
)

// parseIDs reads a reply a test wrote in deal ids as the model writes it: in
// the refs the request showed.
func parseIDs(reply string, in Input) ([]Learning, error) {
	return Parse(promptref.Rewrite(reply, dealRefs(in)), in)
}

func TestTheWeekNamesDealsByRefAndACitationReadsBack(t *testing.T) {
	in := weekWithThree()
	content := Request(in, "en").Messages[0].Content
	for _, id := range []string{dealA.String(), dealB.String(), dealC.String()} {
		if strings.Contains(content, id) {
			t.Fatalf("the week shows the deal id %s:\n%s", id, content)
		}
	}
	got, err := Parse(reply(learning(KindWorked, "Reaching the sponsor early won Nordwind.", `{"type":"deal","id":"d1"}`)), in)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Citations[0].SubjectID != dealA {
		t.Errorf("d1 read back as %s, want %s", got[0].Citations[0].SubjectID, dealA)
	}
}

func TestACitationOfARefNeverShownOrARawIDRefusesTheReply(t *testing.T) {
	in := weekWithThree()
	for _, ref := range []string{"d4", dealA.String()} {
		raw := reply(learning(KindWorked, "Reaching the sponsor early won Nordwind.", `{"type":"deal","id":"`+ref+`"}`))
		if _, err := Parse(raw, in); err == nil {
			t.Errorf("a reply citing %q was kept", ref)
		}
	}
}

func TestALearningSpellingARefInItsTextIsRefused(t *testing.T) {
	raw := reply(learning(KindWorked, "Reaching the sponsor early won d1.", `{"type":"deal","id":"d1"}`))
	if _, err := Parse(raw, weekWithThree()); err == nil {
		t.Error("a learning spelling a ref in its text was kept")
	}
}
