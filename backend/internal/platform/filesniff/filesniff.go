// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package filesniff compares an upload's first bytes with the media type its
// name or Content-Type claims. Both claims come from the sender, so a program
// renamed to .pdf passes a name check and fails here.
package filesniff

import (
	"bytes"
	"encoding/binary"
	"io"
	"strings"

	"github.com/margince/margince/backend/pkg/extension"
)

// headLen covers the PDF rule (the marker may sit within the first 1024 bytes)
// and the PE header offset check.
const headLen = 1024

var (
	zipMagic  = []byte("PK\x03\x04")
	zipEmpty  = []byte("PK\x05\x06")
	oleMagic  = []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}
	rtfMagic  = []byte(`{\rtf`)
	pdfMagic  = []byte("%PDF-")
	tiffLE    = []byte("II*\x00")
	tiffBE    = []byte("MM\x00*")
	elfMagic  = []byte("\x7fELF")
	peMagic   = []byte("PE\x00\x00")
	jpegMagic = []byte{0xFF, 0xD8, 0xFF}
)

// machO covers 32- and 64-bit images of both byte orders, plus the fat header
// that Java class files share.
var machO = [][]byte{
	{0xFE, 0xED, 0xFA, 0xCE}, {0xFE, 0xED, 0xFA, 0xCF},
	{0xCE, 0xFA, 0xED, 0xFE}, {0xCF, 0xFA, 0xED, 0xFE},
	{0xCA, 0xFE, 0xBA, 0xBE},
}

// Contradicts reports whether the content cannot be a file of mediaType. It
// reads the head and rewinds, so the caller stores the same bytes it was given.
// An empty file contradicts nothing; the upload's own empty-file refusal owns it.
func Contradicts(mediaType string, content io.ReadSeeker) (bool, error) {
	head := make([]byte, headLen)
	n, err := io.ReadFull(content, head)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return false, err
	}
	if _, err := content.Seek(0, io.SeekStart); err != nil {
		return false, err
	}
	return headContradicts(mediaType, head[:n]), nil
}

// Knows reports whether Contradicts has a rule for the media type. A type
// without a rule is never refused, so callers test their table against this.
func Knows(mediaType string) bool {
	_, ok := signatureRules[mediaType]
	return ok
}

func headContradicts(mediaType string, head []byte) bool {
	if len(head) == 0 {
		return false
	}
	if isExecutable(head) {
		return true
	}
	rule, ok := signatureRules[mediaType]
	return ok && !rule(head)
}

func isExecutable(head []byte) bool {
	if bytes.HasPrefix(head, elfMagic) {
		return true
	}
	for _, magic := range machO {
		if bytes.HasPrefix(head, magic) {
			return true
		}
	}
	if len(head) >= 0x40 && head[0] == 'M' && head[1] == 'Z' {
		offset := int(binary.LittleEndian.Uint32(head[0x3C:]))
		return offset >= 0 && offset+len(peMagic) <= len(head) && bytes.Equal(head[offset:offset+len(peMagic)], peMagic)
	}
	return false
}

func prefixed(magics ...[]byte) func([]byte) bool {
	return func(head []byte) bool {
		for _, magic := range magics {
			if bytes.HasPrefix(head, magic) {
				return true
			}
		}
		return false
	}
}

func sniffed(mediaType string) func([]byte) bool {
	return func(head []byte) bool { return extension.SniffContentType(head) == mediaType }
}

func isText(head []byte) bool {
	return strings.HasPrefix(extension.SniffContentType(head), "text/")
}

func isPDF(head []byte) bool { return bytes.Contains(head, pdfMagic) }

// isISOBMFF matches the "ftyp" box that HEIC and HEIF files open with.
func isISOBMFF(head []byte) bool { return len(head) >= 12 && string(head[4:8]) == "ftyp" }

// legacyOffice accepts an OLE file, a zip, RTF or text under an old extension.
// Windows saves a CSV or an HTML table as .xls, and Word saves RTF as .doc.
func legacyOffice(head []byte) bool {
	return prefixed(oleMagic, zipMagic, zipEmpty, rtfMagic)(head) || isText(head)
}

var zipBased = prefixed(zipMagic, zipEmpty)

// signatureRules holds, per stored media type, what its first bytes must look
// like. Text types must sniff as text, which keeps any binary out of them.
var signatureRules = map[string]func([]byte) bool{
	"application/pdf":    isPDF,
	"application/rtf":    prefixed(rtfMagic),
	"application/msword": legacyOffice, "application/vnd.ms-excel": legacyOffice,
	"application/vnd.ms-powerpoint":                                             legacyOffice,
	"application/vnd.ms-outlook":                                                prefixed(oleMagic),
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document":   zipBased,
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         zipBased,
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": zipBased,
	"application/vnd.oasis.opendocument.text":                                   zipBased,
	"application/vnd.oasis.opendocument.spreadsheet":                            zipBased,
	"application/vnd.oasis.opendocument.presentation":                           zipBased,
	"application/zip":  zipBased,
	"image/png":        sniffed("image/png"),
	"image/jpeg":       prefixed(jpegMagic),
	"image/gif":        sniffed("image/gif"),
	"image/webp":       sniffed("image/webp"),
	"image/tiff":       prefixed(tiffLE, tiffBE),
	"image/heic":       isISOBMFF,
	"image/heif":       isISOBMFF,
	"text/plain":       isText,
	"text/csv":         isText,
	"text/markdown":    isText,
	"text/html":        isText,
	"message/rfc822":   isText,
	"application/json": isText,
}
