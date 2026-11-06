// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"slices"
	"strconv"

	"github.com/margince/margince/backend/internal/platform/doctext"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// MaxThreadDocuments is how many of a first message's documents the claim
// hands the verdict. The rest are counted in DocumentsOmitted, so the
// classifier is told it did not see every file.
const MaxThreadDocuments = 3

// ThreadDocument is one stored document on the message a thread verdict reads.
//
// The claim fills the row facts. The reader of the bytes fills Excerpt,
// Truncated and Unreadable after the claim commits. They are never stored: a
// second copy of the text at rest would escape every purge of the file.
type ThreadDocument struct {
	AttachmentID ids.UUID `json:"id"`
	Filename     string   `json:"filename"`
	ContentType  string   `json:"content_type"`
	ByteSize     int64    `json:"byte_size"`
	Excerpt      string   `json:"-"`
	Truncated    bool     `json:"-"`
	Unreadable   bool     `json:"-"`
}

// threadDocumentSQL is the condition that an attachment row is a stored
// document on the first message of the claimed verdict v. The caller adds
// that the message is not restricted.
//
// An office type or a zip is a document whatever its name says. The sender
// chooses the name, and the sniffer stores every xlsx as a zip. A text type
// counts only without an extension: a calendar invite sniffs as text/plain,
// and its .ics extension keeps it out.
func threadDocumentSQL(arg func(any) int) string {
	ext := `lower(coalesce(substring(at.filename from '\.[^./]*$'), ''))`
	bareType := `lower(btrim(split_part(coalesce(at.content_type, ''), ';', 1)))`
	return `at.entity_type = 'activity' AND at.entity_id = v.first_activity_id
	   AND at.archived_at IS NULL AND at.storage_key <> '' AND NOT at.bytes_withheld
	   AND (` + ext + ` = ANY($` + strconv.Itoa(arg(documentExtensions())) + `)
	        OR ` + bareType + ` = ANY($` + strconv.Itoa(arg(officeTypes())) + `)
	        OR (` + ext + ` = '' AND ` + bareType + ` = ANY($` + strconv.Itoa(arg(doctext.TextTypes)) + `)))`
}

// officeTypes are the types that make a file a document by themselves.
func officeTypes() []string { return append(slices.Clone(doctext.OfficeTypes), "application/zip") }

func documentExtensions() []string {
	return slices.Concat(doctext.OfficeExtensions, doctext.TextExtensions)
}
