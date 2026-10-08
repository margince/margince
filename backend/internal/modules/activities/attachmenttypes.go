// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The kinds of file an upload may carry. One table for every door that files a
// document, so a contact uploading in the app and an agent are refused the same files.

import (
	"fmt"
	"mime"
	"path"
	"slices"
	"strings"
)

// attachmentType pairs an extension with the media type stored for it, and the
// other spellings a browser sends for that type.
type attachmentType struct {
	extension, mediaType string
	aliases              []string
}

// SVG is left out because an image viewer runs its script, and executables because
// nobody files one as a record. HTML and zip are kept: they are only ever downloaded.
var attachmentTypes = []attachmentType{
	{extension: ".pdf", mediaType: "application/pdf"},
	{extension: ".doc", mediaType: "application/msword"},
	{extension: ".docx", mediaType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
	{extension: ".xls", mediaType: "application/vnd.ms-excel"},
	{extension: ".xlsx", mediaType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
	{extension: ".ppt", mediaType: "application/vnd.ms-powerpoint"},
	{extension: ".pptx", mediaType: "application/vnd.openxmlformats-officedocument.presentationml.presentation"},
	{extension: ".odt", mediaType: "application/vnd.oasis.opendocument.text"},
	{extension: ".ods", mediaType: "application/vnd.oasis.opendocument.spreadsheet"},
	{extension: ".odp", mediaType: "application/vnd.oasis.opendocument.presentation"},
	{extension: ".rtf", mediaType: "application/rtf", aliases: []string{"text/rtf"}},
	{extension: ".txt", mediaType: "text/plain"},
	{extension: ".csv", mediaType: "text/csv", aliases: []string{"application/csv", "text/x-csv"}},
	{extension: ".md", mediaType: "text/markdown", aliases: []string{"text/x-markdown"}},
	{extension: ".png", mediaType: "image/png"},
	{extension: ".jpg", mediaType: "image/jpeg", aliases: []string{"image/jpg"}},
	{extension: ".jpeg", mediaType: "image/jpeg", aliases: []string{"image/jpg"}},
	{extension: ".gif", mediaType: "image/gif"},
	{extension: ".webp", mediaType: "image/webp"},
	{extension: ".heic", mediaType: "image/heic"},
	{extension: ".heif", mediaType: "image/heif"},
	{extension: ".tif", mediaType: "image/tiff"},
	{extension: ".tiff", mediaType: "image/tiff"},
	{extension: ".html", mediaType: "text/html"},
	{extension: ".htm", mediaType: "text/html"},
	{extension: ".zip", mediaType: "application/zip", aliases: []string{"application/x-zip-compressed"}},
	{extension: ".eml", mediaType: "message/rfc822"},
	{extension: ".msg", mediaType: "application/vnd.ms-outlook"},
}

const acceptedAttachmentKinds = "PDF, Word, Excel, PowerPoint, OpenDocument, RTF, text, CSV, Markdown, " +
	"HTML, PNG, JPEG, GIF, WebP, HEIC, HEIF and TIFF images, zip archives, and saved emails (.eml, .msg)"

// AcceptedAttachmentExtensions lists every extension an upload may carry, in
// table order, for the frontend mirror's gate.
func AcceptedAttachmentExtensions() []string {
	out := make([]string, 0, len(attachmentTypes))
	for _, kind := range attachmentTypes {
		out = append(out, kind.extension)
	}
	return out
}

// resolveAttachmentType answers the stored media type. A declared type must be
// in the table; an extension in the table then names the type, since Windows
// declares a .csv as Excel, and otherwise the declared type stands: a stored
// file is only ever downloaded. An empty or octet-stream type defers to the name.
func resolveAttachmentType(declared, typedName, shownName string) (string, error) {
	refusal := &UnsupportedFileTypeError{Filename: shownName}
	declaredType, parsed := normalizeMediaType(declared)
	if !parsed {
		return "", refusal
	}
	named, extensionKnown := kindForExtension(strings.ToLower(path.Ext(typedName)))
	if declaredType == "" {
		if !extensionKnown {
			return "", refusal
		}
		return named.mediaType, nil
	}
	kind, known := kindForMediaType(declaredType)
	switch {
	case !known:
		return "", refusal
	case extensionKnown:
		return named.mediaType, nil
	default:
		return kind.mediaType, nil
	}
}

// normalizeMediaType lowercases a declared type and drops its parameters; an
// empty or octet-stream type says nothing and comes back empty. False means
// the declared value is not a media type at all.
func normalizeMediaType(declared string) (string, bool) {
	if strings.TrimSpace(declared) == "" {
		return "", true
	}
	parsed, _, err := mime.ParseMediaType(declared)
	if err != nil {
		return "", false
	}
	parsed = strings.ToLower(parsed)
	if parsed == "application/octet-stream" {
		return "", true
	}
	return parsed, true
}

func kindForExtension(extension string) (attachmentType, bool) {
	for _, kind := range attachmentTypes {
		if kind.extension == extension {
			return kind, true
		}
	}
	return attachmentType{}, false
}

func kindForMediaType(mediaType string) (attachmentType, bool) {
	for _, kind := range attachmentTypes {
		if kind.mediaType == mediaType || slices.Contains(kind.aliases, mediaType) {
			return kind, true
		}
	}
	return attachmentType{}, false
}

// UnsupportedFileTypeError refuses an upload whose type is not in the table.
// It maps to 422: the caller fixes it by choosing a different file.
type UnsupportedFileTypeError struct{ Filename string }

func (e *UnsupportedFileTypeError) Error() string {
	return fmt.Sprintf("%q is not a kind of file that can be attached; choose one of: %s",
		e.Filename, acceptedAttachmentKinds)
}

// FieldFault names the part of the upload the caller must correct.
func (e *UnsupportedFileTypeError) FieldFault() (field, code, message string) {
	return "file", "unsupported_file_type", e.Error()
}
