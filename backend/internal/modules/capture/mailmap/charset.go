// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailmap

// Legacy charsets (ISO-8859-1, Windows-1252 and the rest) decoded to UTF-8 for
// the text the record keeps: header words and the body part read as text.
//
// go-message's own charset package is not used. It installs a global decoder
// for every text/* entity, attachments included. A stored file must keep the
// exact bytes the sender sent.

import (
	"fmt"
	"io"
	"mime"
	netmail "net/mail"
	"regexp"

	"github.com/emersion/go-message"
	"github.com/emersion/go-message/mail"
	"golang.org/x/net/html/charset"
)

// headerWords decodes RFC 2047 encoded words in any charset the WHATWG
// encoding list names, not only the UTF-8 and ISO-8859-1 the stdlib knows.
var headerWords = &mime.WordDecoder{CharsetReader: legacyCharsetReader}

func legacyCharsetReader(label string, input io.Reader) (io.Reader, error) {
	encoding, _ := charset.Lookup(label)
	if encoding == nil {
		return nil, fmt.Errorf("mailmap: unsupported charset %q", label)
	}
	return encoding.NewDecoder().Reader(input), nil
}

// stdlibDecodedWord matches the encoded words the stdlib decodes itself and
// never hands to legacyCharsetReader. Its ISO-8859-1 maps 0x80-0x9F to control
// characters, where mail clients mean Windows-1252 quotes and dashes.
var stdlibDecodedWord = regexp.MustCompile(`(?i)=\?(?:iso-8859-1|us-ascii)\?`)

// asWindows1252Words relabels those words, so the WHATWG table decodes them as
// it decodes a body part in the same charset.
func asWindows1252Words(raw string) string {
	return stdlibDecodedWord.ReplaceAllLiteralString(raw, "=?windows-1252?")
}

// headerText is one unstructured header field with its encoded words decoded.
// A word in an unknown charset leaves the field as it arrived.
func headerText(header mail.Header, key string) string {
	raw := header.Get(key)
	decoded, err := headerWords.DecodeHeader(asWindows1252Words(raw))
	if err != nil {
		return raw
	}
	return decoded
}

// addressList parses an address header with display names decoded through
// headerWords. go-message's Header.AddressList decodes only the charsets the
// stdlib knows, so a Windows-1252 sender name would stay encoded.
func addressList(header mail.Header, key string) ([]*mail.Address, error) {
	raw := header.Get(key)
	if raw == "" {
		return nil, nil
	}
	parser := netmail.AddressParser{WordDecoder: headerWords}
	return parser.ParseList(asWindows1252Words(raw))
}

// toUTF8 decodes a text body part from the charset its Content-Type declared.
// go-message has already passed UTF-8 and US-ASCII through, so those labels,
// an absent one and one no decoder knows return the bytes unchanged.
func toUTF8(content []byte, label string) []byte {
	encoding, name := charset.Lookup(label)
	if encoding == nil || name == "utf-8" {
		return content
	}
	decoded, err := encoding.NewDecoder().Bytes(content)
	if err != nil {
		// A decoder that refuses its input leaves the part as it arrived.
		return content
	}
	return decoded
}

// usablePart reports whether a go-message error still left a readable entity.
// An unknown charset is that case: the entity arrives undecoded, which is what
// an attachment needs and what toUTF8 handles for body text.
func usablePart(err error) bool {
	return err == nil || message.IsUnknownCharset(err)
}
