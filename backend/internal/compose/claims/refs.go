// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package claims

import "github.com/margince/margince/backend/internal/shared/kernel/promptref"

// refPrefixes is the ref prefix of each record kind a sentence may cite. Ids
// of no citable kind read r1, r2, … and cite nothing.
var refPrefixes = map[string]string{
	"deal": "d", "activity": "a", "contact": "c", "lead": "l", "company": "co",
	"profile_field": "pf", "fact": "fa", "participant": "p", "document": "doc", "thread": "t",
	"": "r",
}

// Refs names the records one prompt shows, one table per entity type. A
// citation's entity_id is then a short ref the model copied, not a UUID.
type Refs map[string]*promptref.Refs

// MintRefs mints a ref for every record id payload carries, of the kind known
// gives it. The request shows payload through Show, and the parse reads the
// reply's citations back through Read.
func MintRefs(payload string, known map[Evidence]string) Refs {
	kindOf := make(map[string]string, len(known))
	for cited := range known {
		kindOf[cited.EntityID] = cited.EntityType
	}
	return MintKinds(payload, kindOf)
}

// MintKinds is MintRefs for a site that keys its citable records by id alone,
// with the kind each one is cited as.
func MintKinds(payload string, kindOf map[string]string) Refs {
	return Refs(promptref.Mint(payload, kindOf, refPrefixes))
}

// Show writes every id the tables hold, wherever text spells it, as its ref.
// That gives the payload as the model reads it, or a scripted answer as a
// model writes it.
func (r Refs) Show(text string) string {
	tables := make([]*promptref.Refs, 0, len(r))
	for _, table := range r {
		tables = append(tables, table)
	}
	return promptref.Rewrite(text, tables...)
}

// Read turns each citation's ref into the record id it names under its entity
// type. A ref of no shown record reads as no id, which grounds nothing. A
// sentence whose text spells a ref is dropped, because reader text never
// carries one.
func (r Refs) Read(sentences []Sentence) []Sentence {
	out := make([]Sentence, 0, len(sentences))
	for _, sentence := range sentences {
		if r.NamedIn(sentence.Text) {
			continue
		}
		sentence.Evidence = r.ReadEvidence(sentence.Evidence)
		out = append(out, sentence)
	}
	return out
}

// ReadEvidence is Read for one list of citations.
func (r Refs) ReadEvidence(cited []Evidence) []Evidence {
	if cited == nil {
		return nil
	}
	out := make([]Evidence, len(cited))
	for i, evidence := range cited {
		evidence.EntityID, _ = r.ID(evidence.EntityType, evidence.EntityID)
		out[i] = evidence
	}
	return out
}

// NamedIn reports whether text spells any ref of any table.
func (r Refs) NamedIn(text string) bool {
	for _, table := range r {
		if _, named := table.NamedIn(text); named {
			return true
		}
	}
	return false
}

// ID reads ref back to the id of the record it names under entityType, and
// false for a ref never shown as that type.
func (r Refs) ID(entityType, ref string) (string, bool) {
	table, ok := r[entityType]
	if !ok {
		return "", false
	}
	return table.ID(ref)
}
