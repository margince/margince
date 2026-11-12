// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"fmt"
	"net/url"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// RemoteImageBoxAttribute marks the box an undrawn image becomes, so the
// approval card knows to offer the drawn read. frontend/src/features/automations/remoteimagebox.ts
// mirrors it.
const RemoteImageBoxAttribute = "data-remote-image"

// The box an image with no size of its own takes.
const remoteImageBoxWidth, remoteImageBoxHeight = "96", "32"

// RewriteRemoteImages hands the first limit https image addresses of the
// markup to draw, and sets each src draw answers. Every other https image
// becomes a box of its size naming its alt text and host.
func RewriteRemoteImages(markup string, limit int, draw func(addresses []string) map[string]string) string {
	if !strings.Contains(markup, "<img") {
		return markup
	}
	body := &html.Node{Type: html.ElementNode, Data: atom.Body.String(), DataAtom: atom.Body}
	nodes, err := html.ParseFragment(strings.NewReader(markup), body)
	if err != nil {
		// The markup is the sanitizer's own output, and a strings.Reader never
		// fails, so this is unreachable; the page then shows what it can.
		return markup
	}
	for _, node := range nodes {
		body.AppendChild(node)
	}
	images := appendRemoteImages(nil, body)
	addresses := make([]string, 0, limit)
	for _, img := range images[:min(len(images), limit)] {
		addresses = append(addresses, attribute(img, "src"))
	}
	copies := draw(addresses)
	for i, img := range images {
		if i < len(addresses) && copies[addresses[i]] != "" {
			setAttribute(img, "src", copies[addresses[i]])
			continue
		}
		img.Parent.InsertBefore(remoteImagePlaceholder(img), img)
		img.Parent.RemoveChild(img)
	}
	var out strings.Builder
	for node := body.FirstChild; node != nil; node = node.NextSibling {
		if err := html.Render(&out, node); err != nil {
			return markup
		}
	}
	return out.String()
}

// RemoteImagePlaceholders returns the markup with every https image as its
// box, and fetches nothing.
func RemoteImagePlaceholders(markup string) string {
	return RewriteRemoteImages(markup, 0, func([]string) map[string]string { return nil })
}

func appendRemoteImages(images []*html.Node, node *html.Node) []*html.Node {
	if node.DataAtom == atom.Img && strings.HasPrefix(attribute(node, "src"), "https://") {
		images = append(images, node)
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		images = appendRemoteImages(images, child)
	}
	return images
}

// remoteImagePlaceholder is a box of the image's size that names it, so the
// preview says an image is there and where it comes from.
func remoteImagePlaceholder(img *html.Node) *html.Node {
	label := attribute(img, "src")
	if parsed, err := url.Parse(label); err == nil {
		label = parsed.Host
	}
	if alt := strings.TrimSpace(attribute(img, "alt")); alt != "" {
		label = alt + " (" + label + ")"
	}
	style := fmt.Sprintf("display:inline-block;box-sizing:border-box;width:%s;height:%s;overflow:hidden;"+
		"border:1px dashed #9aa3ad;color:#5f6b76;font:11px sans-serif;padding:2px",
		boxLength(attribute(img, "width"), remoteImageBoxWidth), boxLength(attribute(img, "height"), remoteImageBoxHeight))
	box := &html.Node{Type: html.ElementNode, Data: "span", DataAtom: atom.Span, Attr: []html.Attribute{
		{Key: "style", Val: style}, {Key: RemoteImageBoxAttribute, Val: ""},
	}}
	box.AppendChild(&html.Node{Type: html.TextNode, Data: label})
	return box
}

// boxLength turns a width or height of digits with an optional % into a CSS
// length. Any other value takes the fallback, so nothing else enters the style.
func boxLength(value, fallback string) string {
	if digits := strings.TrimSuffix(value, "%"); digits == "" || strings.Trim(digits, "0123456789") != "" {
		value = fallback
	}
	if strings.HasSuffix(value, "%") {
		return value
	}
	return value + "px"
}

func attribute(node *html.Node, key string) string {
	for _, attr := range node.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

func setAttribute(node *html.Node, key, value string) {
	for i := range node.Attr {
		if node.Attr[i].Key == key {
			node.Attr[i].Val = value
		}
	}
}
