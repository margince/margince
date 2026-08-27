// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package model_test

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

func TestTheIntersectionIsWhatBothSidesAdmit(t *testing.T) {
	for name, tc := range map[string]struct {
		a, b []string
		want []string
	}{
		// The case the literal comparison got wrong, and the reason this
		// function exists: a permission written as a wildcard against a decoder
		// written as types is agreement, not contradiction.
		"a wildcard meets the types it covers": {
			a:    []string{"image/*"},
			b:    []string{"image/jpeg", "image/png"},
			want: []string{"image/jpeg", "image/png"},
		},
		"and in the other order": {
			a:    []string{"image/jpeg", "image/png"},
			b:    []string{"image/*"},
			want: []string{"image/jpeg", "image/png"},
		},
		"two wildcards keep the narrower": {
			a:    []string{"image/*"},
			b:    []string{"image/x-*"},
			want: []string{"image/x-*"},
		},
		"disjoint wildcards share nothing": {
			a:    []string{"image/*"},
			b:    []string{"audio/*"},
			want: []string{},
		},
		"exact types intersect as a set": {
			a:    []string{"image/jpeg", "image/png", "application/pdf"},
			b:    []string{"image/png", "image/webp", "application/pdf"},
			want: []string{"image/png", "application/pdf"},
		},
		// The document lane a mixed ladder used to lose in silence: one rung
		// narrowed to a vendor's types, the other still declaring the pattern.
		"a mixed ladder keeps its lane": {
			a:    []string{"image/jpeg", "image/png", "image/gif", "image/webp", "application/pdf"},
			b:    []string{"image/*", "application/pdf"},
			want: []string{"image/jpeg", "image/png", "image/gif", "image/webp", "application/pdf"},
		},
		"an empty side carries nothing": {
			a:    []string{"image/*"},
			b:    nil,
			want: []string{},
		},
		// Two spellings of one set would make equality depend on argument
		// order, and a caller comparing carriage sets is the normal case.
		"a covered type does not survive beside the pattern covering it": {
			a:    []string{"image/*", "image/png"},
			b:    []string{"image/*", "image/png"},
			want: []string{"image/*"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := model.IntersectMIMEs(tc.a, tc.b); !slices.Equal(got, tc.want) {
				t.Fatalf("IntersectMIMEs(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

// The safety property, and the one this replaced a blunt rule to keep. Checked
// over concrete media types rather than over spellings, because a set is what
// it admits: whatever survives the intersection must have been admitted by both
// inputs, and whatever both inputs admit must survive.
func TestTheIntersectionIsNeverWiderThanEitherSideAndNeverNarrower(t *testing.T) {
	// Every media type these declarations can distinguish, including the ones
	// no vendor decodes — svg is the type that started this.
	probes := []string{
		"image/jpeg", "image/png", "image/gif", "image/webp", "image/heic",
		"image/svg+xml", "image/x-icon", "application/pdf", "text/plain", "audio/mpeg",
	}
	sets := [][]string{
		nil,
		{"image/*"},
		{"image/*", "application/pdf"},
		{"image/jpeg", "image/png", "image/gif", "image/webp", "application/pdf"},
		{"image/jpeg", "image/png", "image/webp", "image/heic", "application/pdf"},
		{"image/x-*"},
		{"application/pdf"},
		{"*"},
	}
	for _, a := range sets {
		for _, b := range sets {
			got := model.IntersectMIMEs(a, b)
			for _, probe := range probes {
				inBoth := model.CarriesMIME(a, probe) && model.CarriesMIME(b, probe)
				if model.CarriesMIME(got, probe) != inBoth {
					t.Errorf("IntersectMIMEs(%v, %v) = %v: admits %q = %v, both sides admit it = %v",
						a, b, got, probe, !inBoth, inBoth)
				}
			}
		}
	}
}
