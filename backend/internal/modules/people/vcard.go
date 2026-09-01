// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// Reading a .vcf file — the card a person handed over, or exported from the
// address book they keep themselves.
//
// It lives in this package rather than in a shared one because there is exactly
// one caller: the import that turns a card into a person. A format package with
// one consumer is an abstraction without a second caller, and this tree's own
// rule says not to write one until the second appears.
//
// What it reads is RFC 6350 (vCard 4.0) with the 3.0 spellings every real
// exporter still emits. What it deliberately does NOT do is interpret: a
// property this file does not name is dropped rather than guessed at, and a
// card that states nothing this product stores yields an empty entry rather
// than a person assembled out of the fields that happened to parse.

import (
	"bufio"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"
)

// VCardEntry is one card, reduced to what a person record holds. Every field is
// as the card stated it — trimmed, unescaped, and otherwise untouched.
type VCardEntry struct {
	FullName string
	// Organization is the first ORG component, which is the company. The
	// components after it are departments ("Acme;Sales;EMEA"), and a
	// department is not an employer.
	Organization string
	Title        string
	Emails       []VCardChannel
	Phones       []VCardChannel
	URL          string
	Address      string
	// Revised is what the card's REV property states: when this card was last
	// changed. Nil where it states none, and the import then dates the card
	// from its own clock.
	Revised *time.Time
}

// VCardChannel is one address or number with the kind the card gave it.
type VCardChannel struct {
	Value string
	// Kind is the TYPE parameter folded onto this product's own vocabulary,
	// defaulting to work — a business card states a working address unless it
	// says otherwise.
	Kind string
}

// vcardMaxBytes bounds what one upload may be. A .vcf is a text file of a few
// hundred bytes per card; anything past this is not an address book, and
// reading it into memory to find that out is the failure this prevents.
const vcardMaxBytes = 4 << 20 // 4 MiB

// vcardSpillBytes is how much of a multipart parse stays in memory before the
// rest goes to a temp file. Far below the route's own ceiling on purpose: the
// ceiling says what may be sent, this says what may be held.
const vcardSpillBytes = 1 << 20 // 1 MiB

// vcardMaxCards bounds one upload. An address book is hundreds of cards; a file
// holding more than this is not one, and each card opens its own transaction —
// so the count is refused before the writes start rather than discovered
// halfway through them.
const vcardMaxCards = 5000

// vcardMaxLines bounds the unfolded line count. Folding joins a continuation
// onto the line before it, which copies that line, so a file of millions of
// one-character continuations costs quadratic work inside a byte budget that
// looks small. The cap is what makes the fold linear in practice.
const vcardMaxLines = 200_000

// TooManyCardsError refuses a file with more cards than one upload may carry.
type TooManyCardsError struct{ Limit int }

func (e *TooManyCardsError) Error() string {
	return fmt.Sprintf("the file holds more than %d cards", e.Limit)
}

// FieldFault carries the verdict to every surface, as the module's other
// refusals do.
func (e *TooManyCardsError) FieldFault() (field, code, message string) {
	return "file", "too_many_cards", e.Error()
}

// ParseVCards reads every card in one file.
//
// A malformed card is a parse error for the whole file rather than a card
// silently skipped: an import that quietly drops a person is worse than one
// that refuses, because the reader has no way to notice the person who is
// missing.
func ParseVCards(r io.Reader) ([]VCardEntry, error) {
	lines, err := unfoldVCardLines(io.LimitReader(r, vcardMaxBytes+1))
	if err != nil {
		return nil, err
	}
	var out []VCardEntry
	var current *VCardEntry
	for _, line := range lines {
		name, params, value := splitVCardLine(line)
		switch strings.ToUpper(name) {
		case "BEGIN":
			if strings.EqualFold(value, "VCARD") {
				current = &VCardEntry{}
			}
		case "END":
			if current != nil && strings.EqualFold(value, "VCARD") {
				out = append(out, *current)
				current = nil
			}
		default:
			if current != nil {
				applyVCardProperty(current, strings.ToUpper(name), params, value)
			}
		}
	}
	if current != nil {
		return nil, fmt.Errorf("people: the vCard file ends inside a card")
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("people: the file holds no vCard")
	}
	return out, nil
}

// unfoldVCardLines joins continuation lines, which is the first thing any vCard
// reader must do: a long value is split across lines and the continuations are
// marked by a leading space or tab, so a parser reading raw lines sees half an
// address and a fragment.
func unfoldVCardLines(r io.Reader) ([]string, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	var lines []string
	// The line under construction is built in ONE builder rather than by
	// re-appending to the slice's last element: `lines[n-1] += x` copies the
	// whole accumulated line every time, which turns a file of many short
	// continuations into quadratic work.
	var folding strings.Builder
	var open bool
	flush := func() {
		if open {
			lines = append(lines, folding.String())
			folding.Reset()
			open = false
		}
	}
	read := 0
	for scanner.Scan() {
		read += len(scanner.Bytes()) + 1
		if read > vcardMaxBytes {
			// The LimitReader above would otherwise hand back a prefix and this
			// would parse it as though it were the file, silently importing
			// part of an address book.
			return nil, fmt.Errorf("people: the vCard file exceeds %d bytes", vcardMaxBytes)
		}
		if len(lines) > vcardMaxLines {
			return nil, fmt.Errorf("people: the vCard file holds more than %d lines", vcardMaxLines)
		}
		line := strings.TrimRight(scanner.Text(), "\r")
		if line == "" {
			continue
		}
		if (line[0] == ' ' || line[0] == '\t') && open {
			folding.WriteString(line[1:])
			continue
		}
		flush()
		folding.WriteString(line)
		open = true
	}
	flush()
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("people: reading the vCard file: %w", err)
	}
	return lines, nil
}

// splitVCardLine breaks one unfolded line into its property name, its
// parameters, and its value. The name may carry a group prefix ("item1.TEL"),
// which Apple's exporter uses and which means nothing to this reader.
func splitVCardLine(line string) (name string, params []string, value string) {
	colon := indexUnquoted(line, ':')
	if colon < 0 {
		return "", nil, ""
	}
	head, value := line[:colon], line[colon+1:]
	parts := strings.Split(head, ";")
	name = parts[0]
	if dot := strings.LastIndex(name, "."); dot >= 0 {
		name = name[dot+1:]
	}
	return name, parts[1:], value
}

// indexUnquoted finds the first sep outside a quoted parameter value. A TYPE
// parameter may be quoted and may contain a colon, and splitting on the first
// colon regardless turns such a line into a property nobody named.
func indexUnquoted(s string, sep byte) int {
	quoted := false
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '"':
			quoted = !quoted
		case s[i] == sep && !quoted:
			return i
		}
	}
	return -1
}

func applyVCardProperty(entry *VCardEntry, name string, params []string, raw string) {
	value := decodeVCardValue(params, raw)
	switch name {
	case "FN":
		entry.FullName = strings.TrimSpace(value)
	case "N":
		// Only when FN said nothing: FN is the display name the person chose,
		// and N is the structured fallback for a card that omits it. Split on
		// the RAW value for the same reason ORG is: an escaped semicolon
		// inside a family name is part of the name, not a component boundary.
		if entry.FullName == "" {
			entry.FullName = nameFromStructured(params, raw)
		}
	case "ORG":
		// Split on the RAW value: unescaping first would turn an escaped
		// semicolon inside a company's own name into a component separator,
		// and "Acme; Holdings" would arrive as "Acme".
		entry.Organization = strings.TrimSpace(decodeVCardValue(params, firstComponent(raw)))
	case "TITLE":
		entry.Title = strings.TrimSpace(value)
	case "EMAIL":
		if v := strings.TrimSpace(value); v != "" {
			entry.Emails = append(entry.Emails, VCardChannel{Value: v, Kind: emailKindFrom(params)})
		}
	case "TEL":
		if v := strings.TrimSpace(value); v != "" {
			entry.Phones = append(entry.Phones, VCardChannel{Value: v, Kind: phoneKindFrom(params)})
		}
	case "URL":
		if entry.URL == "" {
			entry.URL = strings.TrimSpace(value)
		}
	case "ADR":
		if entry.Address == "" {
			entry.Address = addressFromStructured(params, raw)
		}
	case "REV":
		if entry.Revised == nil {
			entry.Revised = parseVCardRevision(value)
		}
	}
}

// parseVCardRevision reads REV, the card's own statement of when it was last
// revised, and nil when it states none or states one this reader cannot parse.
//
// It matters because a card carries no other date, and the import needs one:
// dating a card from the moment the request ran makes a re-upload of the same
// file a NEWER statement than everything since, so a retry can put back a
// detail a signature had already corrected. REV is what the card itself says,
// so importing the same file twice states the same thing twice.
//
// RFC 6350 writes REV as an ISO 8601 basic-format timestamp; the extended
// format and a bare date appear in real files, so all three are read. A card
// whose REV cannot be read falls back to the import's own clock, which is the
// behaviour every card had before this.
func parseVCardRevision(value string) *time.Time {
	v := strings.TrimSpace(value)
	if v == "" {
		return nil
	}
	// Zoned forms FIRST, and the hour-only offset among them: RFC 6350 admits
	// -05 as well as -0500, and a card carrying one would otherwise fall
	// through to the zoneless layouts, be read as UTC, and claim a time hours
	// from what it says.
	for _, layout := range []string{
		"20060102T150405Z0700", "20060102T150405Z07:00", "20060102T150405Z07",
		time.RFC3339, "2006-01-02T15:04:05Z07", "2006-01-02T15:04:05Z0700",
	} {
		if at, err := time.Parse(layout, v); err == nil {
			return &at
		}
	}
	// Zoneless forms are read as UTC, which is a GUESS — the card states a wall
	// clock and not an instant. It is the safe direction to guess in: UTC is at
	// or behind every zone east of it, so a card read this way can be older
	// than it is and lose a comparison it should have won, never newer and win
	// one it should have lost. A card that loses is a detail not corrected; a
	// card that wrongly wins overwrites a statement made after it.
	for _, layout := range []string{
		"20060102T150405", "2006-01-02T15:04:05", "20060102", "2006-01-02",
	} {
		if at, err := time.Parse(layout, v); err == nil {
			return &at
		}
	}
	return nil
}

// decodeVCardValue undoes the two encodings a real-world card carries: the
// quoted-printable of a 2.1-era exporter, and vCard's own backslash escapes.
func decodeVCardValue(params []string, raw string) string {
	value := raw
	for _, p := range params {
		upper := strings.ToUpper(p)
		if strings.Contains(upper, "QUOTED-PRINTABLE") {
			value = decodeQuotedPrintable(value)
		}
		if strings.Contains(upper, "ENCODING=B") || strings.Contains(upper, "BASE64") {
			if decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(value)); err == nil {
				value = string(decoded)
			}
		}
	}
	return unescapeVCardText(value)
}

func decodeQuotedPrintable(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		// Decoded as the BYTE it is rather than through an int: a hex pair is
		// one octet by definition, and widening it only to narrow it again is
		// an unchecked conversion standing where no conversion is needed.
		if s[i] == '=' && i+2 < len(s) {
			if decoded, err := hex.DecodeString(s[i+1 : i+3]); err == nil && len(decoded) == 1 {
				b.WriteByte(decoded[0])
				i += 2
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// unescapeVCardText undoes the escapes RFC 6350 §3.4 defines. A literal comma
// or semicolon inside a value arrives escaped, and leaving the backslash in
// would store a name nobody wrote.
func unescapeVCardText(s string) string {
	replacer := strings.NewReplacer(`\n`, "\n", `\N`, "\n", `\,`, ",", `\;`, ";", `\\`, `\`)
	return replacer.Replace(s)
}

// splitVCardComponents breaks a structured value on its UNESCAPED semicolons
// and decodes each component afterwards. Decoding first would turn an escaped
// semicolon inside one component into a separator that was never there.
func splitVCardComponents(params []string, raw string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(raw); i++ {
		if raw[i] == ';' && (i == 0 || raw[i-1] != '\\') {
			parts = append(parts, raw[start:i])
			start = i + 1
		}
	}
	parts = append(parts, raw[start:])
	for i, p := range parts {
		parts[i] = strings.TrimSpace(decodeVCardValue(params, p))
	}
	return parts
}

// firstComponent takes the part before the first unescaped semicolon.
func firstComponent(value string) string {
	for i := 0; i < len(value); i++ {
		if value[i] == ';' && (i == 0 || value[i-1] != '\\') {
			return value[:i]
		}
	}
	return value
}

// nameFromStructured reads N: family;given;additional;prefix;suffix, and puts
// it back in reading order.
func nameFromStructured(params []string, raw string) string {
	parts := splitVCardComponents(params, raw)
	var ordered []string
	// given, then family — the two components a person is addressed by. The
	// prefixes and suffixes are dropped: "Dr." is not part of a name this
	// product matches on, and carrying it makes two records of one person.
	if len(parts) > 1 && parts[1] != "" {
		ordered = append(ordered, parts[1])
	}
	if len(parts) > 0 && parts[0] != "" {
		ordered = append(ordered, parts[0])
	}
	return strings.Join(ordered, " ")
}

// addressFromStructured reads ADR: pobox;ext;street;locality;region;code;country
// into one line, dropping the components a card left empty.
func addressFromStructured(params []string, raw string) string {
	parts := splitVCardComponents(params, raw)
	var kept []string
	for i, p := range parts {
		// The first two components are a post-office box and an extended
		// address; neither belongs in the one line a record shows.
		if i < 2 {
			continue
		}
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			kept = append(kept, trimmed)
		}
	}
	return strings.Join(kept, ", ")
}

// The vCard type names this reader recognizes. A card may carry anything here
// (INTERNET, PREF, X-custom); what it does NOT carry is this product's own
// vocabulary, so an unrecognized type folds onto `other` rather than being
// guessed at work.
const (
	vcardTypeHome    = "home"
	vcardTypeWork    = "work"
	vcardTypeCell    = "cell"
	vcardTypeMobile  = "mobile"
	channelKindOther = "other"
)

func emailKindFrom(params []string) string {
	switch typeParam(params) {
	case vcardTypeHome:
		return "personal"
	case vcardTypeWork, "":
		// A business card states a working address unless it says otherwise.
		return emailTypeWork
	default:
		return channelKindOther
	}
}

func phoneKindFrom(params []string) string {
	switch typeParam(params) {
	case vcardTypeCell, vcardTypeMobile:
		return vcardTypeMobile
	case vcardTypeHome:
		return vcardTypeHome
	case vcardTypeWork, "":
		return phoneTypeWork
	default:
		return channelKindOther
	}
}

// typeParam reads the first TYPE value, lowercased. A card may list several
// (TYPE=work,voice,pref); the first is the one that names the kind.
func typeParam(params []string) string {
	for _, p := range params {
		upper := strings.ToUpper(p)
		if !strings.HasPrefix(upper, "TYPE=") {
			// vCard 2.1 writes a bare type ("TEL;WORK:..."), with no TYPE=.
			if bare := strings.Trim(strings.ToLower(p), `"`); isKnownVCardType(bare) {
				return bare
			}
			continue
		}
		values := strings.Split(p[len("TYPE="):], ",")
		if len(values) > 0 {
			first := strings.Trim(strings.ToLower(strings.TrimSpace(values[0])), `"`)
			// PREF says which is primary, not what kind it is: read past it,
			// and when it is the ONLY value the card has named no kind — which
			// is absence, not an unknown kind, so the caller's default applies.
			if first == "pref" {
				if len(values) == 1 {
					continue
				}
				return strings.Trim(strings.ToLower(strings.TrimSpace(values[1])), `"`)
			}
			return first
		}
	}
	return ""
}

func isKnownVCardType(v string) bool {
	switch v {
	case vcardTypeWork, vcardTypeHome, vcardTypeCell, vcardTypeMobile, "fax", "voice":
		return true
	default:
		return false
	}
}
