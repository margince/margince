// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"fmt"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

func TestDocumentExplanationsCoverEveryLanguage(t *testing.T) {
	langcopytest.Census(t, documentWords)
	langcopytest.GermanStyle(t, documentWords)
	langcopytest.Census(t, chatWords)
	langcopytest.GermanStyle(t, chatWords)
	// The panel shows each phrase as its whole message.
	langcopytest.SentenceStart(t, documentWords)
	langcopytest.SentenceStart(t, chatWords)
}

func TestARefusedReadingNamesItsCause(t *testing.T) {
	refused := func(cause error) error { return fmt.Errorf("%w: %w", errRefusedDocument, cause) }
	unconfigured := refused(fmt.Errorf("%w: %w", ai.ErrUnconfiguredModel, ai.ErrOutputRejected))
	d := &DocumentExtractor{said: documentCopyFor("de")}
	cases := []struct {
		name string
		q    documentQuestion
		err  error
		want string
	}{
		{"no provider, deal facts", dealFactsQuestion(), unconfigured, documentWords.NoModel.De},
		{"no provider, chat", chatCaptureQuestion(), unconfigured, documentWords.NoModel.De},
		{"rejected reply", dealFactsQuestion(), refused(ai.ErrOutputRejected), documentWords.Refused.De},
		{"mislabelled bytes", dealFactsQuestion(), refused(model.ErrAttachmentMislabelled), documentWords.Mislabelled.De},
		{"rejected chat reading", chatCaptureQuestion(), refused(ai.ErrOutputRejected), chatWords.Refused.De},
	}
	for _, c := range cases {
		if got := d.refusedReadingDetail(c.q, c.err); got != c.want {
			t.Errorf("%s → %q, want %q", c.name, got, c.want)
		}
	}
	if english := documentWords.NoModel.En; !strings.Contains(english, "AI provider") || !strings.Contains(english, "Settings") {
		t.Errorf("the no-provider sentence names neither the provider nor where to add one: %q", english)
	}
	// The chat panel has no control to read a screenshot again.
	for _, word := range []string{"read again", "document"} {
		if strings.Contains(chatWords.Refused.En, word) {
			t.Errorf("the chat refusal says %q: %q", word, chatWords.Refused.En)
		}
	}
}

func TestATranscriptRefusedForWantOfAProviderClosesAsNoModel(t *testing.T) {
	unconfigured := fmt.Errorf("%w: %w", errRefusedTranscript, fmt.Errorf("%w: %w", ai.ErrUnconfiguredModel, ai.ErrOutputRejected))
	if got := refusedTranscriptFailure(unconfigured).Code; got != activities.TranscriptFailureNoModel {
		t.Errorf("unconfigured model → %q, want %q", got, activities.TranscriptFailureNoModel)
	}
	if got := refusedTranscriptFailure(fmt.Errorf("%w: %w", errRefusedTranscript, ai.ErrOutputRejected)).Code; got != activities.TranscriptFailureRefused {
		t.Errorf("rejected reply → %q, want %q", got, activities.TranscriptFailureRefused)
	}
	for _, code := range []string{
		activities.TranscriptFailureNotTranscript, activities.TranscriptFailureTooLong, activities.TranscriptFailureBlank,
		activities.TranscriptFailureMissing, activities.TranscriptFailureRefused, activities.TranscriptFailureNoModel,
		activities.TranscriptFailureUnanswered, activities.TranscriptFailureUnfinished,
	} {
		for _, lang := range textlang.Shipped {
			got := transcriptCopyFor(string(lang)).failureSentence(activities.TranscriptFailure{Code: code})
			if r, _ := utf8.DecodeRuneInString(got); !unicode.IsUpper(r) {
				t.Errorf("transcript failure %s in %s starts in lower case: %q", code, lang, got)
			}
		}
	}
}

func TestDocumentLengthExplainsCharactersRatherThanBytes(t *testing.T) {
	text := strings.Repeat("ü", maxDocumentTextChars+1)
	for _, language := range []string{"en", "de", "vi"} {
		reader := &DocumentExtractor{said: documentCopyFor(language)}
		source, detail := reader.textSource(textAttachment("text/plain"), []byte(text))
		if source.Text != "" || !strings.Contains(detail, fmt.Sprint(maxDocumentTextChars+1)) || strings.Contains(detail, fmt.Sprint(len(text))) {
			t.Fatalf("%s misreports the character bound: %s", language, detail)
		}
	}
}
