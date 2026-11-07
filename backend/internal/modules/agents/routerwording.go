// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
)

// applyRouterWording sets the router's own description of each argument.
func applyRouterWording(routerName, field string, routerOwn objectSchema, properties map[string]argument) error {
	for name, wording := range routerOwn.Properties {
		_, known := properties[name]
		if _, extra := wording[schemaDescription]; len(wording) != 1 || !extra || (!known && name != field) {
			return fmt.Errorf("router %s may only describe an argument its targets take, and %s is not that", routerName, name)
		}
		if known {
			properties[name][schemaDescription] = wording[schemaDescription]
		}
	}
	return nil
}

// describeArguments names the choices an argument is for where only some take
// it, and the bound each choice holds it to.
func describeArguments(field string, choices []string, schemas []objectSchema, routerOwn objectSchema, properties map[string]argument, takers map[string][]string) {
	for name, choicesTaking := range takers {
		if _, worded := routerOwn.Properties[name]; !worded && field != "" && len(choicesTaking) < len(choices) {
			properties[name][schemaDescription] = scopedDescription(choicesTaking, properties[name][schemaDescription])
		}
	}
	for name, arg := range properties {
		if note := boundNotes(name, choices, schemas); note != "" {
			arg[schemaDescription] = noted(arg[schemaDescription], note)
		}
	}
}

// scopedDescription names the choices that take an argument, so a caller
// learns before the target refuses it that another choice does not.
func scopedDescription(choicesTaking []string, description json.RawMessage) json.RawMessage {
	var text string
	if description != nil && json.Unmarshal(description, &text) != nil {
		return description
	}
	scoped := strings.Join(slices.Sorted(slices.Values(choicesTaking)), ", ") + " only"
	if text != "" {
		scoped += ": " + text
	}
	return mustMarshal(scoped)
}

// boundNotes names the choices whose own bound on an argument is tighter than
// the loosest one the router advertises. Without it the schema promises a value
// those choices refuse. A choice that sets no bound is the loosest of all, so
// every bounded choice is then named. It is empty when all choices share a bound.
func boundNotes(name string, choices []string, schemas []objectSchema) string {
	var notes []string
	for _, b := range []struct {
		member, phrase string
		sign           float64
	}{{schemaMaximum, "at most", 1}, {schemaMinimum, "at least", -1}} {
		byBound := map[float64][]string{}
		loosest := math.Inf(-1)
		for i, schema := range schemas {
			arg, takes := schema.Properties[name]
			if !takes {
				continue
			}
			bound := math.Inf(1)
			if raw := arg[b.member]; raw != nil {
				if err := json.Unmarshal(raw, &bound); err != nil {
					continue
				}
				bound *= b.sign
				byBound[bound] = append(byBound[bound], choices[i])
			}
			loosest = max(loosest, bound)
		}
		for _, bound := range slices.Sorted(maps.Keys(byBound)) {
			if bound < loosest {
				notes = append(notes, fmt.Sprintf("%s %s for %s", b.phrase, strconv.FormatFloat(bound*b.sign, 'f', -1, 64),
					strings.Join(slices.Sorted(slices.Values(byBound[bound])), ", ")))
			}
		}
	}
	return strings.Join(notes, "; ")
}

// noted appends a note to an argument's description, which may be absent.
func noted(description json.RawMessage, note string) json.RawMessage {
	var text string
	if description != nil && json.Unmarshal(description, &text) != nil {
		return description
	}
	if text = strings.TrimRight(text, "."); text != "" {
		note = text + "; " + note
	}
	return mustMarshal(note + ".")
}
