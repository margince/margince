// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package filesniff compares an upload's bytes with the media type its name or
// Content-Type claims. Both claims come from the sender, so a program renamed
// to .pdf passes a name check and fails here.
package filesniff

import (
	"bytes"
	"io"
	"mime"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/margince/margince/backend/pkg/extension"
)

// headLen is how much of the file the byte rules read.
const headLen = 1024

var (
	oleMagic  = []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}
	rtfMagic  = []byte(`{\rtf`)
	pdfMagic  = []byte("%PDF-")
	tiffLE    = []byte("II*\x00")
	tiffBE    = []byte("MM\x00*")
	elfMagic  = []byte("\x7fELF")
	jpegMagic = []byte{0xFF, 0xD8, 0xFF}
	utf8BOM   = []byte{0xEF, 0xBB, 0xBF}
)

// machO covers 32- and 64-bit images of both byte orders, plus the fat header
// that Java class files share.
var machO = [][]byte{
	{0xFE, 0xED, 0xFA, 0xCE},
	{0xFE, 0xED, 0xFA, 0xCF},
	{0xCE, 0xFA, 0xED, 0xFE},
	{0xCF, 0xFA, 0xED, 0xFE},
	{0xCA, 0xFE, 0xBA, 0xBE},
}

// file is what a rule sees: the head for the cheap checks and the whole reader
// for the archive rules.
type file struct {
	head    []byte
	content io.ReadSeeker
	charset string
}

// Contradicts reports whether the content cannot be a file of mediaType.
// charset is the declared charset parameter ("" when none). It reads the file
// and rewinds, so the caller stores the same bytes it was given. An empty file
// contradicts nothing; the upload's own empty-file refusal owns it.
func Contradicts(mediaType, charset string, content io.ReadSeeker) (bool, error) {
	head := make([]byte, headLen)
	n, err := io.ReadFull(content, head)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return false, err
	}
	if _, err := content.Seek(0, io.SeekStart); err != nil {
		return false, err
	}
	f := file{head: head[:n], content: content, charset: strings.ToLower(charset)}
	if len(f.head) == 0 {
		return false, nil
	}
	if isExecutable(f.head) {
		return true, nil
	}
	rule, ok := signatureRules[mediaType]
	if !ok {
		return false, nil
	}
	agrees := rule(f)
	if _, err := content.Seek(0, io.SeekStart); err != nil {
		return false, err
	}
	return !agrees, nil
}

// DeclaredCharset is the charset parameter of a declared Content-Type.
func DeclaredCharset(declared string) string {
	_, params, err := mime.ParseMediaType(declared)
	if err != nil {
		return ""
	}
	return params["charset"]
}

// Knows reports whether Contradicts has a rule for the media type. A type
// without a rule is never refused, so callers test their table against this.
func Knows(mediaType string) bool {
	_, ok := signatureRules[mediaType]
	return ok
}

// isExecutable refuses any file that opens with "MZ": a text file has no
// reason to, and the PE header offset is the attacker's to place.
func isExecutable(head []byte) bool {
	if bytes.HasPrefix(head, []byte("MZ")) || bytes.HasPrefix(head, elfMagic) {
		return true
	}
	for _, magic := range machO {
		if bytes.HasPrefix(head, magic) {
			return true
		}
	}
	return false
}

func prefixed(magics ...[]byte) func(file) bool {
	return func(f file) bool {
		for _, magic := range magics {
			if bytes.HasPrefix(f.head, magic) {
				return true
			}
		}
		return false
	}
}

func sniffed(mediaType string) func(file) bool {
	return func(f file) bool { return extension.SniffContentType(f.head) == mediaType }
}

func isText(f file) bool {
	if strings.HasPrefix(f.charset, "utf-16") {
		return isUTF16Text(f.head)
	}
	return strings.HasPrefix(extension.SniffContentType(f.head), "text/")
}

// isUTF16Text accepts a head that decodes as UTF-16 in either byte order
// without control characters other than whitespace. A BOM is skipped.
func isUTF16Text(head []byte) bool {
	if len(head) >= 2 && (head[0] == 0xFF && head[1] == 0xFE || head[0] == 0xFE && head[1] == 0xFF) {
		head = head[2:]
	}
	head = head[:len(head)&^1]
	for _, bigEndian := range []bool{false, true} {
		units := make([]uint16, len(head)/2)
		for i := range units {
			if bigEndian {
				units[i] = uint16(head[2*i])<<8 | uint16(head[2*i+1])
			} else {
				units[i] = uint16(head[2*i+1])<<8 | uint16(head[2*i])
			}
		}
		if plainRunes(utf16.Decode(units)) {
			return true
		}
	}
	return false
}

// plainRunes also wants a space, newline or delimiter. Random bytes often
// decode to valid UTF-16 units, but never to a word break.
func plainRunes(runes []rune) bool {
	breaks := false
	for _, r := range runes {
		if r == utf8.RuneError || r < 0x20 && !strings.ContainsRune("\t\r\n\f", r) {
			return false
		}
		breaks = breaks || strings.ContainsRune(" \t\r\n,;|", r)
	}
	return len(runes) > 0 && (breaks || len(runes) < 4)
}

// isPDF wants the marker at the start; a leading BOM or whitespace is allowed.
func isPDF(f file) bool {
	head := bytes.TrimPrefix(f.head, utf8BOM)
	return bytes.HasPrefix(bytes.TrimLeft(head, " \t\r\n\x00"), pdfMagic)
}

// heifBrands are the brands that make an ISO base media file an image rather
// than the mp4 or QuickTime video that shares its "ftyp" box.
var heifBrands = map[string]bool{
	"heic": true, "heix": true, "hevc": true, "heim": true, "heis": true,
	"hevm": true, "hevs": true, "mif1": true, "msf1": true,
}

// isHEIF reads the major brand and the compatible brands of the ftyp box.
func isHEIF(f file) bool {
	head := f.head
	if len(head) < 12 || string(head[4:8]) != "ftyp" {
		return false
	}
	size := int(head[0])<<24 | int(head[1])<<16 | int(head[2])<<8 | int(head[3])
	end := min(max(size, 12), len(head))
	if heifBrands[string(head[8:12])] {
		return true
	}
	for i := 16; i+4 <= end; i += 4 {
		if heifBrands[string(head[i:i+4])] {
			return true
		}
	}
	return false
}

// isRTF or OLE only: a Word or PowerPoint file of today is a zip and carries
// its own extension.
var isOLEOrRTF = prefixed(oleMagic, rtfMagic)

// isLegacySpreadsheet adds what Excel saves under .xls: a zip, HTML or CSV.
func isLegacySpreadsheet(f file) bool {
	return isOLEOrRTF(f) || isPlainZip(f) || isText(f)
}

// signatureRules holds, per stored media type, what the file must look like.
// Text types must sniff as text, which keeps any binary out of them.
var signatureRules = map[string]func(file) bool{
	"application/pdf":               isPDF,
	"application/rtf":               prefixed(rtfMagic),
	"application/msword":            isOLEOrRTF,
	"application/vnd.ms-powerpoint": isOLEOrRTF,
	"application/vnd.ms-excel":      isLegacySpreadsheet,
	"application/vnd.ms-outlook":    prefixed(oleMagic),
	"application/zip":               isPlainZip,
	"image/png":                     sniffed("image/png"),
	"image/jpeg":                    prefixed(jpegMagic),
	"image/gif":                     sniffed("image/gif"),
	"image/webp":                    sniffed("image/webp"),
	"image/tiff":                    prefixed(tiffLE, tiffBE),
	"image/heic":                    isHEIF,
	"image/heif":                    isHEIF,
	"text/plain":                    isText,
	"text/csv":                      isText,
	"text/markdown":                 isText,
	"text/html":                     isText,
	"message/rfc822":                isText,
	"application/json":              isText,
}

func init() {
	for mediaType, part := range ooxmlMainParts {
		signatureRules[mediaType] = ooxmlRule(part)
	}
	for mediaType := range odfTypes {
		signatureRules[mediaType] = odfRule(mediaType)
	}
}
