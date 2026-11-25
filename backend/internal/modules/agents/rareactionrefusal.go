// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// forRareAction is a refused call to a rare router action, with guidance that
// does not lean on the router's listing. That listing leaves the action's
// arguments out. The guidance names where each argument goes and where to read
// them. Any other error, or a call to any other action, is returned as it is.
func (r *Registry) forRareAction(called, target string, in json.RawMessage, err error) error {
	var bad *BadArgsError
	if called == target || !errors.As(err, &bad) {
		return err
	}
	routerSpec, _ := r.Spec(called)
	rare := r.rareActions(called, routerSpec.ActionField)
	var members map[string]json.RawMessage
	var action string
	if rare == nil || json.Unmarshal(in, &members) != nil || json.Unmarshal(members[routerSpec.ActionField], &action) != nil || !rare(action) {
		return err
	}
	targetSpec, _ := r.Spec(target)
	declared, schemaErr := readObjectSchema(target, targetSpec.InputSchema)
	if schemaErr != nil {
		return err
	}
	guided := *bad
	guided.Guidance = rareActionGuidance(called, routerSpec.ActionField, action, declared, members)
	return &guided
}

// rareActionGuidance says where a rare action's arguments go. It names the
// required ones the call left out or nested in request_body, and the members
// the action does not take.
func rareActionGuidance(router, field, action string, declared objectSchema, members map[string]json.RawMessage) string {
	var nestedIn map[string]json.RawMessage
	if _, takesBody := declared.Properties[routeBodyArg]; !takesBody && json.Unmarshal(members[routeBodyArg], &nestedIn) != nil {
		nestedIn = nil
	}
	var missing, nested, foreign []string
	for _, name := range declared.Required {
		if raw, ok := members[name]; ok && !isNull(raw) {
			continue
		}
		if _, ok := nestedIn[name]; ok {
			nested = append(nested, "`"+name+"`")
		} else {
			missing = append(missing, "`"+name+"`")
		}
	}
	for name := range members {
		_, takes := declared.Properties[name]
		if !takes && name != field && name != approvalIDArg && name != idempotencyKeyArg {
			foreign = append(foreign, "`"+name+"`")
		}
	}
	slices.Sort(foreign)
	var parts []string
	if len(nested) > 0 {
		parts = append(parts, fmt.Sprintf("move %s out of request_body: this action takes it at the top level, beside `%s`",
			strings.Join(nested, ", "), field))
	}
	if len(missing) > 0 {
		parts = append(parts, fmt.Sprintf("add %s at the top level, beside `%s`", strings.Join(missing, ", "), field))
	}
	if len(foreign) > 0 {
		parts = append(parts, fmt.Sprintf("leave out %s, which %s does not take", strings.Join(foreign, ", "), action))
	}
	return strings.Join(append(parts, fmt.Sprintf(
		"read_workspace with topic=actions, router=%s and action=%s lists every argument this action takes",
		router, action)), "; ")
}
