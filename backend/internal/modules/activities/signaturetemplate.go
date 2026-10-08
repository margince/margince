// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The workspace's signature template, filled in with one sender's own values.
// An admin writes the layout once; every sender's mail carries it with their
// own name, title and phone in the placeholders.

import (
	"fmt"
	"html"
	"strings"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// SenderSignature is what a send knows about its sender's sign-off: their own
// plain-text signature, the values a template fills in, and the workspace's
// template. An empty Template means the workspace has none.
type SenderSignature struct {
	Body     string
	Title    string
	Phone    string
	Template string
}

// signatureValues fills the template's placeholders. Each value is the
// sender's own text, so it is escaped before it lands in markup.
type signatureValues struct {
	Name, Title, Phone string
}

// renderSignatureTemplate fills the template and returns it as sanitized
// markup and as the plain text the text/plain part carries.
func renderSignatureTemplate(template string, v signatureValues) (markup, text string, err error) {
	filled := strings.NewReplacer(
		"{name}", html.EscapeString(v.Name),
		"{title}", html.EscapeString(v.Title),
		"{phone}", html.EscapeString(v.Phone),
	).Replace(template)
	markup, err = sanitizeHTML(filled, signaturePolicy)
	if err != nil {
		return "", "", err
	}
	text, err = markupText(markup)
	return markup, text, err
}

// markupText is the text a reader of the markup sees, one line per line break
// or paragraph, with blank lines a placeholder left behind removed.
func markupText(markup string) (string, error) {
	body := &xhtml.Node{Type: xhtml.ElementNode, Data: atom.Body.String(), DataAtom: atom.Body}
	nodes, err := xhtml.ParseFragment(strings.NewReader(markup), body)
	if err != nil {
		return "", fmt.Errorf("activities: the signature does not parse as HTML: %w", err)
	}
	var out strings.Builder
	for _, node := range nodes {
		writeText(&out, node)
	}
	var lines []string
	for _, line := range strings.Split(out.String(), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n"), nil
}

func writeText(out *strings.Builder, node *xhtml.Node) {
	switch {
	case node.Type == xhtml.TextNode:
		out.WriteString(node.Data)
	case node.DataAtom == atom.Br:
		out.WriteString("\n")
	default:
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			writeText(out, child)
		}
		if node.DataAtom == atom.P {
			out.WriteString("\n")
		}
	}
}
