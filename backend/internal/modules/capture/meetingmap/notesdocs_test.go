// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingmap

import (
	"reflect"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture"
)

const docType = "application/vnd.google-apps.document"

// Only the notes and transcripts Google writes for a Meet call count: a Google
// Doc whose title ends the way Google titles them, on a meeting held on Meet.
func TestOnlyGooglesNotesAndTranscriptsOnAMeetCallCount(t *testing.T) {
	t.Parallel()
	notes := Document{FileID: "1notesFileId0123", Title: "Review - 2026/10/30 10:00 CET -  Notes by Gemini ", MimeType: docType}
	transcript := Document{FileID: "1transcriptId0123", Title: "Review - Transcript", MimeType: docType}
	german := Document{FileID: "1germanNotes0123", Title: "Review – Notizen von Gemini", MimeType: docType}
	germanTranscript := Document{FileID: "1germanTrans0123", Title: "Review - Transkript", MimeType: docType} //nolint:misspell // German title
	agenda := Document{FileID: "1agendaFileId0123", Title: "Agenda", MimeType: docType}
	pdf := Document{FileID: "1pdfFileId012345", Title: "Review - Transcript", MimeType: "application/pdf"}
	badID := Document{FileID: "../escape/0123456", Title: "Review - Transcript", MimeType: docType}

	got := NotesDocuments(Event{GoogleMeet: true, Documents: []Document{notes, agenda, transcript, pdf, badID, notes, german, germanTranscript}})
	want := []capture.MeetingDocument{
		{FileID: notes.FileID, Title: "Review - 2026/10/30 10:00 CET -  Notes by Gemini", Kind: capture.MeetingDocumentNotes},
		{FileID: transcript.FileID, Title: transcript.Title, Kind: capture.MeetingDocumentTranscript},
		{FileID: german.FileID, Title: german.Title, Kind: capture.MeetingDocumentNotes},
		{FileID: germanTranscript.FileID, Title: germanTranscript.Title, Kind: capture.MeetingDocumentTranscript},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("documents = %+v, want %+v", got, want)
	}
	if got := NotesDocuments(Event{Documents: []Document{notes, transcript}}); len(got) != 0 {
		t.Errorf("a meeting off Google Meet gave %+v, want none", got)
	}
}

// One meeting writes a bounded number of rows, whatever its attachment list says.
func TestAMeetingRecordsABoundedNumberOfDocuments(t *testing.T) {
	t.Parallel()
	var docs []Document
	for _, id := range []string{"1aaaaaaaaaaa", "1bbbbbbbbbbb", "1cccccccccccc", "1dddddddddddd", "1eeeeeeeeeeee"} {
		docs = append(docs, Document{FileID: id, Title: "x - Transcript", MimeType: docType})
	}
	if got := NotesDocuments(Event{GoogleMeet: true, Documents: docs}); len(got) != maxMeetingDocuments {
		t.Errorf("%d documents, want the bound %d", len(got), maxMeetingDocuments)
	}
}
