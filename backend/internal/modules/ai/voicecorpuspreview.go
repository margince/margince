// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// The corpus dry-run and honesty surface (§B1.2's user-facing half): the
// speaker preview that lets the owner be asked "which of these is you?"
// BEFORE any words are committed, and the kept-versus-discarded ingest
// stats the conversational meter narrates from. Pure functions over the
// same parsers the ingest pipeline commits with. Every word total here
// sums SPOKEN text only — timestamps, cue counters, speaker labels and
// JSON keys are serialization, not words, and never inflate a count.

import "strings"

// corpusWireFormatText is the wire-level format discriminator (ingest and
// preview requests): plain single-author prose, no speaker structure.
const corpusWireFormatText = "text"

// CorpusSpeaker aggregates one detected speaker in a previewed source.
// Turns counts logical turns: consecutive lines of the same speaker (a
// wrapped cue) fold into one.
type CorpusSpeaker struct {
	Label string
	Turns int
	Words int
}

// CorpusPreview is the dry-run answer to "who speaks in this file?" —
// computed without storing anything, so the owner can be asked which
// speaker they are BEFORE any words enter the corpus.
type CorpusPreview struct {
	DetectedFormat         string
	TotalWords             int
	Speakers               []CorpusSpeaker
	UnattributedWords      int
	IngestibleAsTranscript bool
}

// PreviewCorpusText inspects one candidate source in its wire format
// (text | transcript, mirroring ingest). Pure — no persistence, no model.
func PreviewCorpusText(format, content string) (CorpusPreview, error) {
	if strings.TrimSpace(content) == "" {
		return CorpusPreview{}, &CorpusIngestError{Field: voiceKeyContent, Reason: voiceValidationNotEmpty}
	}
	if len(content) > maxCorpusSourceBytes {
		return CorpusPreview{}, &CorpusIngestError{Field: voiceKeyContent, Reason: "one source is capped at 1 MiB of text — split the upload"}
	}
	var concrete string
	switch format {
	case "", corpusWireFormatText:
		concrete = corpusFormatTxt
	case voiceSourceKindTranscript:
		concrete = transcriptCorpusFormat(content)
	default:
		return CorpusPreview{}, &CorpusIngestError{
			Field:  voiceKeyFormat,
			Code:   CorpusErrUnsupportedFormat,
			Reason: "must be text or transcript",
		}
	}
	turns, plain, err := corpusTurns(concrete, content)
	if err != nil {
		return CorpusPreview{}, err
	}
	if plain {
		return CorpusPreview{DetectedFormat: concrete, TotalWords: WordCount(content)}, nil
	}
	preview := CorpusPreview{DetectedFormat: concrete}
	index := map[string]int{}
	previousSpeaker := ""
	for _, turn := range turns {
		words := WordCount(turn.Text)
		preview.TotalWords += words
		if turn.Speaker == "" {
			preview.UnattributedWords += words
			previousSpeaker = ""
			continue
		}
		key := normalizeSpeaker(turn.Speaker)
		newRun := key != previousSpeaker
		previousSpeaker = key
		at, seen := index[key]
		if !seen {
			at = len(preview.Speakers)
			index[key] = at
			preview.Speakers = append(preview.Speakers, CorpusSpeaker{Label: turn.Speaker})
		}
		if newRun {
			preview.Speakers[at].Turns++
		}
		preview.Speakers[at].Words += words
	}
	// Whether the speaker filter can act on this source, and — when it cannot —
	// what the caller is holding instead.
	//
	// The parser accepts any letter-initial word before a colon, so "Frage:" in
	// an email and "Sam:" in a chat log arrive identically. Nothing in the text
	// separates them; only the SHARE of the words those labels own does. At or
	// above half the source is a conversation and the owner is asked which
	// speaker they are. Below half the labels are headings inside somebody's
	// own writing.
	//
	// The labels are reported either way. A caller reads an empty speakers list
	// as "single-author prose, ingest it whole", so erasing them is what would
	// carry a counterparty's words into the corpus as the owner's own. What the
	// caller does with a minority-labelled file is its own decision: the
	// Settings intake takes it as the prose it reads as, which is why an email
	// with two headings is not refused.
	preview.IngestibleAsTranscript = len(preview.Speakers) > 0 && attributedMajority(turns)
	return preview, nil
}

// attributedMajority reports whether at least half of a source's words belong
// to a named label. It is what separates a conversation from prose carrying
// headings: a meeting export attributes nearly every word, an email's "Frage:"
// a handful. Preview and ingest both decide by it, so a source the preview will
// not offer a speaker question for cannot be filtered to one on the write path
// either — filtering prose to one of its own headings stores a fragment and
// calls it a voice.
func attributedMajority(turns []speakerTurn) bool {
	attributed, total := 0, 0
	for _, turn := range turns {
		words := WordCount(turn.Text)
		total += words
		if turn.Speaker != "" {
			attributed += words
		}
	}
	return attributed > 0 && attributed*2 >= total
}

// CorpusIngestStats reports what the §B1.2 speaker filter did to one
// ingested source — the honest kept-versus-discarded story the meter's
// numbers rest on. InputWords counts the SPOKEN words of every turn (not
// serialization); turn counts fold wrapped same-speaker lines into one.
type CorpusIngestStats struct {
	InputWords     int
	KeptWords      int
	KeptTurns      int
	DiscardedTurns int
	SpeakersSeen   []string
}

func ingestStats(content string, turns []speakerTurn, plain bool, keptText, speakerLabel string) CorpusIngestStats {
	stats := CorpusIngestStats{KeptWords: WordCount(keptText)}
	if plain {
		stats.InputWords = WordCount(content)
		stats.KeptTurns = 1
		return stats
	}
	labelled := false
	for _, turn := range turns {
		if turn.Speaker != "" {
			labelled = true
			break
		}
	}
	// Mirrors filterOwnTurns exactly: in a labelled transcript only the
	// owner's turns survive (an unattributed stray is discarded); a fully
	// unlabelled one passes whole when it was allowed through at all.
	want := normalizeSpeaker(speakerLabel)
	seen := map[string]bool{}
	previousSpeaker := ""
	previousKept := false
	for _, turn := range turns {
		stats.InputWords += WordCount(turn.Text)
		if turn.Speaker != "" && !seen[normalizeSpeaker(turn.Speaker)] {
			seen[normalizeSpeaker(turn.Speaker)] = true
			stats.SpeakersSeen = append(stats.SpeakersSeen, turn.Speaker)
		}
		key := normalizeSpeaker(turn.Speaker)
		kept := !labelled || key == want
		if key != previousSpeaker || kept != previousKept {
			if kept {
				stats.KeptTurns++
			} else {
				stats.DiscardedTurns++
			}
		}
		previousSpeaker = key
		previousKept = kept
	}
	return stats
}
