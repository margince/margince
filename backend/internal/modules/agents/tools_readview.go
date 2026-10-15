// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// ReadView is one read a passport makes on REST that no other verb answers:
// a record's derived views, the workspace's reference lists, reporting objects.
// read_view runs that route's own handler, so the answer is the route's.
type ReadView struct {
	// Name is what a caller asks for, derived from the operation.
	Name string
	// Op is the operation's id, for the contract's own reading of the tool.
	Op string
	// Pattern is the route, path parameters in braces.
	Pattern string
	// PathParams and Query are what the route takes beside the page size.
	PathParams []string
	Query      []ViewQuery
}

// ViewQuery is one query parameter of a view's route, with its JSON type.
type ViewQuery struct {
	Name string
	Type string
}

// queryNames are the names of a view's query parameters.
func (v ReadView) queryNames() []string {
	names := make([]string, 0, len(v.Query))
	for _, q := range v.Query {
		names = append(names, q.Name)
	}
	return names
}

// queryType is the JSON type the view's route gives a query parameter.
func (v ReadView) queryType(name string) (string, bool) {
	for _, q := range v.Query {
		if q.Name == name {
			return q.Type, true
		}
	}
	return "", false
}

// ViewReader runs one view's route as the caller and answers its body.
type ViewReader func(ctx context.Context, view ReadView, path map[string]string, query url.Values) (json.RawMessage, error)

// RegisterReadViewTool joins read_view to the surface over the views the
// contract declares.
func RegisterReadViewTool(r *Registry, views []ReadView, read ViewReader) {
	r.Register(readView{views: views, read: read})
}

type readView struct {
	views []ReadView
	read  ViewReader
}

// ReadViewTool is the verb's name, which the contract's x-mcp-tool rows name.
const ReadViewTool = "read_view"

// viewArg names the view a call reads; pipelineArg gets its provenance said.
const (
	viewArg     = "view"
	pipelineArg = "pipeline_id"
)

// pageArgs are the paging controls every list view takes.
var pageArgs = []string{"cursor", "limit"}

// viewArgSchema publishes one argument with the type its routes give it. Where
// one view takes a list and another one value, the schema says one value. It
// names the views that take a list, which the handler accepts.
func viewArgSchema(arg string, byType map[string][]string) string {
	description := ""
	if arg == pipelineArg {
		description = "comes from " + pipelinesSource
	}
	arrays, takesList := byType[schemaArray]
	if takesList && len(byType) == 1 {
		return `{"type":"array","items":{"type":"string"}}`
	}
	jsonType := schemaString
	for t := range byType {
		if t != schemaArray {
			jsonType = t
		}
	}
	if takesList {
		description = "one value; a list of values for " + strings.Join(arrays, ", ")
	}
	if description == "" {
		return `{"type":` + strconv.Quote(jsonType) + `}`
	}
	return `{"type":` + strconv.Quote(jsonType) + `,"description":` + strconv.Quote(description) + `}`
}

func (t readView) Spec() mcp.ToolSpec {
	names := make([]string, 0, len(t.views))
	ops := make([]string, 0, len(t.views))
	types := map[string]map[string][]string{} // argument → JSON type → views
	note := func(arg, jsonType, view string) {
		if types[arg] == nil {
			types[arg] = map[string][]string{}
		}
		types[arg][jsonType] = append(types[arg][jsonType], view)
	}
	for _, v := range t.views {
		names = append(names, v.Name)
		ops = append(ops, v.Op)
		for _, p := range v.PathParams {
			note(p, schemaString, v.Name)
		}
		for _, q := range v.Query {
			note(q.Name, q.Type, v.Name)
		}
		note("cursor", schemaString, v.Name)
		note("limit", "integer", v.Name)
	}
	slices.Sort(names)
	properties := []string{strconv.Quote(viewArg) + `:{"type":"string","enum":` + mustJSON(names) + `}`}
	for _, arg := range slices.Sorted(maps.Keys(types)) {
		properties = append(properties, strconv.Quote(arg)+":"+viewArgSchema(arg, types[arg]))
	}
	return mcp.ToolSpec{
		Name: ReadViewTool, Title: "Read a view", Version: toolVersionV1,
		Description:   readViewCopy.render(),
		RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute,
		OpenAPIOp: strings.Join(ops, "/"),
		InputSchema: schema(`{"type":"object","required":["view"],"properties":{` + strings.Join(properties, ",") +
			`},"additionalProperties":false}`),
		OutputSchema: schema(`{"type":"object"}`),
	}
}

func (t readView) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	var args map[string]json.RawMessage
	if err := json.Unmarshal(in, &args); err != nil {
		return nil, &BadArgsError{Cause: fmt.Errorf("arguments must be a JSON object: %w", err)}
	}
	var name string
	if err := json.Unmarshal(args[viewArg], &name); err != nil || name == "" {
		return nil, &BadArgsError{Field: viewArg, Cause: errors.New("view names which read to run")}
	}
	idx := slices.IndexFunc(t.views, func(v ReadView) bool { return v.Name == name })
	if idx < 0 {
		return nil, &BadArgsError{Field: viewArg, Cause: fmt.Errorf("%q is not a view", name)}
	}
	view := t.views[idx]
	path := map[string]string{}
	query := url.Values{}
	for key, raw := range args {
		if key == viewArg {
			continue
		}
		if err := view.bind(key, raw, path, query); err != nil {
			return nil, err
		}
	}
	for _, param := range view.PathParams {
		if path[param] == "" {
			return nil, &BadArgsError{Field: param, Cause: fmt.Errorf("view %s needs %s", name, param)}
		}
	}
	return t.read(ctx, view, path, query)
}

// bind puts one argument where the view's route takes it. A list goes to a
// query parameter the route types as an array, one member per value.
func (v ReadView) bind(key string, raw json.RawMessage, path map[string]string, query url.Values) error {
	jsonType, isQuery := v.queryType(key)
	if jsonType == schemaArray {
		var members []json.RawMessage
		if err := json.Unmarshal(raw, &members); err == nil {
			for _, member := range members {
				value, err := operand(member)
				if err != nil {
					return &BadArgsError{Field: key, Cause: err}
				}
				query.Add(key, value)
			}
			return nil
		}
	}
	value, err := operand(raw)
	if err != nil {
		return &BadArgsError{Field: key, Cause: err}
	}
	switch {
	case slices.Contains(v.PathParams, key):
		path[key] = value
	case isQuery || slices.Contains(pageArgs, key):
		query.Set(key, value)
	default:
		return &BadArgsError{Field: key, Cause: fmt.Errorf("view %s takes %s", v.Name,
			strings.Join(append(append(slices.Clone(v.PathParams), v.queryNames()...), pageArgs...), ", "))}
	}
	return nil
}

// operand reads one argument as the string a route parameter spells.
func operand(raw json.RawMessage) (string, error) {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text, nil
	}
	var other any
	if err := json.Unmarshal(raw, &other); err != nil {
		return "", err
	}
	switch v := other.(type) {
	case bool:
		return strconv.FormatBool(v), nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	default:
		return "", errors.New("takes a string, number or boolean")
	}
}

func mustJSON(v []string) string {
	raw, err := json.Marshal(v)
	if err != nil {
		//craft:ignore panic-in-domain a list of strings always encodes; a failure is a composition defect
		panic(err)
	}
	return string(raw)
}
