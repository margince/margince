// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package overlay

// The mirror stores a PROJECTION, not the incumbent's raw record, and a row is
// re-projected only when the incumbent's own baseline advances. So a mapping
// change leaves every already-mirrored row holding a payload this code would
// never produce again, indefinitely. The fingerprint is how a row says which
// declaration produced it, so that condition is detectable rather than silent.
//
// It hashes the declaration's DATA, never its source text: a comment or
// formatting edit must not invalidate an estate, and a semantic change must
// not fail to.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"sort"
	"strconv"
)

// Fingerprint digests one object mapping's declaration. Two mappings that
// would project the same raw record identically share a fingerprint; any
// difference that could change a projected payload changes it.
//
// It returns an error because the declaration values are ENCODED rather than
// rendered — see writeSortedMap. A declaration carrying a value JSON cannot
// represent has no fingerprint, and answering one anyway would be answering
// about a mapping this code cannot project.
func Fingerprint(m ObjectMapping) (string, error) {
	h := sha256.New()
	writeField(h, "source", m.Source)
	writeField(h, "target", m.Target)
	writeField(h, "external_key", m.ExternalKey)
	writeField(h, "baseline", m.Baseline)
	writeField(h, "unmapped_policy", m.UnmappedPolicy)
	if err := writeSortedMap(h, "const", m.Const); err != nil {
		return "", fmt.Errorf("overlay: fingerprinting %s's const declaration: %w", m.Source, err)
	}
	for i, f := range m.Fields {
		// The index is hashed conservatively, not because a reorder is known to
		// matter: order-independence rests on guards declared elsewhere —
		// checkTargetCollisions rejects a shared target outright, and
		// sortChildRowsByPosition re-sorts one parent's child rows by positions
		// checkChildRowDeclarations keeps unique. Pinning the index costs a
		// re-projection whenever Fields is reordered; leaving it out would bet
		// the estate on those guards never being relaxed, and a missed
		// re-projection is the failure this digest exists to prevent.
		//
		// From is written element by element, prefixed with its count, because a
		// flattened rendering cannot separate ["a","b"] from ["a b"] — one
		// TargetAssembler gathering two raw properties from one gathering a
		// single property whose name contains a space. The count also keeps a nil
		// From and an empty one equal, which they are: neither gathers anything.
		writeField(h, fmt.Sprintf("field.%d.from.count", i), strconv.Itoa(len(f.From)))
		for j, from := range f.From {
			writeField(h, fmt.Sprintf("field.%d.from.%d", i, j), from)
		}
		writeField(h, fmt.Sprintf("field.%d.to", i), f.To)
		writeField(h, fmt.Sprintf("field.%d.kind", i), f.Kind.String())
		writeField(h, fmt.Sprintf("field.%d.transform", i), f.Transform)
		writeField(h, fmt.Sprintf("field.%d.resolve", i), f.Resolve)
		writeField(h, fmt.Sprintf("field.%d.always_emit", i), strconv.FormatBool(f.AlwaysEmit))
		if f.Child != nil {
			writeField(h, fmt.Sprintf("field.%d.child.position", i), strconv.Itoa(f.Child.Position))
			if err := writeSortedMap(h, fmt.Sprintf("field.%d.child.attrs", i), f.Child.Attrs); err != nil {
				return "", fmt.Errorf("overlay: fingerprinting %s's field %d child attributes: %w", m.Source, i, err)
			}
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// writeField feeds one named value into the digest length-prefixed, so no
// concatenation of two values can collide with a different pair. The parameter
// is a hash.Hash rather than an io.Writer because that interface documents a
// Write which never returns an error — there is no failure here to report.
func writeField(h hash.Hash, name, value string) {
	field := fmt.Sprintf("%d:%s=%d:%s;", len(name), name, len(value), value)
	h.Write([]byte(field))
}

// writeSortedMap feeds a declaration map in key order, because Go's map
// iteration order varies per run and a fingerprint that varied would mark
// every mirrored row stale forever.
//
// Values are ENCODED as JSON, and the distinction from rendering them is the
// whole point. Const and Attrs are copied verbatim into the projected payload,
// so the digest has to separate values the payload separates: the bool true
// from the string "true", and 1 from 1.0 from "1" — a declaration edit that
// flips one for the other changes the mirrored JSON.
//
// %#v did that too, and was chosen for it. What it is not is a FORMAT. It is a
// documented rendering of Go syntax, and a toolchain upgrade that changed it
// would move every fingerprint in the estate at once — every mirrored row
// reading as stale and re-projecting, spending a full estate's worth of metered
// incumbent API budget for a declaration nobody edited. Not incorrect, since a
// fingerprint change is exactly the event the sweep converges, but paid for
// nothing. json.Marshal is a specified format with a stability guarantee the
// rendering does not carry.
//
// The value's Go TYPE is deliberately not written alongside it, and that is a
// change from the rendering this replaced. %#v needed the type because it is
// type-blind where the payload is not — it renders the bool true and the string
// "true" identically. JSON is not: `true` and `"true"` are different bytes, as
// are `1` and `"1"`. The encoding already draws every distinction the payload
// draws.
//
// Writing the type as well would draw one the payload does NOT: an int 1 and a
// float64 1.0 both reach the mirror as `1`, so they project the same record and
// belong under the same fingerprint. Separating them would mark an estate stale
// for a declaration edit that changed nothing a reader can see — the same
// pointless re-projection this issue is about, arriving by a different route.
//
// encoding/json sorts object keys, so a map one level down is as ordered as the
// loop below makes the top level — and it draws the same distinctions there,
// which a type prefix could not have done at depth anyway.
//
//craft:ignore naked-any the declaration maps hold decoded JSON values; the any is the declared type, not a missed one
func writeSortedMap(h hash.Hash, name string, values map[string]any) error {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		encoded, err := json.Marshal(values[k])
		if err != nil {
			return fmt.Errorf("encoding %s.%s: %w", name, k, err)
		}
		writeField(h, name+"."+k, string(encoded))
	}
	return nil
}
