// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package imagenorm

import (
	"errors"
	"testing"
)

const svgOpen = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 50">`

func TestDecodeRefusesAnSVGWhoseWordsAreLiveText(t *testing.T) {
	cases := map[string]string{
		"a text element": svgOpen + `<circle cx="10" cy="10" r="10"/><text x="30" y="20">Acme</text></svg>`,
		"a nested tspan": svgOpen + `<text x="0" y="20"><tspan>Ac</tspan>me</text></svg>`,
		"a textPath":     svgOpen + `<defs><path id="p" d="M0 0 L100 0"/></defs><textPath href="#p">Acme</textPath></svg>`,
		"a prefixed text": `<svg:svg xmlns:svg="http://www.w3.org/2000/svg" viewBox="0 0 10 10">` +
			`<svg:text>Acme</svg:text></svg:svg>`,
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Decode([]byte(doc))
			if !errors.Is(err, ErrSVGTextNotOutlined) {
				t.Fatalf("err = %v, want ErrSVGTextNotOutlined", err)
			}
			if !errors.Is(err, ErrUnsupported) {
				t.Fatalf("err = %v, want it to stay ErrUnsupported for callers that skip", err)
			}
		})
	}
}

func TestDecodeKeepsAnSVGWhoseOnlyTextIsMetadata(t *testing.T) {
	doc := svgOpen + `<!-- <text>Acme</text> --><title>text logo</title><desc>a text wordmark</desc>` +
		`<circle cx="25" cy="25" r="20" fill="#ff6b00"/></svg>`
	if _, err := Decode([]byte(doc)); err != nil {
		t.Fatalf("title, desc and comments draw no glyphs, yet the SVG was refused: %v", err)
	}
}
