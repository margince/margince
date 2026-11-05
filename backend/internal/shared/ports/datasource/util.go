// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package datasource

// Provider-side mechanics every SystemOfRecordProvider implementation
// needs (the SoR-mode modules today, an overlay adapter tomorrow):
// strict field decoding, payload normalization, record marshalling, and
// the typed errors the transports map to 422. Additive utilities
// only — the seam interfaces above stay frozen (ADR-0054 §4).

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"
)

// UnsupportedEntityError maps to 422 on every surface. It is raised by the
// provider a request reached, which owns a SUBSET of the vocabulary — so the
// type it names is routinely a valid one sitting at the wrong provider
// (`deal` at the contacts provider), not an invalid one. The refusal is
// therefore about service, and the vocabulary rides along as a hint rather
// than as the set being denied.
type UnsupportedEntityError struct{ Type string }

// Error makes two claims and keeps them apart, because the vocabulary contains a
// member no provider serves every verb for: `relationship` has record verbs and
// no search, no schema descriptor, no attachment writer. So the hint names the
// vocabulary AS a vocabulary — a caller learns the spelling — without the implied
// promise that every member reaches every verb. Phrased as "the types served
// here", the sentence would refuse relationship while listing it as available.
func (e *UnsupportedEntityError) Error() string {
	return "entity_type " + e.Type + " is not served here; the vocabulary is " + entityVocabulary +
		", and a valid type can name a verb or provider that does not serve it"
}

// entityVocabulary renders the known set from EntityTypes rather than
// restating it: this package defines that vocabulary, so a restated copy here
// is the one most likely to be believed and the least likely to be updated.
var entityVocabulary = func() string {
	all := EntityTypes()
	names := make([]string, 0, len(all))
	for _, t := range all {
		names = append(names, string(t))
	}
	return strings.Join(names, "|")
}()

// FieldDecodeError maps to 422 on every surface. Its Cause has two very
// different provenances, and a transport must not treat them alike: it is either
// a refusal this package wrote (UnknownFieldError below), which names the
// caller's own key and is theirs to read, or whatever encoding/json and the
// value unmarshalers underneath it produced, which describes OUR program. The
// transport tells them apart by TYPE — the library's prose is a message nobody
// promised to keep, so matching on it is not a boundary.
type FieldDecodeError struct{ Cause error }

func (e *FieldDecodeError) Error() string { return "fields: " + e.Cause.Error() }
func (e *FieldDecodeError) Unwrap() error { return e.Cause }

// UnknownFieldError is this package's OWN refusal of field keys the target does
// not serve. It quotes the keys the caller sent and nothing else, so it is the
// one field-decode cause a transport may show verbatim.
//
// Typed for exactly that reason: it travels wrapped in FieldDecodeError
// alongside decoder failures whose text is Go internals, and the transport has
// to keep one and mask the other.
type UnknownFieldError struct {
	Fields []string
	// Hint says what to do instead, when the target has an answer.
	Hint string
}

func (e *UnknownFieldError) Error() string {
	text := e.keys()
	if e.Hint != "" {
		return text + "; " + e.Hint
	}
	return text
}

func (e *UnknownFieldError) keys() string {
	quoted := make([]string, 0, len(e.Fields))
	for _, field := range e.Fields {
		quoted = append(quoted, strconv.Quote(field))
	}
	if len(quoted) == 1 {
		return "unknown field " + quoted[0]
	}
	return "unknown fields " + strings.Join(quoted, ", ")
}

// RawFields normalizes the seam's `any` payload: tools hand over the
// agent's raw JSON; in-process callers may hand the typed request struct.
//
//craft:ignore naked-any the frozen seam's Fields payload is declared any (ports/datasource) — this is the one normalizer of that seam
func RawFields(v any) (json.RawMessage, error) {
	switch f := v.(type) {
	case json.RawMessage:
		return f, nil
	case []byte:
		return f, nil
	default:
		return json.Marshal(v)
	}
}

// StrictDecode rejects unknown fields: an agent misspelling an argument gets a
// 422 naming it, never a silent drop.
//
// A key must match a contract field name byte for byte. encoding/json would
// match `FULL_NAME` to `full_name`, while the human-edit probe reads audit keys
// case-sensitively and would clear that patch as touching no human-owned field.
// A target with a catch-all map is decoded by StrictCustomFieldDecode, which
// holds its core keys to the same rule and admits cf_ keys beside them.
//
//craft:ignore naked-any the deserialization seam: the decode target is whichever provider request struct the caller owns
func StrictDecode(raw json.RawMessage, into any) error {
	if HasCustomFieldMap(into) {
		return StrictCustomFieldDecode(raw, into)
	}
	if err := RejectNonCanonicalKeys(raw, into); err != nil {
		return &FieldDecodeError{Cause: err}
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		// Which field, and what shape it wanted. The decoder answers neither
		// for a generated UnmarshalJSON (see fieldshape.go), and the transport
		// left with an unnamed shape failure says "the payload must be a JSON
		// object" about a payload that IS one.
		if shape := LocalizeFieldFault(raw, into, err, strictProbe); shape != nil {
			return &FieldDecodeError{Cause: shape}
		}
		return &FieldDecodeError{Cause: err}
	}
	if outOfRange := RejectOutOfRangeTimes(into); outOfRange != nil {
		return &FieldDecodeError{Cause: outOfRange}
	}
	return nil
}

// RejectNonCanonicalKeys refuses a key that is no contract field, byte for byte.
// A target with a catch-all map is left to the split decode. That decode lifts
// its cf_ keys first and holds the rest to this rule. The provider seam and the
// REST decode both call it, so they refuse the same keys.
//
//craft:ignore naked-any mirror of StrictDecode's seam target
func RejectNonCanonicalKeys(raw json.RawMessage, into any) error {
	t := reflect.TypeOf(into)
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil || t.Kind() != reflect.Struct {
		return nil
	}
	canonical := make(map[string]struct{})
	if collectCanonicalKeys(t, canonical) {
		return nil
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		// Not a JSON object — the decoder below reports the shape error.
		return nil
	}
	var unknown []string
	for key := range keys {
		if _, ok := canonical[key]; !ok {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	// Sorted because map iteration is not: a refusal that names a different one
	// of the caller's mistakes on each identical request is not reproducible,
	// and reporting all of them saves a round trip per misspelling.
	slices.Sort(unknown)
	return &UnknownFieldError{Fields: unknown}
}

// collectCanonicalKeys gathers the json field names of a struct into out, walking
// embedded structs as encoding/json does. It stops and answers true at an
// AdditionalProperties catch-all, a `json:"-"` map field. The test is the field's
// kind, so a plain ignored `json:"-"` field is not one.
func collectCanonicalKeys(t reflect.Type, out map[string]struct{}) (hasCatchAll bool) {
	for field := range t.Fields() {
		tag := field.Tag.Get("json")
		name := tag
		if before, _, ok := strings.Cut(tag, ","); ok {
			name = before
		}
		if name == "-" {
			if field.Type.Kind() == reflect.Map {
				return true
			}
			continue
		}
		if field.Anonymous && name == "" {
			et := field.Type
			for et.Kind() == reflect.Pointer {
				et = et.Elem()
			}
			if et.Kind() == reflect.Struct && collectCanonicalKeys(et, out) {
				return true
			}
			continue
		}
		if name != "" {
			out[name] = struct{}{}
		}
	}
	return false
}

// NewRecord marshals a provider's typed fields into the seam's Record
// shape. SoR-mode freshness is trivially authoritative: there is no
// mirror to go stale.
//
//craft:ignore naked-any provider field shapes are per-entity structs serialized into the seam's schemaless Record.Fields
func NewRecord(ref EntityRef, fields any, version *int64) (Record, error) {
	raw, err := json.Marshal(fields)
	if err != nil {
		return Record{}, err
	}
	rec := Record{Ref: ref, Fields: raw, Freshness: FreshnessInfo{LastSyncedAt: time.Now().UTC(), Authoritative: true}}
	if version != nil {
		rec.Version = *version
	}
	return rec, nil
}
