// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// How many bytes one request may carry on each route that accepts a file.
// The operator chooses the numbers; which routes may carry a file stays in
// uploadCeilings, so no route can claim the file bound at run time.
//
// Decimal megabytes: the key, the server's refusal and the upload form's hint
// all state this number, and `25 << 20` reads as 25 MB while admitting 26.2
// million bytes. The floor is 1 MB, below which the JSON bound governs; past
// 100 MB the answer is an upload straight to object storage, not a bigger
// number here.

import "github.com/margince/margince/backend/internal/platform/config"

// bytesPerMB is the decimal megabyte the upload ceilings are written in.
const bytesPerMB = 1_000_000

// The per-route ceilings, in decimal MB. A corpus document is plain text and
// costs passages the ask scans, so its ceiling is the lowest.
var (
	UploadAttachmentMB = config.Define(config.Spec[int]{
		Key: "uploads.attachment_mb", Env: "MARGINCE_UPLOADS_ATTACHMENT_MB", Type: config.Int,
		Default: config.Some(25), Min: config.Some(1), Max: config.Some(100), Roles: bothRoles,
		Doc: "Largest request POST /v1/attachments takes, in decimal MB.",
	})
	UploadCSVImportMB = config.Define(config.Spec[int]{
		Key: "uploads.csv_import_mb", Env: "MARGINCE_UPLOADS_CSV_IMPORT_MB", Type: config.Int,
		Default: config.Some(10), Min: config.Some(1), Max: config.Some(100), Roles: bothRoles,
		Doc: "Largest request POST /v1/imports/sources and the vCard import take, in decimal MB.",
	})
	UploadLinkedInImportMB = config.Define(config.Spec[int]{
		Key: "uploads.linkedin_import_mb", Env: "MARGINCE_UPLOADS_LINKEDIN_IMPORT_MB", Type: config.Int,
		Default: config.Some(8), Min: config.Some(1), Max: config.Some(100), Roles: bothRoles,
		Doc: "Largest request POST /v1/me/linkedin-connections takes, in decimal MB.",
	})
	UploadKnowledgeDocumentMB = config.Define(config.Spec[int]{
		Key: "uploads.knowledge_document_mb", Env: "MARGINCE_UPLOADS_KNOWLEDGE_DOCUMENT_MB", Type: config.Int,
		Default: config.Some(5), Min: config.Some(1), Max: config.Some(100), Roles: bothRoles,
		Doc: "Largest document POST /v1/knowledge/corpora/{id}/documents takes, in decimal MB.",
	})
)

// UploadLimits are the ceilings in bytes, every one present, so no consumer
// reads a setting or applies a default a second time.
type UploadLimits struct {
	Attachment        int64
	CSVImport         int64
	LinkedInImport    int64
	KnowledgeDocument int64
}

// UploadLimitsFrom resolves the loaded ceilings.
func UploadLimitsFrom(c *config.Config) UploadLimits {
	return UploadLimits{
		Attachment:        megabytes(config.Val(c, UploadAttachmentMB)),
		CSVImport:         megabytes(config.Val(c, UploadCSVImportMB)),
		LinkedInImport:    megabytes(config.Val(c, UploadLinkedInImportMB)),
		KnowledgeDocument: megabytes(config.Val(c, UploadKnowledgeDocumentMB)),
	}
}

// DefaultUploadLimits are the ceilings of a composition no file configured:
// the settings' own defaults.
func DefaultUploadLimits() UploadLimits {
	return UploadLimits{
		Attachment:        megabytes(defaultOf(UploadAttachmentMB)),
		CSVImport:         megabytes(defaultOf(UploadCSVImportMB)),
		LinkedInImport:    megabytes(defaultOf(UploadLinkedInImportMB)),
		KnowledgeDocument: megabytes(defaultOf(UploadKnowledgeDocumentMB)),
	}
}

func uploadSettings() []config.Any {
	return []config.Any{UploadAttachmentMB, UploadCSVImportMB, UploadLinkedInImportMB, UploadKnowledgeDocumentMB}
}

func megabytes(mb int) int64 { return int64(mb) * bytesPerMB }

func defaultOf(s *config.Setting[int]) int {
	v, _ := s.Spec().Default.Get()
	return v
}
