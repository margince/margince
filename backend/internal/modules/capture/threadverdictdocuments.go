// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

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
	// Personnel is the document rules' answer over more of the text than
	// Excerpt holds: a CV's headings often start past the excerpt.
	Personnel bool `json:"-"`
}

// IsThreadDocument says whether a file is a document a thread verdict must
// read before it may open the thread. The claim, the earlier messages a
// verdict stamps and an arriving reply all ask this one question.
//
// An office type or a zip is a document whatever its name says. The sender
// chooses the name, and the sniffer stores every xlsx as a zip. A text type
// counts only without an extension: a calendar invite sniffs as text/plain,
// and its .ics extension keeps it out.
func IsThreadDocument(filename, contentType string) bool {
	if NamedAsPrivate(filename) {
		return true
	}
	ext := strings.ToLower(path.Ext(filename))
	bare, _, _ := strings.Cut(contentType, ";")
	bare = strings.ToLower(strings.TrimSpace(bare))
	if slices.Contains(doctext.OfficeExtensions, ext) || slices.Contains(doctext.TextExtensions, ext) ||
		bare == "application/zip" || slices.Contains(doctext.OfficeTypes, bare) {
		return true
	}
	return ext == "" && slices.Contains(doctext.TextTypes, bare)
}

// storedFilesSQL is a JSON list of the stored, live files on the message
// whose id is activity. It lists at most limit of them; the caller asks for
// one past its bound, so a longer list is known to be longer.
func storedFilesSQL(activity, limit string) string {
	return `coalesce((SELECT jsonb_agg(jsonb_build_object('id', f.id, 'filename', f.filename,
	                     'content_type', coalesce(f.content_type, ''),
	                     'byte_size', coalesce(f.byte_size, 0)) ORDER BY f.filename, f.id)
	          FROM (SELECT at.id, at.filename, at.content_type, at.byte_size FROM attachment at
	                 WHERE at.entity_type = 'activity' AND at.entity_id = ` + activity + `
	                   AND at.archived_at IS NULL AND at.storage_key <> '' AND NOT at.bytes_withheld
	                 ORDER BY at.filename, at.id LIMIT ` + limit + `) f), '[]')`
}

// maxStoredFiles bounds how many of one message's files a query lists.
const maxStoredFiles = 200

// threadDocuments keeps the documents among a message's files, the first
// MaxThreadDocuments of them, and counts the rest. A list cut at its bound
// counts one more, because a file past the bound is unknown.
func threadDocuments(files []ThreadDocument) (kept []ThreadDocument, omitted int) {
	if len(files) > maxStoredFiles {
		files, omitted = files[:maxStoredFiles], 1
	}
	for _, f := range files {
		if !IsThreadDocument(f.Filename, f.ContentType) {
			continue
		}
		if len(kept) == MaxThreadDocuments {
			omitted++
			continue
		}
		kept = append(kept, f)
	}
	return kept, omitted
}

// activityCarriesDocumentTx says whether a stored message carries a document.
func activityCarriesDocumentTx(ctx context.Context, tx pgx.Tx, activity ids.UUID) (bool, error) {
	var files []ThreadDocument
	if err := tx.QueryRow(ctx, `SELECT `+storedFilesSQL("$1", "$2"), activity, maxStoredFiles+1).Scan(&files); err != nil {
		return false, fmt.Errorf("capture: reading a message's documents: %w", err)
	}
	return hasThreadDocument(files), nil
}

// hasThreadDocument says whether any of files is a document.
func hasThreadDocument(files []ThreadDocument) bool {
	kept, omitted := threadDocuments(files)
	return len(kept) > 0 || omitted > 0
}
