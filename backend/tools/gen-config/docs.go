// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"fmt"
	"regexp"
	"strings"
)

// The markers around the generated region of the reference page. Everything
// outside them is written by hand.
const (
	docsBegin = "<!-- generated:config:start -->"
	docsEnd   = "<!-- generated:config:end -->"
)

// emptyCell is what a cell holds with no value: no flag, no default, or a
// secret's default.
const emptyCell = "—"

// spliceDocs replaces the region between the markers with the settings tables.
func spliceDocs(page string, settings []setting) (string, error) {
	head, rest, found := strings.Cut(page, docsBegin)
	if !found {
		return "", fmt.Errorf("gen-config: %s has no %s marker", docsPath, docsBegin)
	}
	_, tail, found := strings.Cut(rest, docsEnd)
	if !found {
		return "", fmt.Errorf("gen-config: %s has no %s marker after %s", docsPath, docsEnd, docsBegin)
	}
	return head + docsBegin + "\n" + settingsTables(settings) + docsEnd + tail, nil
}

// settingsTables is one table per section, top-level keys first.
func settingsTables(settings []setting) string {
	var b strings.Builder
	b.WriteString("<!-- Written by tools/gen-config from the settings registry: edit a setting's declaration, then run `make config-gen`. -->\n")
	for _, group := range groups(settings) {
		title := "Top-level keys"
		if name := group[0].section(); name != "" {
			title = "`" + name + "`"
		}
		fmt.Fprintf(&b, "\n### %s\n\n", title)
		b.WriteString("| Key | Env | Flag | Type | Default | Programs | Doc |\n")
		b.WriteString("|---|---|---|---|---|---|---|\n")
		for _, s := range group {
			b.WriteString(tableRow(s))
		}
	}
	b.WriteString("\n")
	return b.String()
}

func tableRow(s setting) string {
	flagCell := emptyCell
	if s.Flag() != "" {
		flagCell = codeSpan("--" + s.Flag())
	}
	cells := []string{
		codeSpan(s.Key()), codeSpan(s.Env()), flagCell, typeCell(s), defaultCell(s),
		programNames(s.programs), cellText(docText(s.Doc())),
	}
	return "| " + strings.Join(cells, " | ") + " |\n"
}

// typeCell names the type, its bounds and whether the value is a secret. It
// also says whether an admin may change it as a stored setting, and whether a
// data reset keeps that saved value.
func typeCell(s setting) string {
	cell := codeSpan(s.TypeName())
	switch lo, hi := s.shape.Min, s.shape.Max; {
	case lo != "" && hi != "":
		cell += fmt.Sprintf(", %s to %s", cellText(lo), cellText(hi))
	case lo != "":
		cell += ", at least " + cellText(lo)
	case hi != "":
		cell += ", at most " + cellText(hi)
	}
	if s.Secret() {
		cell += ", secret"
	}
	if s.Stored() {
		cell += ", stored"
	}
	if s.KeptByDataReset() {
		cell += ", kept by a reset"
	}
	return cell
}

// defaultCell never shows a secret's default; the registry refuses one, and
// the cell would not print it if it did not.
func defaultCell(s setting) string {
	text, ok := s.DefaultText()
	if s.Secret() || !ok {
		return emptyCell
	}
	if text == "" {
		return codeSpan(`""`)
	}
	return codeSpan(text)
}

// codeToken is a word of a doc that names something to type. That is a path
// (a leading slash, or two slashes), a flag, a reference, or an identifier
// with an underscore. A name such as Pub/Sub has one slash and stays a word.
var codeToken = regexp.MustCompile(`^(/|--|\$\{)|_|/.*/`)

// docText sets each such word of a doc in code. The hand-written prose of the
// page does the same, so a key or a path reads as one.
func docText(doc string) string {
	words := strings.Split(doc, " ")
	for i, word := range words {
		core := strings.TrimRight(strings.TrimLeft(word, "("), ".,;:)")
		if core == "" || strings.Contains(core, "`") || !codeToken.MatchString(core) {
			continue
		}
		at := strings.Index(word, core)
		words[i] = word[:at] + codeSpan(core) + word[at+len(core):]
	}
	return strings.Join(words, " ")
}

// codeSpan wraps text in a fence longer than any backtick run inside it, and
// escapes the table's own separator.
func codeSpan(text string) string {
	fence := "`"
	for strings.Contains(text, fence) {
		fence += "`"
	}
	pad := ""
	if strings.HasPrefix(text, "`") || strings.HasSuffix(text, "`") {
		pad = " "
	}
	return fence + pad + cellText(text) + pad + fence
}

// cellText keeps a value on one table row.
func cellText(text string) string {
	return strings.NewReplacer("|", `\|`, "\n", " ").Replace(text)
}
