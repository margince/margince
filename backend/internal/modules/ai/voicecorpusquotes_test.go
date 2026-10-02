// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// prepareKnowingNoOne prepares a source in a workspace that recognises nobody,
// which is how every label read before anyone could be named.
func prepareKnowingNoOne(in IngestSourceInput) (preparedSource, error) {
	return prepareSource(context.Background(), in, nil)
}

// workspaceKnowing recognises exactly the named speakers, as the compose seam would.
func workspaceKnowing(names ...string) KnownSpeakers {
	known := map[string]bool{}
	for _, name := range names {
		known[normalizeSpeaker(name)] = true
	}
	return func(_ context.Context, labels []string) ([]string, error) {
		var named []string
		for _, label := range labels {
			if known[normalizeSpeaker(label)] {
				named = append(named, label)
			}
		}
		return named, nil
	}
}

const quotedMemo = "Sam: we ship on Friday\n\n" +
	"Frage: Ich hatte nicht geplant zu kommen. Was haelst du von der Idee?\n\n" +
	"The room went quiet after that and somebody opened a window."

func prepareMemo(t *testing.T, content string, known KnownSpeakers) preparedSource {
	t.Helper()
	prepared, err := prepareSource(context.Background(), IngestSourceInput{
		Kind: voiceSourceKindDocument, Format: corpusWireFormatText,
		SourceLabel: "memo.txt", Content: content,
	}, known)
	if err != nil {
		t.Fatal(err)
	}
	return prepared
}

func TestAQuotedSpeakerInProseIsNotTheOwnersVoice(t *testing.T) {
	prepared := prepareMemo(t, quotedMemo, workspaceKnowing("Sam"))
	if strings.Contains(prepared.Text, "ship on Friday") {
		t.Fatalf("Sam's words reached the owner's corpus: %q", prepared.Text)
	}
	if !strings.Contains(prepared.Text, "Ich hatte nicht geplant") || !strings.Contains(prepared.Text, "opened a window") {
		t.Fatalf("the owner's heading and narration are their own and stay: %q", prepared.Text)
	}
	if prepared.Stats.DiscardedTurns != 1 || prepared.Stats.InputWords != WordCount(quotedMemo) {
		t.Fatalf("stats = %+v, want one discarded turn out of the whole source", prepared.Stats)
	}
	if prepared.Words != prepared.Stats.KeptWords || prepared.Words != WordCount(quotedMemo)-WordCount("Sam: we ship on Friday") {
		t.Fatalf("words = %d, kept = %d — only Sam's line leaves", prepared.Words, prepared.Stats.KeptWords)
	}
}

func TestAWrappedQuoteLeavesWhole(t *testing.T) {
	memo := "Sam: we ship on Friday\nand test on Monday\n\nThat was the plan."
	prepared := prepareMemo(t, memo, workspaceKnowing("sam"))
	if prepared.Text != "\nThat was the plan." {
		t.Fatalf("text = %q — a label holds to the blank line, so the wrapped line is Sam's too", prepared.Text)
	}
}

func TestProseNobodyIsNamedInPassesUnchanged(t *testing.T) {
	prepared := prepareMemo(t, quotedMemo, workspaceKnowing("Anna"))
	if prepared.Text != quotedMemo || prepared.Stats.DiscardedTurns != 0 {
		t.Fatalf("a source naming nobody in the workspace is stored as written: %q %+v", prepared.Text, prepared.Stats)
	}
}

func TestEachLabelIsAskedAboutOnce(t *testing.T) {
	var asked []string
	known := func(_ context.Context, labels []string) ([]string, error) {
		asked = labels
		return nil, nil
	}
	prepareMemo(t, "Sam: one\n\nsam: two\n\nFrage: three\n\nnarration", known)
	if strings.Join(asked, ",") != "Sam,Frage" {
		t.Fatalf("asked = %v, want each distinct label once in first-seen order", asked)
	}
}

func TestProseThatIsAllQuotesIsRefused(t *testing.T) {
	_, err := prepareSource(context.Background(), IngestSourceInput{
		Kind: voiceSourceKindDocument, SourceLabel: "chat.txt",
		Content: "Sam: we ship on Friday\n\nAnna: fine by me",
	}, workspaceKnowing("Sam", "Anna"))
	var ingest *CorpusIngestError
	if !errors.As(err, &ingest) || ingest.Code != CorpusErrUnattributedTranscript {
		t.Fatalf("err = %v — nothing of the owner's own is left to store", err)
	}
}

func TestAWorkspaceThatCannotAnswerStopsTheIngest(t *testing.T) {
	unreachable := errors.New("directory unavailable")
	_, err := prepareSource(context.Background(), IngestSourceInput{
		Kind: voiceSourceKindDocument, SourceLabel: "memo.txt", Content: quotedMemo,
	}, func(context.Context, []string) ([]string, error) { return nil, unreachable })
	if !errors.Is(err, unreachable) {
		t.Fatalf("err = %v — an unanswered name check must not store the quote as the owner's", err)
	}
}
