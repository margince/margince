// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/promptref"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// parseIDs reads a reply a test wrote in record ids as the model writes it:
// in the refs the request showed.
func parseIDs(text string, in StatusInput) (WrittenStatus, error) {
	return ParseStatus(promptref.Rewrite(text, refsOf(in).tables()...), in)
}

func TestTheSummaryNamesRecordsByRefAndTheReplyReadsBack(t *testing.T) {
	in := inputWithTimeline()
	content := StatusRequest(in, string(textlang.English)).Messages[0].Content
	for _, id := range []string{"deal-1", "act-1", "act-2"} {
		if strings.Contains(content, id) {
			t.Errorf("the summary shows the record id %q:\n%s", id, content)
		}
	}
	got, err := ParseStatus(reply(draft{story: []map[string]any{line("The call set the next step.", "a2", "a9", "act-1")}}), in)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Story) != 1 || !slices.Equal(got.Story[0].Evidence, []string{"act-2"}) {
		t.Errorf("story = %+v, want one line citing act-2 alone: a9 was never shown and act-1 is no ref", got.Story)
	}
}

func TestAReplyThatSpellsARefInReaderTextIsRefused(t *testing.T) {
	_, err := ParseStatus(reply(draft{story: []map[string]any{line("As a1 shows, the call happened.", "a1")}}), inputWithTimeline())
	if !errors.Is(err, errRefInReaderText) {
		t.Fatalf("err = %v, want the reply refused for a ref in reader text", err)
	}
}
