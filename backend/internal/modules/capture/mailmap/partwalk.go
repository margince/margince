// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailmap

// The walk over a message's MIME tree. A part go-message cannot decode is
// still a part, and its siblings are read. Its own bytes are decoded leniently
// where the header's intent is plain.

import (
	"bytes"
	"encoding/base64"
	"errors"
	"io"
	"mime/quotedprintable"
	"strings"

	"github.com/emersion/go-message"
	"github.com/emersion/go-message/mail"

	"github.com/margince/margince/backend/internal/modules/capture"
)

// forwardedType is a whole message carried as one part: a forward as attachment.
const forwardedType = "message/rfc822"

// The transfer encodings go-message decodes (encodingReader in v0.18.2).
const (
	encodingQuotedPrintable = "quoted-printable"
	encodingBase64          = "base64"
	encoding7bit            = "7bit"
	encoding8bit            = "8bit"
	encodingBinary          = "binary"
)

// errWalkBounded stops message.Entity.Walk once maxPartsExamined is reached.
var errWalkBounded = errors.New("mailmap: part walk bound reached")

// partWalker is what one walk over a message's parts has learned so far.
type partWalker struct {
	files *collector
	// textOnly walks a forwarded original for its text alone. Its files were
	// already kept once, as the forwarded part itself.
	textOnly    bool
	plain, html string
	calendar    calendarPart
	examined    int
	// textLost is a body text part not read in full: its transfer encoding
	// could not be undone, or it ran past the per-part limit. opaque is a body
	// only a mail program can open.
	textLost  bool
	opaque    bool
	forwarded []byte
}

// readEntity reads a message's root. An unknown transfer encoding or charset
// still leaves a readable entity; the walk decodes it from its header.
func readEntity(raw []byte) (*message.Entity, error) {
	entity, err := message.Read(bytes.NewReader(raw))
	if err != nil && !message.IsUnknownEncoding(err) && !message.IsUnknownCharset(err) {
		return nil, err
	}
	return entity, nil
}

func (w *partWalker) run(root *message.Entity) {
	err := root.Walk(w.visit)
	if err != nil && !errors.Is(err, errWalkBounded) {
		// A structural failure ends the walk, so any file after this point is
		// lost. Recorded, so a message whose files went missing does not read
		// like one that carried none (DOC-AC-12).
		w.files.truncated()
	}
}

// visit takes one entity of the tree. The error Walk passes is only ever an
// unknown encoding or charset. The header alone decides how to decode, because
// go-message reports a charset error in place of an encoding error.
func (w *partWalker) visit(_ []int, entity *message.Entity, _ error) error {
	// Counted over every entity, containers and unnamed text included. Parts
	// cost 25 bytes each on the wire, so without this bound a sender chooses
	// how long we walk.
	if w.examined >= maxPartsExamined {
		w.files.truncated()
		return errWalkBounded
	}
	w.examined++
	mediaType, params, _ := entity.Header.ContentType()
	if strings.HasPrefix(mediaType, "multipart/") {
		return nil
	}
	if opaqueBody(entity.Header, mediaType, params) {
		w.opaque = true
	}
	body, decoded := transferDecoded(entity)
	if !isInlinePart(entity.Header, mediaType) {
		w.attachment(&mail.AttachmentHeader{Header: entity.Header}, mediaType, body)
		return nil
	}
	inline := &mail.InlineHeader{Header: entity.Header}
	if inlineFilename(inline) != "" || !isBodyTextType(mediaType) {
		w.inline(inline, mediaType, body)
		return nil
	}
	watched := &decodeWatch{r: body}
	readInlinePart(inline, watched, w.files, &w.plain, &w.html, &w.calendar)
	if !decoded || watched.err != nil || readPastLimit(watched) {
		w.textLost = true
	}
	return nil
}

// attachment takes a part the sender attached, a forwarded original among them.
func (w *partWalker) attachment(part *mail.AttachmentHeader, mediaType string, body io.Reader) {
	if w.textOnly {
		return
	}
	w.files.take(part, body)
	if mediaType == forwardedType {
		w.holdForwarded(w.files.justAdmitted())
	}
	if params, declared := declaresCalendar(part); declared {
		w.calendar.see(params, w.files.justAdmitted())
	}
}

// inline takes an inline part that is not body text: a named file, a calendar
// payload or a forwarded original.
func (w *partWalker) inline(part *mail.InlineHeader, mediaType string, body io.Reader) {
	named := inlineFilename(part) != ""
	if mediaType == forwardedType && !named {
		if !w.textOnly {
			w.holdForwarded(readBoundedRaw(body))
		}
		return
	}
	if w.textOnly && (named || isCalendarType(mediaType)) {
		return
	}
	readInlinePart(part, body, w.files, &w.plain, &w.html, &w.calendar)
	if mediaType == forwardedType {
		w.holdForwarded(w.files.justAdmitted())
	}
}

// holdForwarded keeps the first forwarded original, whose text stands in for
// the body when the message itself says nothing. The original's own forwards
// are not opened: one level is a forward, more is a sender buying work.
func (w *partWalker) holdForwarded(content []byte) {
	if !w.textOnly && w.forwarded == nil && len(content) > 0 {
		w.forwarded = content
	}
}

// readForwarded walks the held original for its text. nil when there is none
// or it cannot be read as a message.
func (w *partWalker) readForwarded() *partWalker {
	if w.forwarded == nil {
		return nil
	}
	root, err := readEntity(w.forwarded)
	if err != nil {
		return nil
	}
	inner := &partWalker{files: newCollector(), textOnly: true}
	inner.run(root)
	return inner
}

// completeness says how much of the mail's text was read. It is none when the
// text is empty and the walk saw a reason. The reasons are an early stop, a
// text part not read in full, and an opaque body. It is part when the walk
// stopped early, or the HTML render was cut. It is part, too, when any text part
// was not read in full, even one bodyText did not pick. An alternative is no
// proof that a lost part said the same. A file alone is complete.
func (w *partWalker) completeness() capture.BodyCompleteness {
	text, renderCut := bodyText(w.plain, w.html)
	switch {
	case text == "" && (w.files.walkTruncated() || w.textLost || w.opaque):
		return capture.BodyUnreadable
	case text != "" && (w.files.walkTruncated() || w.textLost || renderCut):
		return capture.BodyPartial
	}
	return capture.BodyComplete
}

func (w *partWalker) text() string {
	text, _ := bodyText(w.plain, w.html)
	return text
}

// readPastLimit reports a body text part that had more bytes than readBounded
// took. Only reached after the read, so it costs one byte.
func readPastLimit(body io.Reader) bool {
	var next [1]byte
	n, err := body.Read(next[:])
	if err != nil && !errors.Is(err, io.EOF) {
		// More was there to read, or the read would not have failed.
		return true
	}
	return n > 0
}

// isInlinePart classifies a leaf as inline or attachment, by the rule
// mail.Reader.NextPart applies in go-message v0.18.2.
func isInlinePart(header message.Header, mediaType string) bool {
	disposition, _, _ := header.ContentDisposition()
	return disposition == "inline" || (disposition != "attachment" && strings.HasPrefix(mediaType, "text/"))
}

func isBodyTextType(mediaType string) bool {
	return mediaType == "text/plain" || mediaType == "text/html"
}

// opaqueBody reports a part whose content only a mail program can open: a TNEF
// container (winmail.dat), or an S/MIME envelope that carries the body. A
// signature, a certificate or an S/MIME file sent as an attachment is not one.
func opaqueBody(header message.Header, mediaType string, params map[string]string) bool {
	switch mediaType {
	case "application/ms-tnef", "application/vnd.ms-tnef":
		return true
	case "application/pkcs7-mime", "application/x-pkcs7-mime":
		disposition, _, _ := header.ContentDisposition()
		smime := strings.ToLower(params["smime-type"])
		return disposition != "attachment" && (smime == "" || smime == "enveloped-data")
	}
	return false
}

// transferDecoded answers the part's body with its transfer encoding undone.
// go-message has already decoded the values it knows. For any other value its
// body is still encoded, and a value that is a known one misspelled
// (`"base64"`, `8bit;`, `7-bit`) is decoded here. decoded is false when the
// value stays unknown and the bytes are passed on as they arrived.
func transferDecoded(entity *message.Entity) (body io.Reader, decoded bool) {
	declared := entity.Header.Get("Content-Transfer-Encoding")
	switch strings.ToLower(declared) {
	case encodingQuotedPrintable, encodingBase64, encoding7bit, encoding8bit, encodingBinary, "":
		return entity.Body, true
	}
	switch lenientEncoding(declared) {
	case encodingQuotedPrintable:
		return quotedprintable.NewReader(entity.Body), true
	case encodingBase64:
		return base64.NewDecoder(base64.StdEncoding, &unfoldedBase64{r: entity.Body}), true
	case encoding7bit, encoding8bit, encodingBinary:
		return entity.Body, true
	}
	return entity.Body, false
}

// lenientEncoding normalises the misspellings of a transfer encoding that mail
// programs send: quotes, a trailing semicolon, a hyphenated bit width.
func lenientEncoding(declared string) string {
	value := strings.TrimSpace(strings.ToLower(declared))
	value = strings.TrimSpace(strings.TrimSuffix(value, ";"))
	value = strings.TrimSpace(strings.Trim(value, `"'`))
	switch value {
	case "7-bit":
		return encoding7bit
	case "8-bit":
		return encoding8bit
	}
	return value
}

// unfoldedBase64 turns the spaces and tabs of a folded base64 body into line
// breaks, which the decoder skips. go-message does the same for the value it knows.
type unfoldedBase64 struct{ r io.Reader }

func (u *unfoldedBase64) Read(p []byte) (int, error) {
	n, err := u.r.Read(p)
	for i := range p[:n] {
		if p[i] == ' ' || p[i] == '\t' {
			p[i] = '\n'
		}
	}
	return n, err
}

// decodeWatch ends a body text part at its first decode error and remembers
// it, so the text decoded before the error is kept.
type decodeWatch struct {
	r   io.Reader
	err error
}

func (d *decodeWatch) Read(p []byte) (int, error) {
	n, err := d.r.Read(p)
	if err != nil && !errors.Is(err, io.EOF) {
		d.err = err
		return n, io.EOF
	}
	return n, err
}
