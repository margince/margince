// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package searchplan

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/promptref"
)

// Refs replaces supplied record IDs before vocabulary reaches a model.
func Refs(question, vocabulary string) *promptref.Refs {
	tables := promptref.Mint(vocabulary+"\nQuestion: "+question, nil, map[string]string{"": "r"})
	if refs, ok := tables[""]; ok {
		return refs
	}
	return promptref.New("r")
}

// Read resolves supplied identities and applies production admission to the plan.
func Read(reply, question, vocabulary string) (Plan, error) {
	plan, err := Decode(reply)
	if err != nil {
		return plan, err
	}
	arguments, err := searchArgumentIDs(json.RawMessage(plan.Arguments), Refs(question, vocabulary), false)
	if err != nil {
		return plan, err
	}
	plan.Arguments = string(arguments)
	return Admit(plan, question, vocabulary)
}

// Only identity fields resolve refs; a name that happens to read "r1" stays a name.
func searchArgumentIDs(raw json.RawMessage, refs *promptref.Refs, identity bool) (json.RawMessage, error) {
	trimmed := strings.TrimSpace(string(raw))
	if strings.HasPrefix(trimmed, "{") {
		return searchObjectIDs(raw, refs)
	}
	if strings.HasPrefix(trimmed, "[") {
		var items []json.RawMessage
		if err := json.Unmarshal(raw, &items); err != nil {
			return nil, err
		}
		for index, item := range items {
			expanded, err := searchArgumentIDs(item, refs, identity)
			if err != nil {
				return nil, err
			}
			items[index] = expanded
		}
		return json.Marshal(items)
	}
	if !identity || trimmed == jsonNull {
		return raw, nil
	}
	var ref string
	if err := json.Unmarshal(raw, &ref); err != nil {
		return nil, err
	}
	id, ok := refs.ID(ref)
	if !ok {
		return nil, fmt.Errorf("identity %q is not a supplied record ref", ref)
	}
	return json.Marshal(id)
}

func searchObjectIDs(raw json.RawMessage, refs *promptref.Refs) (json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	var predicateField string
	if field, ok := fields["field"]; ok {
		if err := json.Unmarshal(field, &predicateField); err != nil {
			return nil, err
		}
	}
	for key, value := range fields {
		identity := searchIdentityField(key) || ((key == "value" || key == "values") && searchIdentityField(predicateField))
		expanded, err := searchArgumentIDs(value, refs, identity)
		if err != nil {
			return nil, err
		}
		fields[key] = expanded
	}
	return json.Marshal(fields)
}

func searchIdentityField(field string) bool {
	return field == "id" || strings.HasSuffix(field, "_id")
}
