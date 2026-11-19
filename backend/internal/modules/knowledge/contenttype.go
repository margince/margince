// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package knowledge

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// What this product will read, and what it says when it will not.
//
// Kept apart from the upload itself because the accepted set is a PRODUCT
// decision that changes on its own schedule — a new reader lands here and
// nowhere else — while the upload around it is a write shape that does not.

// acceptedContentTypes lists the types whose bytes are their text. A PDF taken
// on these terms would file its binary envelope as prose and answer nothing.
//
// platform/pdftext is not used here: a corpus is retrieved from unchecked, so a
// bad extraction becomes a cited passage the source does not hold.
var acceptedContentTypes = []string{
	"text/plain",
	"text/markdown",
	"text/csv",
	"application/json",
}

// UnsupportedTypeError is a file whose bytes are not its own text. It carries
// no field name: the refusal is about the file, and the transport maps it to
// 415 rather than to a validation error on a form field.
//
// Kind is the file's kind as its owner would say it ("Word"), or empty when
// nothing names one. A media type is not a name an uploader recognises.
type UnsupportedTypeError struct {
	Kind string
}

func (e *UnsupportedTypeError) Error() string {
	kind := "Files of this kind are"
	if e.Kind != "" {
		kind = e.Kind + " files are"
	}
	return kind + " not supported. Save it as " + acceptedKindsInWords + " first."
}

// acceptedKindsInWords is acceptedContentTypes as an uploader names them.
const acceptedKindsInWords = "plain text, Markdown, CSV or JSON"

// The kinds an uploader names an office file by.
const (
	kindWord       = "Word"
	kindExcel      = "Excel"
	kindPowerPoint = "PowerPoint"
)

// officeKinds name the files whose kind is not their own letters: nobody calls a
// .docx a "DOCX file". Keyed by ending and by media type, so either one names it.
var officeKinds = map[string]string{
	"doc":                kindWord,
	"docx":               kindWord,
	"xls":                kindExcel,
	"xlsx":               kindExcel,
	"ppt":                kindPowerPoint,
	"pptx":               kindPowerPoint,
	"application/msword": kindWord,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": kindWord,
	"application/vnd.ms-excel": kindExcel,
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         kindExcel,
	"application/vnd.ms-powerpoint":                                             kindPowerPoint,
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": kindPowerPoint,
}

// maxKindEnding bounds the ending quoted back as a kind. A longer one is not a
// file type anyone would recognise, and the name is the caller's own text.
const maxKindEnding = 5

// refusedKind names a refused file's kind from its ending, then its derived
// media type. The caller's raw Content-Type is never quoted back.
func refusedKind(media, filename string) string {
	_, ending, _ := strings.CutLast(strings.ToLower(filename), ".")
	if kind, ok := officeKinds[ending]; ok {
		return kind
	}
	if kind, ok := officeKinds[media]; ok {
		return kind
	}
	if ending == "" || len(ending) > maxKindEnding || strings.ContainsFunc(ending, notAlphanumeric) {
		return ""
	}
	return strings.ToUpper(ending)
}

func notAlphanumeric(r rune) bool {
	return (r < 'a' || r > 'z') && (r < '0' || r > '9')
}

// EmptyDocumentError is an upload carrying no bytes. Filed, it would read to
// its uploader like success while answering nothing. It maps to 422 on the file.
// activities.EmptyUploadError is its twin; a module never imports a sibling.
type EmptyDocumentError struct{}

func (e *EmptyDocumentError) Error() string {
	return "The file is empty, so there is nothing to add. Choose a file that has content."
}

// FieldFault names the part of the upload the caller must correct.
func (e *EmptyDocumentError) FieldFault() (field, code, message string) {
	return "file", "empty_file", e.Error()
}

// AlreadyFiledError is a file whose bytes are already in this corpus.
//
// It names the document that holds them, because the useful answer to "why was
// my upload refused" is "you already uploaded it, it is called X". The name is
// empty only when two identical uploads raced and the index refused the second.
type AlreadyFiledError struct {
	Filename string
}

func (e *AlreadyFiledError) Error() string {
	if e.Filename == "" {
		// The name is unknown only on the racing path, where the index refused
		// the second of two simultaneous uploads and the transaction that would
		// have read it is already aborted.
		return "these exact bytes are already filed in this set"
	}
	return fmt.Sprintf("these exact bytes are already filed in this set as %q", e.Filename)
}

// ErrBlobstoreUnconfigured is an upload arriving at an installation with no
// object storage bound. It is the deployment's fault, not the caller's, and
// answering it as a validation failure would send them off to fix their file.
var ErrBlobstoreUnconfigured = errors.New("knowledge: object storage is not configured; a document cannot be stored")

// acceptedType decides the media type an upload is filed under. An ending this
// route reads wins over the stated type, as an attachment's does: a browser
// sends `.csv` as an Excel type and `.md` as `text/x-markdown` or nothing. The
// bytes are still sniffed against the type, so a renamed binary is refused.
func acceptedType(contentType, filename string) (string, bool) {
	if named := typeByExtension(filename); named != "" {
		return named, true
	}
	media := strings.ToLower(strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0]))
	return media, slices.Contains(acceptedContentTypes, media)
}

// typeByExtension names the media type a file ending implies, or "" for one
// this route does not read.
func typeByExtension(filename string) string {
	_, after, ok := strings.CutLast(filename, ".")
	if !ok {
		return ""
	}
	switch strings.ToLower(after) {
	case "md", "markdown":
		return "text/markdown"
	case "txt":
		return "text/plain"
	case "csv":
		return "text/csv"
	case "json":
		return "application/json"
	default:
		return ""
	}
}
