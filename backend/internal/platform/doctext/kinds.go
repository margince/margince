// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package doctext

// OfficeTypes are the content types an office document arrives as. Images are
// not here: an image on a mail is almost always a logo or a signature part.
var OfficeTypes = []string{
	"application/pdf", "application/msword", "application/rtf",
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	"application/vnd.openxmlformats-officedocument.presentationml.presentation",
	"application/vnd.ms-excel", "application/vnd.ms-powerpoint",
	"application/vnd.oasis.opendocument.text",
	"application/vnd.oasis.opendocument.spreadsheet",
	"application/vnd.oasis.opendocument.presentation",
}

// OfficeExtensions stand in for a content type the provider did not send, or
// one the sniffer reduced to application/zip.
var OfficeExtensions = []string{
	".pdf", ".doc", ".docx", ".rtf", ".odt", ".xls", ".xlsx", ".ods", ".ppt", ".pptx", ".odp",
}

// TextTypes and TextExtensions are the files whose bytes are their text, such
// as a payroll export saved as CSV.
var (
	TextTypes      = []string{"text/plain", "text/csv", "text/markdown"}
	TextExtensions = []string{".csv", ".txt", ".md"}
)
