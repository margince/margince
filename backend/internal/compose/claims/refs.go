// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package claims

import "github.com/margince/margince/backend/internal/shared/kernel/promptref"

// Refs names the records one prompt shows, one table per entity type, so a
// citation's entity_id is a short ref the model copied rather than a UUID.
type Refs map[string]*promptref.Refs

// Tables lists every table, for promptref.Rewrite over the payload the prompt
// shows.
func (r Refs) Tables() []*promptref.Refs {
	out := make([]*promptref.Refs, 0, len(r))
	for _, table := range r {
		out = append(out, table)
	}
	return out
}

// Read turns each citation's ref into the record id it names under its entity
// type. A ref of no shown record reads as no id, which grounds nothing, and a
// sentence whose text spells a ref is dropped: reader text never carries one.
func (r Refs) Read(sentences []Sentence) []Sentence {
	out := make([]Sentence, 0, len(sentences))
	for _, sentence := range sentences {
		if r.namedIn(sentence.Text) {
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
		evidence.EntityID = r.id(evidence.EntityType, evidence.EntityID)
		out[i] = evidence
	}
	return out
}

// namedIn reports whether text spells any ref of any table.
func (r Refs) namedIn(text string) bool {
	for _, table := range r {
		if _, named := table.NamedIn(text); named {
			return true
		}
	}
	return false
}

func (r Refs) id(entityType, ref string) string {
	table, ok := r[entityType]
	if !ok {
		return ""
	}
	id, shown := table.ID(ref)
	if !shown {
		return ""
	}
	return id
}
