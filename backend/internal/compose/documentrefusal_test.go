// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// refusedWith is a refusal as ask returns it: the reading's own sentinel over
// what the model lane said.
func refusedWith(cause error) error {
	return fmt.Errorf("%w: %w", errRefusedDocument, cause)
}

func TestAReadingRefusedForWantOfAProviderNamesTheProviderAndWhoAddsIt(t *testing.T) {
	unconfigured := refusedWith(fmt.Errorf("%w: %w", ai.ErrUnconfiguredModel, ai.ErrOutputRejected))
	for _, lang := range textlang.Shipped {
		d := &DocumentExtractor{said: documentCopyFor(string(lang))}
		for _, q := range []documentQuestion{dealFactsQuestion(), chatCaptureQuestion()} {
			got := d.refusedReadingDetail(q, unconfigured)
			if got != documentWords.NoModel.In(lang) {
				t.Errorf("%s/%s: unconfigured model → %q, want the no-provider sentence", lang, q.name, got)
			}
		}
	}
	got := (&DocumentExtractor{said: documentCopyFor("en")}).refusedReadingDetail(dealFactsQuestion(), unconfigured)
	if !strings.Contains(got, "AI provider") || !strings.Contains(got, "Settings") {
		t.Errorf("the no-provider sentence names neither the provider nor where to add one: %q", got)
	}
}

func TestARefusedReadingKeepsItsOtherCauses(t *testing.T) {
	d := &DocumentExtractor{said: documentCopyFor("en")}
	if got := d.refusedReadingDetail(dealFactsQuestion(), refusedWith(ai.ErrOutputRejected)); got != documentWords.Refused.In(textlang.English) {
		t.Errorf("a rejected reply → %q, want the general refusal", got)
	}
	if got := d.refusedReadingDetail(dealFactsQuestion(), refusedWith(model.ErrAttachmentMislabelled)); got != documentWords.Mislabelled.In(textlang.English) {
		t.Errorf("mislabelled bytes → %q, want the mislabelled sentence", got)
	}
}

// The chat panel has no control to read a screenshot again, only the field
// to type the chat into.
func TestARefusedChatReadingOffersNoSecondReading(t *testing.T) {
	for _, lang := range textlang.Shipped {
		d := &DocumentExtractor{said: documentCopyFor(string(lang))}
		got := d.refusedReadingDetail(chatCaptureQuestion(), refusedWith(ai.ErrOutputRejected))
		if got != chatWords.Refused.In(lang) {
			t.Errorf("%s: refused chat reading → %q, want the chat's own refusal", lang, got)
		}
	}
	english := chatWords.Refused.In(textlang.English)
	for _, word := range []string{"read again", "document"} {
		if strings.Contains(english, word) {
			t.Errorf("the chat refusal says %q: %q", word, english)
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
}

// Every phrase is shown on its own as the panel's whole message, so each reads
// as a sentence in every language.
func TestEveryReadingFailurePhraseStartsAsASentence(t *testing.T) {
	tables := map[string]any{"documentWords": documentWords, "chatWords": chatWords}
	for name, table := range tables {
		value := reflect.ValueOf(table)
		for i := range value.NumField() {
			phrase, ok := value.Field(i).Interface().(langcopy.Phrase)
			if !ok {
				t.Fatalf("%s.%s is not a phrase", name, value.Type().Field(i).Name)
			}
			assertSentenceStart(t, name+"."+value.Type().Field(i).Name, phrase)
		}
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

func assertSentenceStart(t *testing.T, name string, phrase langcopy.Phrase) {
	t.Helper()
	for _, lang := range textlang.Shipped {
		text := phrase.In(lang)
		if r, _ := utf8.DecodeRuneInString(text); !unicode.IsUpper(r) {
			t.Errorf("%s in %s starts in lower case: %q", name, lang, text)
		}
	}
}
