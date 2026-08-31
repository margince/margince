// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// encodeInstallationPatch's completeness, held rather than claimed.
//
// The comment above that function says every field of the patch appears in it
// exactly once, and a field left out is a value that silently stops saving:
// the form still shows it, the PATCH still carries it, the write still returns
// 200, and the row never moves. Nothing failed when that claim was only a
// comment — dropping a line from the encoder left every test in the tree green.
//
// So the list is DERIVED from InstallationPatch rather than restated here. A
// sixth setting added to the struct fails this test until it is encoded, which
// is the point: the fields that exist are the fields that must be written, and
// only the struct knows what those are.

import (
	"reflect"
	"testing"
)

func TestEveryInstallationPatchFieldIsEncoded(t *testing.T) {
	patchType := reflect.TypeOf(InstallationPatch{})

	// A patch with EVERY field set, built by reflection so no field can be
	// missed here either. A hand-written literal would have the same gap as
	// the encoder it is checking.
	filled := reflect.New(patchType).Elem()
	for i := range patchType.NumField() {
		field := filled.Field(i)
		if field.Kind() != reflect.Pointer {
			t.Fatalf("%s is %s, want a pointer — a sparse patch marks absence with nil",
				patchType.Field(i).Name, field.Kind())
		}
		// A non-zero value, so an encoder that wrote the type's zero would
		// still be distinguishable from one that wrote what it was given.
		value := reflect.New(field.Type().Elem())
		switch value.Elem().Kind() {
		case reflect.String:
			value.Elem().SetString("x")
		case reflect.Int:
			value.Elem().SetInt(7)
		case reflect.Slice:
			// NON-EMPTY, for the same reason the scalars above are non-zero: an
			// encoder that wrote a slice the caller did not give it would be
			// indistinguishable from one that passed theirs through. The empty
			// list matters here beyond the usual zero-value argument, because it
			// is a legitimate CHOICE on this field — offer password only — and a
			// fixture that used it could not tell the two apart.
			filledSlice := reflect.MakeSlice(value.Elem().Type(), 1, 1)
			if filledSlice.Index(0).Kind() != reflect.String {
				t.Fatalf("%s is a slice of %s, which this test does not know how to fill",
					patchType.Field(i).Name, filledSlice.Index(0).Kind())
			}
			filledSlice.Index(0).SetString("x")
			value.Elem().Set(filledSlice)
		default:
			t.Fatalf("%s holds %s, which this test does not know how to fill — "+
				"give it a case rather than letting the field go unchecked",
				patchType.Field(i).Name, value.Elem().Kind())
		}
		field.Set(value)
	}

	encoded, err := encodeInstallationPatch(filled.Interface().(InstallationPatch))
	if err != nil {
		t.Fatalf("encoding a fully-populated patch: %v", err)
	}

	if len(encoded) != patchType.NumField() {
		t.Errorf("the encoder produced %d writes for %d patch fields — "+
			"a field it does not encode is one that silently stops saving",
			len(encoded), patchType.NumField())
	}

	// Every write carries bytes, and no key appears twice. A duplicated entry
	// would pass the count above while leaving a real field unwritten.
	seen := map[string]bool{}
	for _, w := range encoded {
		if w.raw == nil {
			t.Errorf("%s encoded to nil bytes from a field that was set", w.entry.Key())
		}
		if seen[w.entry.Key()] {
			t.Errorf("%s is encoded twice, so some other field is not encoded at all", w.entry.Key())
		}
		seen[w.entry.Key()] = true
	}
}

// The other half: an absent field must write NOTHING, not a zero.
//
// This is the direction with teeth. `any(p)` on a nil typed pointer is not
// equal to nil — the interface carries the type — so an encoder that widened
// too early would see every absent field as present and write the type's zero
// over it. For the fiscal month that zero is month 0, which is not a month.
func TestAnAbsentInstallationPatchFieldEncodesToNothing(t *testing.T) {
	encoded, err := encodeInstallationPatch(InstallationPatch{})
	if err != nil {
		t.Fatalf("encoding an empty patch: %v", err)
	}
	for _, w := range encoded {
		if w.raw != nil {
			t.Errorf("%s encoded %q from an absent field; the write loop would store it",
				w.entry.Key(), string(w.raw))
		}
	}
}
