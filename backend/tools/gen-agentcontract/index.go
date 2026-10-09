// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/tools/internal/oasnode"
)

// untagged heads the operations that declare no tag.
const untagged = "Other"

// renderIndex lists every kept operation on one table row, grouped under its
// first tag in the order the contract declares its tags, so an agent can pick
// a call without reading the whole contract.
func renderIndex(root *yaml.Node, ops []operation) []byte {
	var order []string
	if tags, ok := oasnode.Lookup(root, "tags"); ok {
		for _, tag := range tags.Content {
			order = append(order, oasnode.Scalar(tag, "name"))
		}
	}
	order = append(order, untagged)
	byTag := map[string][]operation{}
	for _, op := range ops {
		tag := op.tag
		if tag == "" {
			tag = untagged
		}
		byTag[tag] = append(byTag[tag], op)
	}

	var b strings.Builder
	b.WriteString("# Margince operations\n\n")
	b.WriteString("Every call a passport can make, grouped by area. Find the operationId in openapi.yaml for its parameters and answers.\n")
	for _, tag := range order {
		rows := byTag[tag]
		if len(rows) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n## %s\n\n", tag)
		b.WriteString("| Call | operationId | What it does | Permission | Runs |\n")
		b.WriteString("| --- | --- | --- | --- | --- |\n")
		for _, op := range rows {
			fmt.Fprintf(&b, "| `%s %s` | %s | %s | %s | %s |\n", op.method, op.path,
				oasnode.Scalar(op.node, "operationId"), cell(oasnode.Scalar(op.node, "summary")),
				op.permission(), op.runs())
		}
	}
	return []byte(b.String())
}

// permission is the passport permission a call needs at the agent gate. A
// read asks for none beyond the passport itself.
func (op operation) permission() string {
	if !op.mutating() {
		return "any"
	}
	return permissionNames[op.scope]
}

func (op operation) runs() string {
	if !op.mutating() {
		return "at once"
	}
	switch op.tier {
	case "confirmation_required":
		return "waits for approval"
	case "dynamic":
		return "may wait for approval"
	}
	return "at once"
}

// cell keeps a summary on its table row.
func cell(text string) string {
	return strings.ReplaceAll(strings.ReplaceAll(text, "\n", " "), "|", "/")
}
