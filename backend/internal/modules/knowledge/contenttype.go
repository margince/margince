// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package knowledge

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/margince/margince/backend/pkg/extension"
)

// What this product will read, and what it says when it will not.
//
// Kept apart from the upload itself because the accepted set is a PRODUCT
// decision that changes on its own schedule — a new reader lands here and
// nowhere else — while the upload around it is a write shape that does not.

// acceptedContentTypes is the whole list, and it is short for one reason: this
// product has no document parser. A type is here only when the file's bytes ARE
// its text, so accepting a PDF would mean ingesting its binary envelope as
// prose — a corpus that answers nothing while reporting a successful upload.
//
// The refusal names this list, because "unsupported" alone leaves the uploader
// with nothing to do.
var acceptedContentTypes = []string{
	"text/plain",
	"text/markdown",
	"text/csv",
	"application/json",
}

// UnsupportedTypeError is a file whose bytes are not its own text. It carries
// no field name: the refusal is about the file, and the transport maps it to
// 415 rather than to a validation error on a form field.
type UnsupportedTypeError struct {
	Got string
}

func (e *UnsupportedTypeError) Error() string {
	return fmt.Sprintf("%s cannot be read as text; the accepted types are %s",
		clampEchoedType(e.Got), strings.Join(acceptedContentTypes, ", "))
}

// maxEchoedType bounds what of the caller's own Content-Type is quoted back.
// A media type is a short token; anything longer is not one, and echoing an
// unbounded caller-chosen string into a problem body and the operator log is a
// gift to whoever sends a megabyte of it.
const maxEchoedType = 64

func clampEchoedType(got string) string {
	runes := []rune(got)
	if len(runes) <= maxEchoedType {
		return got
	}
	return string(runes[:maxEchoedType]) + "…"
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

// acceptedType decides the media type an upload is filed under.
//
// It normalises a browser's Content-Type (which carries parameters —
// `text/markdown; charset=utf-8`) down to the media type the list names, and
// falls back to the FILENAME's extension when the client sent none.
//
// The fallback is not a nicety. A platform with no registry entry for `.md`
// makes the browser report an EMPTY type, and this route's flagship file would
// then be refused with "` ` cannot be read as text" — a refusal naming nothing,
// for exactly the file the route wants. A curl caller that omits the part
// header hits the same wall. It runs HERE rather than in the browser so every
// client gets it, and only when the client said NOTHING: a stated type is the
// client's claim and stands, wrong extension or not.
//
// An extension is still only a name, which is why it decides nothing else. It
// picks a label from a closed list; the bytes are never parsed on the strength
// of it, because there is no parser here at all.
func acceptedType(contentType, filename string) (string, bool) {
	media := strings.ToLower(strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0]))
	if media == "" || media == "application/octet-stream" {
		media = typeByExtension(filename)
	}
	return media, slices.Contains(acceptedContentTypes, media)
}

// typeByExtension names the media type a file ending implies, or "" for one
// this route does not read.
func typeByExtension(filename string) string {
	dot := strings.LastIndex(filename, ".")
	if dot < 0 {
		return ""
	}
	switch strings.ToLower(filename[dot+1:]) {
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

// refusedTypeName is what a 415 calls the file: the DERIVED media type when
// there is one, and the caller's own words when there is not.
//
// An extension this route does not read resolves to an empty media type, and a
// refusal reading "` ` cannot be read as text" names nothing at all — worse
// than useless to whoever sent a .docx.
// in.Filename has already been through SafeFilename by the time this is
// reached. in.ContentType has NOT — it is a header the caller controls and this
// is the only place it is echoed back — so it goes through the same normaliser
// here rather than being quoted raw.
func refusedTypeName(media string, in NewDocument) string {
	if media != "" {
		return media
	}
	if stated := extension.SafeFilename(strings.TrimSpace(in.ContentType), 0); stated != "" {
		return stated
	}
	return in.Filename
}
