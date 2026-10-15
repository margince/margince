// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	Query      []string
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

// pageArgs are the paging controls every list view takes.
var pageArgs = []string{"cursor", "limit"}

func (t readView) Spec() mcp.ToolSpec {
	names := make([]string, 0, len(t.views))
	ops := make([]string, 0, len(t.views))
	args := map[string]bool{"view": true}
	for _, v := range t.views {
		names = append(names, v.Name)
		ops = append(ops, v.Op)
		for _, a := range append(append(slices.Clone(v.PathParams), v.Query...), pageArgs...) {
			args[a] = true
		}
	}
	slices.Sort(names)
	properties := []string{`"view":{"type":"string","enum":` + mustJSON(names) + `}`}
	for _, name := range sortedKeys(args) {
		if name == "view" {
			continue
		}
		described := `{"type":"string"}`
		if name == "pipeline_id" {
			described = `{"type":"string","description":` + strconv.Quote("comes from "+pipelinesSource) + `}`
		}
		properties = append(properties, strconv.Quote(name)+":"+described)
	}
	return mcp.ToolSpec{
		Name: "read_view", Title: "Read a view", Version: toolVersionV1,
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
	if err := json.Unmarshal(args["view"], &name); err != nil || name == "" {
		return nil, &BadArgsError{Field: "view", Cause: errors.New("view names which read to run")}
	}
	idx := slices.IndexFunc(t.views, func(v ReadView) bool { return v.Name == name })
	if idx < 0 {
		return nil, &BadArgsError{Field: "view", Cause: fmt.Errorf("%q is not a view", name)}
	}
	view := t.views[idx]
	path := map[string]string{}
	query := url.Values{}
	for key, raw := range args {
		if key == "view" {
			continue
		}
		value, err := operand(raw)
		if err != nil {
			return nil, &BadArgsError{Field: key, Cause: err}
		}
		switch {
		case slices.Contains(view.PathParams, key):
			path[key] = value
		case slices.Contains(view.Query, key) || slices.Contains(pageArgs, key):
			query.Set(key, value)
		default:
			return nil, &BadArgsError{Field: key, Cause: fmt.Errorf("view %s takes %s", name,
				strings.Join(append(append(slices.Clone(view.PathParams), view.Query...), pageArgs...), ", "))}
		}
	}
	for _, param := range view.PathParams {
		if path[param] == "" {
			return nil, &BadArgsError{Field: param, Cause: fmt.Errorf("view %s needs %s", name, param)}
		}
	}
	return t.read(ctx, view, path, query)
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

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	slices.Sort(out)
	return out
}

func mustJSON(v []string) string {
	raw, err := json.Marshal(v)
	if err != nil {
		//craft:ignore panic-in-domain a list of strings always encodes; a failure is a composition defect
		panic(err)
	}
	return string(raw)
}
