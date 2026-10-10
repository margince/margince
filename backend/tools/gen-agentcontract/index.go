// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/tools/internal/oasnode"
)

// untagged heads the operations that declare no tag.
const untagged = "Other"

// renderIndex lists every kept operation on one table row, under its first tag.
// Tags come in the contract's order. An agent picks a call here without
// reading the whole contract.
func renderIndex(root *yaml.Node, ops []operation) []byte {
	var order []string
	if tags, ok := oasnode.Lookup(root, "tags"); ok {
		for _, tag := range tags.Content {
			order = append(order, oasnode.Scalar(tag, "name"))
		}
	}
	byTag := map[string][]operation{}
	for _, op := range ops {
		tag := op.tag
		if tag == "" {
			tag = untagged
		}
		byTag[tag] = append(byTag[tag], op)
	}
	order = append(order, undeclaredTags(order, byTag)...)
	order = append(order, untagged)

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

// permission is the passport permission a call needs. A read needs Read
// records, which a passport can be made without: the gate refuses every read
// of one that lacks it, whether or not the operation declares its scope.
func (op operation) permission() string {
	if op.scope == "" && !op.mutating() {
		return permissionNames["read"]
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

// undeclaredTags are the tags an operation names that the document's tag list
// does not, sorted. Their operations are as callable as any other, so the
// index lists them after the declared areas instead of leaving them out.
func undeclaredTags(declared []string, byTag map[string][]operation) []string {
	known := map[string]bool{untagged: true}
	for _, tag := range declared {
		known[tag] = true
	}
	var extra []string
	for tag := range byTag {
		if !known[tag] {
			extra = append(extra, tag)
		}
	}
	sort.Strings(extra)
	return extra
}
