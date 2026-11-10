// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package promptref

import "regexp"

var recordID = regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

// Mint makes one table per record kind and mints a ref for every record id
// text carries, in the order text shows them. A site that renders a whole
// record as JSON mints here rather than field by field. Then Rewrite with
// these tables leaves no id the rendering carries. kindOf names each id's kind
// and prefixOf each kind's ref prefix. An id of no named kind is of kind "".
// A kind with no prefix uses its own name.
func Mint(text string, kindOf, prefixOf map[string]string) map[string]*Refs {
	tables := map[string]*Refs{}
	for _, id := range recordID.FindAllString(text, -1) {
		kind := kindOf[id]
		table, ok := tables[kind]
		if !ok {
			prefix, named := prefixOf[kind]
			if !named {
				prefix = kind
			}
			table = New(prefix)
			tables[kind] = table
		}
		table.For(id)
	}
	return tables
}
