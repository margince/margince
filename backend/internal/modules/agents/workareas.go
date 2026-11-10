// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"encoding/json"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/workarea"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// placedCall narrows spec's areas to the one the call's record type picks. A
// verb serving records in two areas is then admitted on the one this call
// names. A record type the table does not place leaves the call in no area, so
// it never borrows the union of the verb's areas.
func placedCall(t mcp.Tool, spec mcp.ToolSpec, args json.RawMessage) mcp.ToolSpec {
	typed, ok := t.(recordTypeNamer)
	if !ok || len(spec.Areas) < 2 {
		return spec
	}
	spec.Areas = nil
	if area, placed := workarea.OfRecordCall(spec.Name, typed.RecordTypeOf(args)); placed {
		spec.Areas = []string{area}
	}
	return spec
}

// inAreas reports whether p's passport admits verb's areas, as the gate does.
func (r *Registry) inAreas(p principal.Principal, verb string) bool {
	r.mu.RLock()
	spec, ok := r.specs[verb]
	r.mu.RUnlock()
	return ok && auth.AreasAdmit(p, spec)
}

// ReachableAreas lists the choosable areas p's grants reach, in the table's
// order. An area is reached when p holds the measured checks of a verb placed
// there alone. A verb the sweep measured asking no grant check is held by every
// member. A verb placed in several areas says nothing about any one.
func (r *Registry) ReachableAreas(p principal.Principal) []string {
	reached := map[string]bool{}
	r.mu.RLock()
	for name, spec := range r.specs {
		if len(spec.Areas) != 1 || spec.HumanOnly || spec.Gated || !r.holdsChecks(p, name) {
			continue
		}
		if key, free := workarea.SwitchOf(spec.Areas[0]); !free {
			reached[key] = true
		}
	}
	r.mu.RUnlock()
	var out []string
	for _, area := range workarea.Choosable() {
		if reached[area.Key] {
			out = append(out, area.Key)
		}
	}
	return out
}
