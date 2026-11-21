// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package imagenorm

import (
	"errors"
	"image/color"
	"strings"
	"testing"
)

const paintOpen = `xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100"`

const wholeCanvasCircle = `<circle cx="50" cy="50" r="45"`

func centreColor(t *testing.T, doc string) color.NRGBA {
	t.Helper()
	img, err := Decode([]byte(doc))
	if err != nil {
		t.Fatalf("an SVG a browser draws was refused: %v", err)
	}
	b := img.Bounds()
	return color.NRGBAModel.Convert(img.At(b.Dx()/2, b.Dy()/2)).(color.NRGBA)
}

func TestDecodeDrawsCurrentColorInTheRootsColor(t *testing.T) {
	orange := color.NRGBA{R: 0xff, G: 0x66, B: 0x00, A: 0xff}
	black := color.NRGBA{A: 0xff}
	cases := map[string]struct {
		doc  string
		want color.NRGBA
	}{
		"fill, color attribute":      {`<svg ` + paintOpen + ` color="#ff6600">` + wholeCanvasCircle + ` fill="currentColor"/></svg>`, orange},
		"fill, color in style":       {`<svg ` + paintOpen + ` style="color:#ff6600">` + wholeCanvasCircle + ` fill="currentColor"/></svg>`, orange},
		"fill, default black":        {`<svg ` + paintOpen + `>` + wholeCanvasCircle + ` fill="currentColor"/></svg>`, black},
		"stroke only":                {`<svg ` + paintOpen + ` color="#ff6600">` + `<line x1="0" y1="50" x2="100" y2="50" stroke="currentColor" stroke-width="40"/></svg>`, orange},
		"in an element style":        {`<svg ` + paintOpen + ` color="#ff6600">` + wholeCanvasCircle + ` style="fill:currentColor"/></svg>`, orange},
		"in a class rule":            {`<svg ` + paintOpen + ` color="#ff6600"><style>.a{fill:currentColor}</style>` + wholeCanvasCircle + ` class="a"/></svg>`, orange},
		"mixed case keyword":         {`<svg ` + paintOpen + ` color="#ff6600">` + wholeCanvasCircle + ` fill="CURRENTCOLOR"/></svg>`, orange},
		"root color is currentColor": {`<svg ` + paintOpen + ` color="currentColor">` + wholeCanvasCircle + ` fill="currentColor"/></svg>`, black},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := centreColor(t, tc.doc); got != tc.want {
				t.Fatalf("centre pixel = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDecodeAppliesElementAndIdSelectorsToTheShapesTheyMatch(t *testing.T) {
	orange := color.NRGBA{R: 0xff, G: 0x66, B: 0x00, A: 0xff}
	blue := color.NRGBA{B: 0xff, A: 0xff}
	cases := map[string]struct {
		doc  string
		want color.NRGBA
	}{
		"element selector":  {`<svg ` + paintOpen + `><style>circle{fill:#ff6600}</style>` + wholeCanvasCircle + `/></svg>`, orange},
		"id selector":       {`<svg ` + paintOpen + `><style>#mark{fill:#ff6600}</style>` + wholeCanvasCircle + ` id="mark"/></svg>`, orange},
		"selector list":     {`<svg ` + paintOpen + `><style>rect, circle { fill: #ff6600 }</style>` + wholeCanvasCircle + `/></svg>`, orange},
		"class still works": {`<svg ` + paintOpen + `><style>.a{fill:#ff6600}</style>` + wholeCanvasCircle + ` class="a"/></svg>`, orange},
		"id beats element":  {`<svg ` + paintOpen + `><style>circle{fill:#0000ff} #mark{fill:#ff6600}</style>` + wholeCanvasCircle + ` id="mark"/></svg>`, orange},
		"inline style beats the selector": {
			`<svg ` + paintOpen + `><style>circle{fill:#0000ff}</style>` + wholeCanvasCircle + ` style="fill:#ff6600"/></svg>`, orange},
		"selector beats the presentation attribute": {
			`<svg ` + paintOpen + `><style>circle{fill:#0000ff}</style>` + wholeCanvasCircle + ` fill="#ff6600"/></svg>`, blue},
		"selector paints currentColor": {
			`<svg ` + paintOpen + ` color="#ff6600"><style>circle{fill:currentColor}</style>` + wholeCanvasCircle + `/></svg>`, orange},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := centreColor(t, tc.doc); got != tc.want {
				t.Fatalf("centre pixel = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestResolveSVGPaintNeverWritesAnExternalReference(t *testing.T) {
	doc := `<svg ` + paintOpen + `><style>circle{fill:url(https://evil.example/x.svg#a)}</style>` + wholeCanvasCircle + `/></svg>`
	out, err := resolveSVGPaint([]byte(doc))
	if err != nil || string(out) != doc {
		t.Fatalf("a selector naming an external url was inlined: %s (err %v)", out, err)
	}
}

func TestResolveSVGPaintLeavesAnUnstyledDocumentByteForByte(t *testing.T) {
	doc := `<?xml version="1.0"?><svg ` + paintOpen + `><!-- c --><circle cx="1" cy="1" r="1" fill="#fff"/></svg>`
	out, err := resolveSVGPaint([]byte(doc))
	if err != nil || string(out) != doc {
		t.Fatalf("rewrote a document with nothing to resolve: %s (err %v)", out, err)
	}
}

func TestResolveSVGPaintDropsAPaintValueOverTheLengthCap(t *testing.T) {
	long := strings.Repeat("a", 10_000)
	uses := strings.Repeat(`<circle r="1" fill="currentColor"/>`, 200)
	doc := `<svg ` + paintOpen + ` color="` + long + `">` + uses + `</svg>`
	out, err := resolveSVGPaint([]byte(doc))
	if err != nil {
		t.Fatalf("a long root color should fall back to black, got %v", err)
	}
	if strings.Count(string(out), long) > 1 || len(out) > len(doc) {
		t.Fatalf("the long colour was copied into the document (%d bytes from %d)", len(out), len(doc))
	}
	if !strings.Contains(string(out), svgDefaultColor) {
		t.Fatal("the long root color did not fall back to black")
	}
}

func TestResolveSVGPaintRefusesAnAmplifyingRewrite(t *testing.T) {
	value := strings.Repeat("a", svgMaxPaintBytes)
	shapes := strings.Repeat(`<circle r="1"/>`, 12_000)
	doc := `<svg ` + paintOpen + `><style>circle{fill:` + value + `}</style>` + shapes + `</svg>`
	out, err := resolveSVGPaint([]byte(doc))
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("err = %v (%d bytes out), want ErrUnsupported for a rewrite past the bound", err, len(out))
	}
	if _, err := Decode([]byte(doc)); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Decode err = %v, want ErrUnsupported", err)
	}
}
