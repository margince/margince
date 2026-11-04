// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import "github.com/margince/margince/backend/internal/platform/httperr"

// Without this tool an agent handed a PDF saved its text as a note.
var attachDocumentCopy = toolCopy{
	Purpose: "Put a file the user gave you on a company, contact, deal, lead or project, where it " +
		"appears on the record's Documents tab, or on one activity such as the meeting it came from " +
		"— use it whenever the user wants a file kept on a record, and never save a file's text as " +
		"a note instead.",
	Limits: "Up to " + httperr.Megabytes(MaxInlineFileBytes) + " per file, or less where the " +
		"workspace sets a smaller upload limit. Common document, image and email formats are " +
		"accepted; any other kind is refused, and the refusal names the accepted ones. Tell the user " +
		"about a refused file; never rename, convert or zip it to get it accepted. The file is " +
		"stored, not read. It is filed against a contract only when you name contract_id; " +
		"read_project (view: overview) lists its contracts with their ids.",
	Instead: "Use log_activity for what was said about the file, linked to the same record.",
	Retain:  "Keep attachment_id to name the file to the user.",
}

var listDocumentsCopy = toolCopy{
	Purpose: "List the files stored on a company, contact, deal, lead, project or activity, " +
		"newest first: name, type, size, who added them and the contract each is filed against. " +
		"Send entity_type and entity_id to list, or a file's id to read its contents.",
	Limits: "A stored file answers as content_base64, up to " + httperr.Megabytes(MaxInlineFileBytes) + "; a larger one is " +
		"refused with the limit. A document link or a withheld mail attachment has no bytes to read, so it is " +
		"refused too. Check the list before attaching a file again, so the record does not " +
		"carry the same file twice.",
	Retain: "Keep next_cursor to read the next page.",
	Instead: "Use read_view with view company_documents or deal_documents for a company's or deal's " +
		"document register, filtered by category, contract or state.",
}
