// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"slices"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/tools/internal/oasnode"
)

// narrowForAgents offers the skill's reader only what an agent may send. A
// member marked x-human-only leaves its object. A member marked x-agent-values
// keeps only those values in its enum. The agent gate refuses
// the rest, so a schema that listed it would describe a call that fails.
func narrowForAgents(n *yaml.Node) {
	if n.Kind == yaml.MappingNode {
		if props, ok := oasnode.Lookup(n, "properties"); ok && props.Kind == yaml.MappingNode {
			dropped := narrowProperties(props)
			if required, has := oasnode.Lookup(n, "required"); has {
				required.Content = slices.DeleteFunc(required.Content, func(name *yaml.Node) bool {
					return slices.Contains(dropped, name.Value)
				})
			}
		}
	}
	for _, child := range n.Content {
		narrowForAgents(child)
	}
}

// narrowProperties applies both markers to one properties map and names the
// members it dropped.
func narrowProperties(props *yaml.Node) []string {
	var dropped []string
	kept := props.Content[:0]
	for i := 0; i+1 < len(props.Content); i += 2 {
		name, member := props.Content[i], props.Content[i+1]
		if oasnode.Scalar(member, "x-human-only") == "true" {
			dropped = append(dropped, name.Value)
			continue
		}
		if allowed, marked := oasnode.Lookup(member, "x-agent-values"); marked {
			if enum, has := oasnode.Lookup(member, "enum"); has {
				enum.Content = allowed.Content
			}
		}
		kept = append(kept, name, member)
	}
	props.Content = kept
	return dropped
}
