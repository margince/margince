// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package imagenorm

// SVG is the one source format that arrives as a DOCUMENT rather than pixels,
// and a modern site increasingly ships its mark as nothing else. It is
// rasterized here on the way in, never stored or served as markup: an SVG can
// carry script, and a scripted document served back from this deployment's own
// origin is a hole a favicon has no business opening. Rasterizing closes it by
// construction — what comes out the other side is pixels this package drew.

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

// svgRasterEdge is the square a vector mark is drawn into. It sits above the
// stored ceiling so SquarePNG's own downscale does the final resample — a
// vector has no native resolution to preserve, so drawing it small and
// enlarging later would throw away detail nothing can recover.
const svgRasterEdge = 512

// looksLikeSVG reports whether the bytes are an SVG document. Content types
// are not consulted: a server mislabels an asset often enough that the bytes
// are the only honest answer. The scan walks XML tokens rather than searching
// for "<svg" as text, so an XML comment or a stray mention in a JSON blob
// cannot pass for a document.
//
// It reads only as far as the first element, so its cost is set by where that
// element sits and not by the document's size — a byte cap here would only
// misreport a large vector as an undecodable raster.
func looksLikeSVG(src []byte) bool {
	if len(src) == 0 {
		return false
	}
	decoder := xml.NewDecoder(bytes.NewReader(src))
	// A favicon SVG routinely carries entities the strict decoder rejects, and
	// this pass only has to find the root element's name.
	decoder.Strict = false
	for {
		token, err := decoder.Token()
		if err != nil {
			return false // EOF or malformed before any element: not a document
		}
		if start, ok := token.(xml.StartElement); ok {
			return strings.EqualFold(start.Name.Local, "svg")
		}
	}
}

// ErrSVGTextNotOutlined reports live text, which the rasterizer draws no glyphs
// for. It wraps ErrUnsupported so a candidate chain still skips the file.
var ErrSVGTextNotOutlined = fmt.Errorf("%w: the SVG draws live text, which has to be converted to outlines", ErrUnsupported)

// svgTextElements put glyphs on the canvas. Names are matched without their
// namespace prefix, so svg:text counts too.
var svgTextElements = map[string]bool{"text": true, "tspan": true, "textpath": true}

const svgClipPath = "clippath"

// svgUndrawnElements drop their drawing in the renderer; inside <defs> only a
// referenced one is refused. A clipPath of one viewBox-sized rect loses nothing.
var svgUndrawnElements = map[string]bool{
	"foreignobject": true, "image": true, "filter": true, "pattern": true,
	"mask": true, "marker": true, "symbol": true, svgClipPath: true,
}

// ErrSVGNotDrawable reports an SVG the renderer would draw blank or only partly.
// It wraps ErrUnsupported so a candidate chain still skips the file.
var ErrSVGNotDrawable = fmt.Errorf("%w: the SVG has parts the renderer cannot draw", ErrUnsupported)

var svgURLReference = regexp.MustCompile(`url\(\s*['"]?#([^)'"\s]+)`)

// svgDefsElement is an undrawn element met inside <defs>, refused only if some
// paint, clip or href names its id.
type svgDefsElement struct{ name, id string }

// lossyScan is the state of one refuseLossySVG pass.
type lossyScan struct {
	textDepth, defsDepth int
	hasText              bool
	viewBox              [4]float64
	clip                 *svgClip
	pending              []svgDefsElement
	referenced           map[string]bool
}

// svgClip collects the children of the <clipPath> being read inside <defs>.
type svgClip struct {
	id          string
	transformed bool
	children    []xml.StartElement
	depth       int
}

// refuseLossySVG rejects a document the renderer would only partly draw; the
// stored PNG would drop content with no sign to the author.
func refuseLossySVG(src []byte) error {
	decoder := xml.NewDecoder(bytes.NewReader(src))
	decoder.Strict = false
	scan := &lossyScan{referenced: map[string]bool{}}
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return scan.verdict()
		}
		if err != nil {
			return fmt.Errorf("%w: the SVG is not readable as XML: %w", ErrUnsupported, err)
		}
		switch t := token.(type) {
		case xml.StartElement:
			if err := scan.start(t); err != nil {
				return err
			}
		case xml.EndElement:
			scan.end(strings.ToLower(t.Name.Local))
		case xml.CharData:
			if scan.textDepth > 0 && len(bytes.TrimSpace(t)) > 0 {
				scan.hasText = true
			}
			for _, m := range svgURLReference.FindAllSubmatch(t, -1) {
				scan.referenced[string(m[1])] = true
			}
		}
	}
}

func (s *lossyScan) start(t xml.StartElement) error {
	name := strings.ToLower(t.Name.Local)
	noteSVGReferences(t.Attr, s.referenced)
	if s.clip != nil {
		s.clip.children = append(s.clip.children, t)
		s.clip.depth++
		return nil
	}
	switch {
	case name == "svg" && s.viewBox == [4]float64{}:
		s.viewBox = svgViewBox(t.Attr)
	case name == "defs":
		s.defsDepth++
	case name == svgClipPath && s.defsDepth > 0:
		s.clip = &svgClip{id: svgElementID(t.Attr), transformed: svgHasTransform(t.Attr)}
	case svgUndrawnElements[name] && s.defsDepth > 0:
		s.pending = append(s.pending, svgDefsElement{name, svgElementID(t.Attr)})
	case svgUndrawnElements[name]:
		return fmt.Errorf("%w: the SVG holds <%s>", ErrSVGNotDrawable, name)
	case svgTextElements[name]:
		s.textDepth++
	}
	return nil
}

func (s *lossyScan) end(name string) {
	switch {
	case s.clip != nil && s.clip.depth > 0:
		s.clip.depth--
	case s.clip != nil && name == svgClipPath:
		if s.clip.transformed || !svgIsArtboardClip(s.clip.children, s.viewBox) {
			s.pending = append(s.pending, svgDefsElement{svgClipPath, s.clip.id})
		}
		s.clip = nil
	case name == "defs":
		s.defsDepth = max(0, s.defsDepth-1)
	case svgTextElements[name]:
		s.textDepth = max(0, s.textDepth-1)
	}
}

func (s *lossyScan) verdict() error {
	return refuseAtEnd(s.hasText, s.pending, s.referenced)
}

// svgViewBox reads the root's viewBox, or the zero box when it has none.
func svgViewBox(attrs []xml.Attr) [4]float64 {
	var box [4]float64
	for _, attr := range attrs {
		if attr.Name.Local != "viewBox" {
			continue
		}
		fields := strings.FieldsFunc(attr.Value, func(r rune) bool { return r == ',' || unicode.IsSpace(r) })
		if len(fields) != 4 {
			return [4]float64{}
		}
		for i, field := range fields {
			value, err := strconv.ParseFloat(field, 64)
			if err != nil {
				return [4]float64{}
			}
			box[i] = value
		}
	}
	return box
}

// svgIsArtboardClip reports a clip made of one rect that covers the viewBox.
func svgIsArtboardClip(children []xml.StartElement, box [4]float64) bool {
	if len(children) != 1 || strings.ToLower(children[0].Name.Local) != "rect" || box[2] <= 0 {
		return false
	}
	if svgHasTransform(children[0].Attr) {
		return false
	}
	rect := map[string]float64{}
	for _, attr := range children[0].Attr {
		if value, err := strconv.ParseFloat(attr.Value, 64); err == nil {
			rect[attr.Name.Local] = value
		}
	}
	return rect["x"] <= box[0] && rect["y"] <= box[1] &&
		rect["x"]+rect["width"] >= box[0]+box[2] && rect["y"]+rect["height"] >= box[1]+box[3]
}

func refuseAtEnd(hasText bool, pending []svgDefsElement, referenced map[string]bool) error {
	for _, element := range pending {
		if element.id != "" && referenced[element.id] {
			return fmt.Errorf("%w: the SVG uses <%s>", ErrSVGNotDrawable, element.name)
		}
	}
	if hasText {
		return ErrSVGTextNotOutlined
	}
	return nil
}

// svgHasTransform reports a transform attribute, which moves or scales a clip
// off the artboard.
func svgHasTransform(attrs []xml.Attr) bool {
	for _, attr := range attrs {
		if attr.Name.Local == "transform" {
			return true
		}
	}
	return false
}

func svgElementID(attrs []xml.Attr) string {
	for _, attr := range attrs {
		if attr.Name.Local == "id" {
			return attr.Value
		}
	}
	return ""
}

// noteSVGReferences records every id an attribute points at, through url(#id)
// in a paint, style or clip-path, or through href="#id".
func noteSVGReferences(attrs []xml.Attr, referenced map[string]bool) {
	for _, attr := range attrs {
		for _, m := range svgURLReference.FindAllStringSubmatch(attr.Value, -1) {
			referenced[m[1]] = true
		}
		if attr.Name.Local == "href" && strings.HasPrefix(attr.Value, "#") {
			referenced[attr.Value[1:]] = true
		}
	}
}

// svgMaxElements bounds how much document the rasterizer is asked to draw. A
// company's mark is tens of shapes; a file with tens of thousands is not a
// logo, and refusing it keeps one hostile favicon from occupying a deep-read
// worker for its whole timeout.
const svgMaxElements = 20_000

// rasterizeSVG draws a vector mark into pixels, fitting it inside an
// svgRasterEdge square with its aspect preserved. A document oksvg cannot
// parse — an exotic filter, a feature it owns no renderer for — is
// ErrUnsupported like any other undecodable candidate, and the caller moves on
// to the next one.
func rasterizeSVG(src []byte) (image.Image, error) {
	img, err := drawSVG(src)
	if err != nil && !errors.Is(err, ErrSVGTextNotOutlined) && !errors.Is(err, ErrSVGNotDrawable) {
		// A recognised SVG that fails is one the admin can still fix by exporting
		// a PNG; "use an SVG" would be the wrong advice.
		return nil, fmt.Errorf("%w: %w", ErrSVGNotDrawable, err)
	}
	return img, err
}

func drawSVG(src []byte) (image.Image, error) {
	if err := refuseUnsafeSVG(src); err != nil {
		return nil, err
	}
	if err := refuseLossySVG(src); err != nil {
		return nil, err
	}
	icon, err := oksvg.ReadIconStream(bytes.NewReader(splitImplicitPathRepeats(src)), oksvg.WarnErrorMode)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnsupported, err)
	}
	width, height := icon.ViewBox.W, icon.ViewBox.H
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("%w: the SVG declares a %gx%g viewBox", ErrUnsupported, width, height)
	}

	// Fit inside the square: the longest side becomes the edge, the other
	// scales with it, so the drawing keeps the shape its author gave it.
	scale := float64(svgRasterEdge) / max(width, height)
	target := image.Rect(0, 0, max(1, int(width*scale+0.5)), max(1, int(height*scale+0.5)))
	// The transform is set here rather than through icon.SetTarget, which
	// translates by the viewBox origin WITHOUT scaling that translation — so a
	// document whose viewBox does not start at 0,0 (a mark centred on its own
	// origin, say) draws shifted off the canvas by the unscaled remainder.
	// Shifting first and scaling the whole thing puts every viewBox origin in
	// the same place: the top-left corner of the raster.
	icon.Transform = rasterx.Identity.Scale(scale, scale).Translate(-icon.ViewBox.X, -icon.ViewBox.Y)

	// The canvas starts fully transparent, so a mark drawn without its own
	// background keeps one — the render layer supplies the backdrop.
	canvas := image.NewNRGBA(target)
	scanner := rasterx.NewScannerGV(target.Dx(), target.Dy(), canvas, target)
	icon.Draw(rasterx.NewDasher(target.Dx(), target.Dy(), scanner), 1)
	// Only SVG is held to this: Decode also reads stored PNGs, and a blank one
	// must still reach TrimTransparentPNG unchanged.
	if _, painted := paintedBounds(canvas); !painted {
		return nil, ErrSVGNotDrawable
	}
	return canvas, nil
}

// refuseUnsafeSVG rejects the documents the renderer cannot be trusted with,
// BEFORE it sees them.
//
// The renderer expands <use> by re-dispatching the referenced element through
// the same handler table, with no depth limit and no self-reference check: the
// 130 bytes
//
//	<defs><g id="a"><use href="#a"/></g></defs><use href="#a"/>
//
// exhaust the goroutine stack, and a stack overflow in Go is a FATAL error —
// no recover, no job-timeout, the process dies. Nesting <use> instead of
// recursing gets the same worker stuck exponentially. So <use> is refused
// outright rather than analysed: a company's mark is flat shapes, references
// buy a favicon nothing, and a rule with no graph to reason about has no
// subtle case to get wrong. A refused document is simply an unusable
// candidate — the chain moves on and the record keeps its monogram.
func refuseUnsafeSVG(src []byte) error {
	decoder := xml.NewDecoder(bytes.NewReader(src))
	decoder.Strict = false
	for elements := 0; ; elements++ {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("%w: the SVG is not readable as XML: %w", ErrUnsupported, err)
		}
		if elements > svgMaxElements {
			return fmt.Errorf("%w: the SVG carries more than %d elements", ErrUnsupported, svgMaxElements)
		}
		if start, ok := token.(xml.StartElement); ok && strings.EqualFold(start.Name.Local, "use") {
			return fmt.Errorf("%w: the SVG references other elements with <use>", ErrUnsupported)
		}
	}
}
