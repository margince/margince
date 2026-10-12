// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// A label inside somebody's own prose is either a heading ("Frage:") or a
// quoted speaker ("Sam: we ship on Friday"), and the two are textually
// identical — no rule over the text separates them. The workspace does: a
// label naming a contact or colleague the owner can see is somebody else
// speaking, so that turn leaves the source before it enters the corpus. Every
// other label is the owner's own heading and stays.

import (
	"context"
	"strings"
)

// KnownSpeakers answers which of a source's labels name a contact or colleague
// in the workspace other than the caller. It returns the labels it recognised, as
// given; a label it does not return is read as the owner's own heading.
type KnownSpeakers func(ctx context.Context, labels []string) ([]string, error)

// withoutKnownSpeakers takes the turns of every speaker the workspace recognises
// out of a prose source. A source with nothing left was never the owner's
// writing, so it is refused rather than stored empty.
func withoutKnownSpeakers(ctx context.Context, content string, known KnownSpeakers) (text string, removedTurns int, err error) {
	labels := proseLabels(content)
	if known == nil || len(labels) == 0 {
		return content, 0, nil
	}
	quoted, err := known(ctx, labels)
	if err != nil {
		return "", 0, err
	}
	text, removedTurns = withoutQuotedTurns(content, quoted)
	if strings.TrimSpace(text) == "" {
		return "", 0, &CorpusIngestError{
			Field:  voiceKeyContent,
			Code:   CorpusErrUnattributedTranscript,
			Reason: "every line of this source is attributed to someone in the workspace, so none of it is the owner's own writing; send it as a transcript and name the owner's speaker label",
		}
	}
	return text, removedTurns, nil
}

// WithKnownSpeakers returns a store whose ingest takes quoted speakers' turns out
// of prose.
func (s *VoiceStore) WithKnownSpeakers(known KnownSpeakers) *VoiceStore {
	copied := *s
	copied.knownSpeakers = known
	return &copied
}

// WithVoiceKnownSpeakers rebinds the handler set's voice store with the
// workspace's contacts and colleagues.
func (h Handlers) WithVoiceKnownSpeakers(known KnownSpeakers) Handlers {
	h.voice = h.voice.WithKnownSpeakers(known)
	return h
}

// proseLabels lists the distinct labels a prose source carries, in first-seen
// order — the names the workspace is asked about.
func proseLabels(content string) []string {
	seen := map[string]bool{}
	var labels []string
	for _, line := range scanSRT(content) {
		key := normalizeSpeaker(line.Speaker)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		labels = append(labels, line.Speaker)
	}
	return labels
}

// withoutQuotedTurns rebuilds prose without every line attributed to a quoted
// label, and counts the turns it removed. The lines it keeps are the source's
// own, unchanged, so a heading's paragraph reaches the corpus as written.
func withoutQuotedTurns(content string, quoted []string) (text string, removedTurns int) {
	if len(quoted) == 0 {
		return content, 0
	}
	drop := make(map[string]bool, len(quoted))
	for _, label := range quoted {
		drop[normalizeSpeaker(label)] = true
	}
	var kept []string
	previous := ""
	for _, line := range scanSRT(content) {
		key := normalizeSpeaker(line.Speaker)
		if !drop[key] {
			kept = append(kept, line.Raw)
			previous = ""
			continue
		}
		if key != previous {
			removedTurns++
		}
		previous = key
	}
	return strings.Join(kept, "\n"), removedTurns
}
