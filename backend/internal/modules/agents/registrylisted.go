// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"cmp"
	"encoding/json"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// Listed is a spec as the external listing words it. A verb only a router
// reaches is named by that router and action, the call a client can make. A
// Surface-B run is offered the verbs themselves and reads their own names.
func (r *Registry) Listed(spec mcp.ToolSpec) mcp.ToolSpec {
	return r.lister().listed(spec)
}

// ListedSpecs returns the registered specs with each listed one worded as the
// external listing words it, for a catalog that mirrors tools/list.
func (r *Registry) ListedSpecs() []mcp.ToolSpec {
	words := r.lister()
	specs := r.Specs()
	for i, spec := range specs {
		if !spec.Unlisted {
			specs[i] = words.listed(spec)
		}
	}
	return specs
}

// lister rewords descriptions for one listing: each verb a field router fronts
// becomes `router with field=action`. It is built once per listing.
type lister struct {
	calls map[string]string
	verbs *regexp.Regexp
}

func (r *Registry) lister() lister {
	calls := map[string]string{}
	for _, spec := range r.Specs() {
		if spec.Unlisted || spec.ActionField == "" {
			continue
		}
		for _, a := range spec.Actions {
			calls[a.Verb] = spec.Name + " with " + spec.ActionField + "=" + a.Action
		}
	}
	if len(calls) == 0 {
		return lister{}
	}
	// Longest first, so no verb is matched as the start of a longer one.
	verbs := slices.SortedFunc(maps.Keys(calls), func(a, b string) int { return cmp.Compare(len(b), len(a)) })
	for i, verb := range verbs {
		verbs[i] = regexp.QuoteMeta(verb)
	}
	return lister{calls: calls, verbs: regexp.MustCompile(`\b(` + strings.Join(verbs, "|") + `)\b`)}
}

func (l lister) listed(spec mcp.ToolSpec) mcp.ToolSpec {
	if l.verbs == nil {
		return spec
	}
	// A word the tool takes as one of its own values names that value, not the
	// verb spelled alike.
	own := enumWords(spec.InputSchema)
	reword := func(text string) string {
		return l.verbs.ReplaceAllStringFunc(text, func(verb string) string {
			if own[verb] {
				return verb
			}
			return l.calls[verb]
		})
	}
	spec.Description = reword(spec.Description)
	spec.InputSchema = rewordDescriptions(spec.InputSchema, reword)
	return spec
}

// enumWords collects every string an enum in the schema admits, at any depth.
func enumWords(schema json.RawMessage) map[string]bool {
	words := map[string]bool{}
	var doc any
	if json.Unmarshal(schema, &doc) != nil {
		return words
	}
	walkSchema(doc, func(node map[string]any) {
		if values, ok := node["enum"].([]any); ok {
			for _, v := range values {
				if word, ok := v.(string); ok {
					words[word] = true
				}
			}
		}
	})
	return words
}

// rewordDescriptions rewrites the descriptions in a schema, at any depth, and
// leaves every other member as it is.
func rewordDescriptions(schema json.RawMessage, reword func(string) string) json.RawMessage {
	var doc any
	if len(schema) == 0 || json.Unmarshal(schema, &doc) != nil {
		return schema
	}
	changed := false
	walkSchema(doc, func(node map[string]any) {
		if d, ok := node["description"].(string); ok {
			if r := reword(d); r != d {
				node["description"], changed = r, true
			}
		}
	})
	if !changed {
		return schema
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return schema
	}
	return out
}

// walkSchema visits every object in a decoded schema.
//
//craft:ignore naked-any a schema decoded without a type is any JSON value, and each is told apart below
func walkSchema(node any, visit func(map[string]any)) {
	switch n := node.(type) {
	case map[string]any:
		visit(n)
		for _, v := range n {
			walkSchema(v, visit)
		}
	case []any:
		for _, v := range n {
			walkSchema(v, visit)
		}
	}
}
