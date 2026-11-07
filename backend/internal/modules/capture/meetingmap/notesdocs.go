// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingmap

// Which documents attached to a meeting are the notes or the transcript Google
// wrote for it. A rule about meetings, not about Google's wire format: the
// decoder states what was attached, and this decides what counts.

import (
	"regexp"
	"strings"

	"github.com/margince/margince/backend/internal/modules/capture"
)

// Document is one file the provider lists as attached to the event.
type Document struct {
	FileID   string
	Title    string
	MimeType string
}

// googleDocMimeType is the MIME type Drive reports for a Google Doc. Notes and
// transcripts are always one; a PDF or a sheet attached by hand is not read.
const googleDocMimeType = "application/vnd.google-apps.document"

// notesTitleSuffixes are the endings Google gives the notes and transcript
// documents it writes for a meeting, per language. Google titles them
// "<meeting> - <date> - Notes by Gemini" and "<meeting> - Transcript". An
// agenda attached by hand carries no such ending, so it is not read.
//
// Vietnamese is not listed until its exact titles are known; a document with an
// unknown title is simply not matched.
var notesTitleSuffixes = []struct {
	suffix string
	kind   string
}{
	{"notes by gemini", capture.MeetingDocumentNotes},
	{"notizen von gemini", capture.MeetingDocumentNotes},
	{"transcript", capture.MeetingDocumentTranscript},
	{"transkript", capture.MeetingDocumentTranscript}, //nolint:misspell // German: Google's title for a transcript
}

// driveFileID is the shape of a Drive file id. Anything else is refused, so
// the document address built from it cannot point anywhere but at that file.
var driveFileID = regexp.MustCompile(`^[A-Za-z0-9_-]{10,200}$`)

// maxMeetingDocuments bounds the notes rows one meeting writes. Google writes
// one notes document and one transcript per meeting; a longer list is not theirs.
const maxMeetingDocuments = 4

// NotesDocuments picks the notes and transcripts Google wrote for this
// meeting, read from owner's calendar. All four must hold:
//   - the meeting ran on Google Meet;
//   - its organizer is on owner's own domain;
//   - the file is a Google Doc;
//   - its title ends the way Google titles them.
//
// The organizer matters because a title proves nothing on its own: anybody who
// sends an invitation can attach a Doc titled "... Transcript". Google writes
// the notes into the organizer's Drive, so an organizer of our own is the
// author the documents claim. The event states no file owner, so the
// organizer's domain is the proof. A repeated file counts once.
func NotesDocuments(ev Event, owner string) []capture.MeetingDocument {
	if !ev.GoogleMeet || !organizedInHouse(ev.Organizer.Email, owner) {
		return nil
	}
	var out []capture.MeetingDocument
	seen := map[string]bool{}
	for _, doc := range ev.Documents {
		id := strings.TrimSpace(doc.FileID)
		if doc.MimeType != googleDocMimeType || !driveFileID.MatchString(id) || seen[id] {
			continue
		}
		kind := notesKindOf(doc.Title)
		if kind == "" {
			continue
		}
		seen[id] = true
		out = append(out, capture.MeetingDocument{FileID: id, Title: strings.TrimSpace(doc.Title), Kind: kind})
		if len(out) == maxMeetingDocuments {
			break
		}
	}
	return out
}

// organizedInHouse reports whether the organizer is on the owner's own domain.
// An address with no domain on either side proves nothing, so it answers no.
func organizedInHouse(organizer, owner string) bool {
	organizerDom, ownerDom := domainOf(organizer), domainOf(owner)
	return organizerDom != "" && ownerDom != "" && organizerDom == ownerDom
}

// notesKindOf is the kind a title names, or "" when it is not one Google writes.
func notesKindOf(title string) string {
	folded := strings.ToLower(strings.Join(strings.Fields(title), " "))
	for _, s := range notesTitleSuffixes {
		if strings.HasSuffix(folded, s.suffix) {
			return s.kind
		}
	}
	return ""
}
