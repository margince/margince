// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package promptref

import "regexp"

var (
	recordID = regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	refShape = regexp.MustCompile(`\b[a-z]+[0-9]+\b`)
)

// Payload is one rendered payload with every record id in it written as a
// ref, one table per record kind, each ref minted where its id first
// appears. A site that renders a whole record as JSON rewrites it here
// rather than field by field, so no id the rendering carries is missed.
type Payload struct {
	text   string
	tables map[string]*Refs
	minted map[string]bool
}

// Rewrite writes each record id in text as a ref. kindOf names each id's
// record kind and prefixOf each kind's ref prefix. An id of no named kind is
// of kind "", and a kind with no prefix uses its own name.
func Rewrite(text string, kindOf, prefixOf map[string]string) *Payload {
	p := &Payload{tables: map[string]*Refs{}, minted: map[string]bool{}}
	p.text = recordID.ReplaceAllStringFunc(text, func(id string) string {
		kind := kindOf[id]
		table, ok := p.tables[kind]
		if !ok {
			prefix, named := prefixOf[kind]
			if !named {
				prefix = kind
			}
			table = New(prefix)
			p.tables[kind] = table
		}
		ref := table.For(id)
		p.minted[ref] = true
		return ref
	})
	return p
}

// Text is the payload as the model reads it.
func (p *Payload) Text() string { return p.text }

// ID reads ref back to the id of the record of kind it names, and false for a
// ref this payload never showed as that kind.
func (p *Payload) ID(kind, ref string) (string, bool) {
	table, ok := p.tables[kind]
	if !ok {
		return "", false
	}
	return table.ID(ref)
}

// Names reports whether text writes a ref this payload showed: a ref in
// prose is developer output, as an id is.
func (p *Payload) Names(text string) bool {
	for _, token := range refShape.FindAllString(text, -1) {
		if p.minted[token] {
			return true
		}
	}
	return false
}

// Refer writes each id this payload showed, wherever text spells it, as its
// ref: the answer as the model would have written it.
func (p *Payload) Refer(text string) string {
	return recordID.ReplaceAllStringFunc(text, func(id string) string {
		for _, table := range p.tables {
			if ref, ok := table.RefOf(id); ok {
				return ref
			}
		}
		return id
	})
}
