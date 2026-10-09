// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/tools/internal/oasnode"
)

// passportScheme is the security scheme a passport authenticates under. An
// operation whose security does not name it refuses a passport at the
// transport, whatever the agent gate would say.
const passportScheme = "bearerAuth"

// refusedToAgents are the x-agent-access classes the agent gate refuses to
// every passport, whatever its permissions.
var refusedToAgents = map[string]bool{"human-only": true, "auth-bootstrap": true}

// operation is one kept operation and the facts its x-* keys carried, read
// before those keys are stripped.
type operation struct {
	method, path string
	node         *yaml.Node
	tag          string
	scope, tier  string
	waitsOnModel string
}

// keepAgentOperations drops every operation a passport cannot call, and every
// path left with none. It returns the kept operations in source order, the
// tags they use and the security schemes they name.
func keepAgentOperations(root, paths *yaml.Node) ([]operation, map[string]bool, map[string]bool) {
	var kept []operation
	tags, schemes := map[string]bool{}, map[string]bool{}
	if paths == nil {
		return nil, tags, schemes
	}
	globalSecurity, _ := oasnode.Lookup(root, "security")
	keptPaths := paths.Content[:0]
	for i := 0; i+1 < len(paths.Content); i += 2 {
		path, item := paths.Content[i].Value, paths.Content[i+1]
		keptKeys := item.Content[:0]
		before := len(kept)
		for j := 0; j+1 < len(item.Content); j += 2 {
			key, value := item.Content[j], item.Content[j+1]
			if method, isOperation := oasnode.HTTPMethods[key.Value]; isOperation {
				if !passportCallable(value, globalSecurity) {
					continue
				}
				kept = append(kept, readOperation(method, path, value))
				collectNames(tags, value, "tags")
				addSchemeNames(schemes, value)
			}
			keptKeys = append(keptKeys, key, value)
		}
		item.Content = keptKeys
		if len(kept) > before {
			keptPaths = append(keptPaths, paths.Content[i], item)
		}
	}
	paths.Content = keptPaths
	return kept, tags, schemes
}

// passportCallable reports whether a passport may call the operation at all.
// The agent gate must not refuse its class. Its security, its own or else the
// document's, must accept a passport.
func passportCallable(op, globalSecurity *yaml.Node) bool {
	if refusedToAgents[agentAccess(op)] {
		return false
	}
	security, own := oasnode.Lookup(op, "security")
	if !own {
		security = globalSecurity
	}
	if security == nil {
		return false
	}
	for _, requirement := range security.Content {
		if _, ok := oasnode.Lookup(requirement, passportScheme); ok {
			return true
		}
	}
	return false
}

// agentAccess reads x-agent-access in both spellings: the core contract's
// scalar and an extension fragment's mapping, which names it under access.
func agentAccess(op *yaml.Node) string {
	value, ok := oasnode.Lookup(op, "x-agent-access")
	if !ok {
		return ""
	}
	if value.Kind == yaml.MappingNode {
		return oasnode.Scalar(value, "access")
	}
	return value.Value
}

func readOperation(method, path string, op *yaml.Node) operation {
	o := operation{method: method, path: path, node: op, waitsOnModel: oasnode.Scalar(op, "x-waits-on-model")}
	if tags, ok := oasnode.Lookup(op, "tags"); ok && len(tags.Content) > 0 {
		o.tag = tags.Content[0].Value
	}
	if tool, ok := oasnode.Lookup(op, "x-mcp-tool"); ok {
		o.scope, o.tier = oasnode.Scalar(tool, "scope"), oasnode.Scalar(tool, "tier")
	}
	return o
}

// collectNames adds every scalar in the sequence under key.
func collectNames(into map[string]bool, node *yaml.Node, key string) {
	seq, ok := oasnode.Lookup(node, key)
	if !ok {
		return
	}
	for _, item := range seq.Content {
		into[item.Value] = true
	}
}

// addSchemeNames adds the scheme names of node's security requirements.
func addSchemeNames(into map[string]bool, node *yaml.Node) {
	requirements, ok := oasnode.Lookup(node, "security")
	if !ok {
		return
	}
	for _, requirement := range requirements.Content {
		for i := 0; i < len(requirement.Content); i += 2 {
			into[requirement.Content[i].Value] = true
		}
	}
}
