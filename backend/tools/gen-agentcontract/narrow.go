// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/tools/internal/agentmarks"
	"github.com/margince/margince/backend/tools/internal/oasnode"
)

// narrowRequestBodies offers the skill's reader only what an agent may send,
// narrowed as the action catalog narrows it. A marked body becomes its own
// inline copy, so a component that is also an answer keeps every member there.
func narrowRequestBodies(root *yaml.Node, ops []operation) error {
	components, _ := oasnode.Lookup(root, "components")
	declared, _ := oasnode.Lookup(components, "schemas")
	//craft:ignore naked-any a JSON schema is walked as decoded YAML, whatever its shape
	var schemas map[string]any
	if declared != nil {
		if err := declared.Decode(&schemas); err != nil {
			return fmt.Errorf("reading components.schemas: %w", err)
		}
	}
	for _, op := range ops {
		body, _ := oasnode.Lookup(op.node, "requestBody")
		content, _ := oasnode.Lookup(body, "content")
		if content == nil {
			continue
		}
		for i := 1; i < len(content.Content); i += 2 {
			if err := narrowMedia(content.Content[i], schemas); err != nil {
				return fmt.Errorf("%s %s: %w", op.method, op.path, err)
			}
		}
	}
	return nil
}

// narrowMedia replaces one media type's schema with its narrowed copy when it
// carries a marker, and leaves it as written otherwise.
//
//craft:ignore naked-any a JSON schema is walked as decoded YAML, whatever its shape
func narrowMedia(media *yaml.Node, schemas map[string]any) error {
	schema, ok := oasnode.Lookup(media, "schema")
	if !ok {
		return nil
	}
	var decoded any
	if err := schema.Decode(&decoded); err != nil {
		return err
	}
	narrowed, marks, err := agentmarks.Narrow(decoded, schemas)
	if err != nil || (len(marks.HumanOnly) == 0 && len(marks.AgentValues) == 0) {
		return err
	}
	var copied yaml.Node
	if err := copied.Encode(narrowed); err != nil {
		return err
	}
	*schema = copied
	return nil
}
