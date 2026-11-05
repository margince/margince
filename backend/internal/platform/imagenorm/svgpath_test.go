// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package imagenorm

import (
	"image"
	"image/draw"
	"testing"
)

func svgWithPath(d string) []byte {
	return []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 40 40"><path d="` + d + `" fill="#000"/></svg>`)
}

func TestDecodeDrawsARepeatedArcLikeOneLetterPerArc(t *testing.T) {
	cases := map[string][2]string{
		"relative arcs": {
			"M10 5h10a7 7 0 0 1 0 14 9 9 0 0 1 0 18H10z",
			"M10 5h10a7 7 0 0 1 0 14a9 9 0 0 1 0 18H10z",
		},
		"compact flags": {
			"M10 5h10a7 7 0 016 6 9 9 0 016 12H10z",
			"M10 5h10a7 7 0 0 1 6 6a9 9 0 0 1 6 12H10z",
		},
		"absolute arcs": {
			"M10 5H20A7 7 0 0 1 20 19 9 9 0 0 1 20 37H10z",
			"M10 5H20A7 7 0 0 1 20 19A9 9 0 0 1 20 37H10z",
		},
	}
	for name, pair := range cases {
		t.Run(name, func(t *testing.T) {
			repeated, err := Decode(svgWithPath(pair[0]))
			if err != nil {
				t.Fatalf("repeated: %v", err)
			}
			separate, err := Decode(svgWithPath(pair[1]))
			if err != nil {
				t.Fatalf("separate: %v", err)
			}
			if !samePixels(repeated, separate) {
				t.Fatal("a repeated arc command draws differently from one letter per arc")
			}
		})
	}
}

func TestExpandPathDataGivesEveryParameterSetItsOwnLetter(t *testing.T) {
	cases := map[string]string{
		"a7 7 0 0 1-4.1 6.27 7.4 7.4 0 0 1 5.36 7.37": "a7 7 0 0 1 -4.1 6.27 a7.4 7.4 0 0 1 5.36 7.37",
		"a7 7 0 016 6 9 9 0 016 12":                   "a7 7 0 0 1 6 6 a9 9 0 0 1 6 12",
		"M1 2 3 4m5 6 7 8":                            "M1 2 L3 4 m5 6 l7 8",
		"M0,0 h1 2 v3 4z":                             "M0 0 h1 h2 v3 v4 z",
		"c1 2 3 4 5 6 7 8 9 10 11 12":                 "c1 2 3 4 5 6 c7 8 9 10 11 12",
		"M.5.5 1e2-1.5E-1":                            "M.5 .5 L1e2 -1.5E-1",
	}
	for in, want := range cases {
		got, ok := expandPathData(in)
		if !ok || got != want {
			t.Errorf("expandPathData(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
}

func TestExpandPathDataRefusesMalformedData(t *testing.T) {
	for _, in := range []string{"10 20", "M1", "M1 2 x", "a1 1 0 2 0 1 1"} {
		if got, ok := expandPathData(in); ok {
			t.Errorf("expandPathData(%q) = %q, want a refusal", in, got)
		}
	}
}

func samePixels(a, b image.Image) bool {
	if a.Bounds() != b.Bounds() {
		return false
	}
	left, right := image.NewNRGBA(a.Bounds()), image.NewNRGBA(b.Bounds())
	draw.Draw(left, left.Bounds(), a, a.Bounds().Min, draw.Src)
	draw.Draw(right, right.Bounds(), b, b.Bounds().Min, draw.Src)
	return string(left.Pix) == string(right.Pix)
}
