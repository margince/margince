// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// What a refused tool call says back to the model, and what it may not.
//
// A tool result is read by the model that wrote the arguments, and an agent
// run's transcript is CUMULATIVE — every later prompt of the run carries this
// text. So a refusal here has two obligations the REST twin does not: it must
// name only what the caller authored or what we authored ourselves, never the
// program in between (T2), and the caller-authored half must be bounded and
// escaped, because its author is the same model the text is fed back to.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// var-checked here rather than left implicit: a signature typo on the method
// below would otherwise compile clean and simply never match in
// httperr.Classify, which is a silent 500 nobody would connect to this file.
var _ apperrors.FieldFaults = (*BadArgsError)(nil)

// decodeArgs is the surface's input validation: strict JSON — unknown argument
// names are errors rather than silent drops, and the arguments are exactly ONE
// JSON value rather than the first of several.
//
// The unknown-key half is datasource.RejectNonCanonicalKeys, the same gate the
// REST body decode and the provider seam apply, for two reasons. It refuses a
// key that only case-FOLDS onto an argument, which encoding/json would otherwise
// accept — so `{"LIMIT":1}` is a field patch on no surface rather than on this
// one alone. And its refusal is a TYPE (datasource.UnknownFieldError), which is
// what lets the refusal below keep the caller's own key while masking the
// decoder's words about this program; encoding/json's own unknown-field message
// is prose only a string match could recognise.
//
// It does NOT settle whether a required uuid argument was supplied: `ids.UUID`
// zero-values an absent key without erroring, so that claim is made once for the
// whole surface at Registry.Invoke (requireDeclaredIDs) rather than in each
// handler — which is how thirteen handlers came to miss it.
func decodeArgs[T any](in json.RawMessage, into *T) error {
	if unknown := datasource.RejectNonCanonicalKeys(in, into); unknown != nil {
		// OURS, and it quotes the caller's own argument names and nothing else —
		// the most actionable thing this surface says, so it travels verbatim.
		return &BadArgsError{Cause: unknown}
	}
	dec := json.NewDecoder(bytes.NewReader(in))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		// Everything left is encoding/json or a value unmarshaler underneath it
		// describing THIS program — the Go struct being filled, the Go type of a
		// field, a library's `invalid UUID length: 6` — which an agent can
		// neither act on nor is entitled to read, and which lands in a run's
		// cumulative transcript. A shape we can name is restated as the argument
		// plus the form it takes; one we cannot is masked, and the words withheld
		// go to the operator's log instead of nowhere.
		safe, withheld := httperr.SafeDecodeError(err)
		if withheld {
			slog.Warn("unnamed tool-argument decode failure", "err", err)
		}
		return &BadArgsError{Cause: safe}
	}
	if dec.More() {
		// One value, the same boundary the REST body decode draws
		// (httperr.Decode): decoding the first and discarding the rest would
		// take a caller's second value — a correction, a merged retry — without
		// saying that anything was dropped, and a silent truncation is the one
		// refusal shape an agent cannot re-plan around. OURS, so it travels
		// verbatim.
		return &BadArgsError{
			Cause:    errors.New("trailing content after the first JSON value"),
			Guidance: "send exactly one JSON object carrying this tool's arguments",
		}
	}
	return nil
}

// maxBadArgsDetail bounds what a rejected tool call may say back. The
// unknown-key refusal quotes the caller's own argument names verbatim,
// the refusal becomes an observation, and an agent run's transcript is
// cumulative — so an unbounded message is an unbounded write into every
// later prompt of that run, by the one author that has already been shown
// the fence marker. The tool NAME is bounded for exactly this reason
// (runner.maxToolNameLen); this is the other field a model chooses freely.
// Long enough to name the offending key and what was wanted, short enough
// that the field cannot carry prose.
const maxBadArgsDetail = 200

// MaxFaultDetail bounds a CLASSIFIED refusal's detail, which is a different
// obligation from maxBadArgsDetail above.
//
// The two differ in who wrote the text. A bad-args detail quotes the caller's
// own argument names and is mostly their words, so 200 is generous. A
// classified fault's detail is OURS: it names what was refused and then, for
// every closed vocabulary on this surface, lists the whole set that would have
// worked. A set measured against a budget sized for an argument-name echo does
// not fit, and a truncated set is worse than an absent one — it reads as
// complete, so a caller stops looking for what was removed.
//
// The caller's share of one of these is bounded at its SOURCE
// (httperr.QuoteCaller), so this figure is spent on our own text rather than on
// whatever a caller sent. That is what makes it safe to be the larger number:
// the refusal's length is a property of the vocabulary it names.
//
// NOT derived, and it cannot be from here — the vocabularies live in compose,
// downstream of this package, so naming them would invert the dependency.
//
// Exported as a shared FIGURE for that reason, the way httperr.MaxFaultText is:
// the gate that CAN see those vocabularies reads this number and fails when one
// of them no longer fits, rather than keeping a second copy of it that would
// agree only until somebody edited one.
const MaxFaultDetail = 512

// BadArgsError maps to a tool-call validation failure.
//
// The two members have opposite provenance, and that is the whole reason they
// are separate. Cause may quote the CALLER — the key refusal echoes the argument
// name it refused — so it is bounded and escaped. Guidance is OURS: a fixed
// vocabulary reflected off the contract, chosen by no caller.
type BadArgsError struct {
	Cause error
	// Field is the argument the caller must change, as the contract spells it,
	// and empty only where no single one is at fault.
	//
	// It exists because almost every refusal of this kind DOES name an input —
	// `to_phase "vibing" is not a project phase` — and saying so only in prose
	// left the 422's `details.errors` empty on the REST agent door while the
	// session door answering the identical mistake filled it. A client
	// branching on the structured list got nothing to branch on, and the
	// coverage gate had to match a substring instead of a field code.
	Field string
	// Code is the per-field machine code, empty for the validation_error every
	// BadArgsError answers by default.
	//
	// It exists for the refusals a REST door answers more precisely. `links`
	// missing is `required` on activities' own store, and a client branching on
	// details.errors must not have to know which door refused it to recognise
	// the same mistake — which is the property ADR-0055 is about, arriving at
	// the field level rather than the status one.
	Code string
	// Guidance is server-authored text appended after the echo, and it is NOT
	// bounded. Bounding it with the echo is what made the accepted-field list
	// truncate mid-word on a long unknown key — cutting away the list the
	// message exists to teach, exactly when the caller most needed it. The
	// bound guards against an unbounded write into a run's transcript by the
	// model being prompted; our own strings were never that.
	Guidance string
}

func (e *BadArgsError) Error() string {
	msg := "arguments: " + echoSafe(e.Cause.Error(), maxBadArgsDetail)
	if e.Guidance == "" {
		return msg
	}
	return msg + "; " + e.Guidance
}
func (e *BadArgsError) Unwrap() error { return e.Cause }

// FieldFaults lets a BadArgsError classify correctly when it reaches the REST
// surface — which the create/patch resolvers' shared Guards are the first
// callers to do, since every earlier BadArgsError site answered the MCP tool
// door alone. httperr.Classify has no notion of this package's own error type,
// and moduleDeclaredFault (its comment: "a module opts in by implementing a
// method") is the seam built for exactly that; without it the REST door would
// answer a caller's own mistake with an opaque 500, which is the one thing
// clientInputValidation's own doc says must never happen.
//
// The MCP tool door is untouched by this: Dispatcher.explain matches
// *BadArgsError by type BEFORE it ever consults httperr.Classify, so this
// method is never read on that path.
//
// FieldFaults — the PLURAL — rather than FieldFault or MessageFault, because it
// is the only one of the three that can answer both shapes from one method. A
// type implements exactly one, and errors.As matches whichever it finds, so a
// conditional choice between the singular and the message form is not
// expressible. The plural returns one entry when a field is named and NONE when
// it is not, which is precisely the difference: the caller gets a per-field
// entry to act on, or the same 422 with an empty list this refusal has always
// answered. Inventing an entry for a refusal that names no input would point a
// caller at an argument that is not theirs to change.
func (e *BadArgsError) FieldFaults() []apperrors.FieldRefusal {
	if e.Field == "" {
		return nil
	}
	// The default REUSES validation_error rather than minting one: crm.yaml
	// already declares it for exactly this class of caller mistake, and
	// inventing a second would put an undocumented code in front of a client
	// that branches on the documented one (P3 — the contract wins). A refusal
	// that names a MORE precise code says so through Code, and those are
	// codes crm.yaml already declares too — `required` is what every REST door
	// answers for a value that was absent.
	code := e.Code
	if code == "" {
		code = "validation_error"
	}
	return []apperrors.FieldRefusal{{Field: e.Field, Code: code, Message: e.Error()}}
}

// boundDetail caps a message at n bytes, cutting on a rune boundary so the
// result stays valid UTF-8 rather than ending mid-sequence.
func boundDetail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}

// invalidByteAt says whether the byte at i is one UTF-8 cannot decode, as
// opposed to the first byte of a legitimately encoded U+FFFD.
//
// The two are indistinguishable to a range loop — both yield RuneError — and
// telling them apart by re-validating the single byte gets it WRONG for the
// legitimate one: the lead byte of U+FFFD (0xEF) is not valid UTF-8 on its own,
// so a caller who sent a replacement character had it reported back as a bare
// \xef with its two continuation bytes dropped. The decode WIDTH is what
// separates them: one byte means the decoder gave up, three means it succeeded.
func invalidByteAt(s string, i int) bool {
	_, size := utf8.DecodeRuneInString(s[i:])
	return size == 1
}

// echoSafe prepares caller-authored text for a tool result: bounded, and with
// everything that does not PRINT rendered as a visible escape.
//
// Bounding alone is not enough. A tool result lands in a transcript that later
// prompts of the same run read, and the author of these strings is the model
// being prompted — so a newline in a field name can open what reads as a new
// line of conversation, and an escape byte can move a terminal's cursor.
// Rendering them keeps what the caller actually wrote while taking away its
// ability to forge the frame around it.
//
// UNPRINTABLE, not "an ASCII control character", and the difference is a
// reachable injection. This switch tested `r < 0x20 || r == 0x7f`, which admits
// every character above ASCII that ends a line or reverses one: U+2028 LINE
// SEPARATOR and U+2029 PARAGRAPH SEPARATOR are line terminators to a great many
// tokenizers and renderers, U+0085 NEXT LINE is one to some, U+202E flips the
// reading order of everything after it, and U+200B is invisible. A caller who
// cannot write a newline into the frame could write U+2028 and get one.
//
// unicode.IsPrint is the same question `%q` asks — which matters, because
// httperr.QuoteCaller bounds a caller's token with `%q` before it ever reaches
// here, and two spellings of "safe to echo" that disagreed would mean the
// protection depended on which door the text came through.
func echoSafe(s string, n int) string {
	var b strings.Builder
	b.Grow(len(s))
	for i, r := range s {
		switch {
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == utf8.RuneError && invalidByteAt(s, i):
			// A byte that is not UTF-8 at all. Ranging yields RuneError for it,
			// which IS printable, so writing the rune back would replace the
			// caller's byte with U+FFFD and report a name they did not send.
			fmt.Fprintf(&b, `\x%02x`, s[i])
		case !unicode.IsPrint(r) && r > 0xFFFF:
			// An UNPRINTABLE rune above the BMP, and the printability test comes
			// first for a reason: an emoji and a CJK extension character are
			// both astral AND printable, so escaping every astral rune would
			// mangle a real name. `\u` takes no more than four digits, so
			// `\ue0020` for U+E0020 is not a legal escape anywhere and reads as
			// `\ue002` followed by a `0` — and U+E0000..U+E007F is the tag block
			// used to smuggle invisible text.
			fmt.Fprintf(&b, `\U%08x`, r)
		case !unicode.IsPrint(r):
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			b.WriteRune(r)
		}
	}
	return boundDetail(b.String(), n)
}
