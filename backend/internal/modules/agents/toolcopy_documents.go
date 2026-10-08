// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import "github.com/margince/margince/backend/internal/platform/httperr"

// Without this tool an agent handed a PDF saved its text as a note.
var attachDocumentCopy = toolCopy{
	Purpose: "Put a file the user gave you on a company, contact, deal, lead or project, where it " +
		"appears on the record's Documents tab — use it whenever the user wants a file kept on a " +
		"record, and never save a file's text as a note instead.",
	Limits: "Up to " + httperr.Megabytes(maxInlineFileBytes) + " per file, or less where the " +
		"workspace sets a smaller upload limit. Common document, image and email formats are " +
		"accepted; any other kind is refused, and the refusal names the accepted ones. The file is " +
		"stored, not read, and is not filed against a contract.",
	Instead: "Use log_activity for what was said about the file, linked to the same record.",
	Retain:  "Keep attachment_id to name the file to the user.",
}

var listDocumentsCopy = toolCopy{
	Purpose: "List the files stored on a company, contact, deal, lead or project, newest first: " +
		"name, type, size and who added them.",
	Limits: "It lists and never returns a file's contents. Check it before attaching a file " +
		"again, so the record does not carry the same file twice.",
	Retain: "Keep next_cursor to read the next page.",
}
