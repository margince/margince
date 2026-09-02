// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

// Conversations, folded into the threads they belong to.
//
// A twelve-message argument about one thing is one thing that happened, not
// twelve. Counting it as twelve is how a ranked history fills with the loudest
// week rather than the most important one, and how "29 interactions" turns out
// to be four conversations.
//
// The fold keys on `thread_key` where the capture wrote one. Where it did not —
// a meeting, a note, a message from a channel that carries no thread identity —
// the normalised subject stands in, because a reply keeps the subject and adds
// a prefix to it. A row with neither is its own thread: merging every
// subjectless note into one bucket would invent a conversation nobody had.

import (
	"sort"
	"strings"
	"time"
)

// thread is one conversation, folded.
type thread struct {
	Key     string
	Kind    string
	Subject string
	First   time.Time
	Last    time.Time
	// IDs are the conversation's activities this caller may READ, newest
	// first. A withheld row is counted in Rows and never listed here: it still
	// dates the thread, and citing it would send a reader to a record they
	// cannot open — which also tells them a limited conversation belongs to
	// this thread, a fact the audience narrowing exists to keep.
	IDs []string
	// Rows is how many conversations the thread holds, readable or not.
	Rows       int
	Inbound    int
	Outbound   int
	OnDeal     bool
	HasMeeting bool
	// Readable is how many of the thread's rows this caller may read. A thread
	// that is entirely withheld still exists and still dates the arc.
	Readable int
}

// replyPrefixes are the openers a reply adds to a subject, in the languages
// this product is used in. Stripped repeatedly, because a long thread stacks
// them ("AW: Re: WG: …").
var replyPrefixes = []string{"re:", "aw:", "fw:", "fwd:", "wg:", "antwort:", "tr:"}

// normalisedSubject reduces a subject to what two messages in one thread share.
func normalisedSubject(subject string) string {
	out := strings.ToLower(strings.TrimSpace(subject))
	for trimmed := true; trimmed; {
		trimmed = false
		for _, prefix := range replyPrefixes {
			if strings.HasPrefix(out, prefix) {
				out = strings.TrimSpace(strings.TrimPrefix(out, prefix))
				trimmed = true
			}
		}
	}
	return strings.Join(strings.Fields(out), " ")
}

// threadKeyOf decides which conversation a row belongs to.
func threadKeyOf(row HistoryIn) string {
	if row.ThreadKey != "" {
		return "t:" + row.ThreadKey
	}
	if subject := normalisedSubject(row.Subject); subject != "" {
		return "s:" + row.Kind + "|" + subject
	}
	// Nothing to merge on. Its own thread, keyed by its own id — a subjectless
	// note is not the same conversation as every other subjectless note.
	return "a:" + row.ID
}

// threadsOf folds a history into conversations, oldest first by their start.
func threadsOf(history []HistoryIn) []thread {
	byKey := map[string]*thread{}
	var order []string
	for _, row := range history {
		key := threadKeyOf(row)
		found, ok := byKey[key]
		if !ok {
			found = &thread{Key: key, Kind: row.Kind, First: row.At, Last: row.At}
			byKey[key] = found
			order = append(order, key)
		}
		found.Rows++
		if !row.Withheld {
			found.IDs = append(found.IDs, row.ID)
		}
		if row.At.Before(found.First) {
			found.First = row.At
		}
		if row.At.After(found.Last) {
			found.Last = row.At
		}
		// The newest readable subject names the thread: a later message in a
		// chain carries the subject the participants settled on.
		if !row.Withheld {
			if found.Subject == "" || row.At.Equal(found.Last) {
				if row.Subject != "" {
					found.Subject = row.Subject
				}
			}
		}
		switch row.Direction {
		case "inbound":
			found.Inbound++
		case "outbound":
			found.Outbound++
		}
		found.OnDeal = found.OnDeal || row.OnDeal
		found.HasMeeting = found.HasMeeting || row.Kind == "meeting"
	}
	threads := make([]thread, 0, len(order))
	for _, key := range order {
		threads = append(threads, *byKey[key])
	}
	sort.SliceStable(threads, func(i, j int) bool {
		return threads[i].First.Before(threads[j].First)
	})
	return threads
}
