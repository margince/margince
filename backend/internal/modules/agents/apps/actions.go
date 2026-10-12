// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package apps

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"sync"
)

// actionsJSON is what each view may ask its host to run, AUTHORED in
// frontend/src/mcp-apps/actions.json and copied by `make -C backend
// mcp-apps-vocab`, for the reason forbiddenJSON is. The view imports the same
// file to declare its own list, so the build, this admission check and the
// tool listing's visibility all read one source.
//
//go:embed actions.json
var actionsJSON []byte

// viewActions decodes the list once. A file that does not decode yields no
// actions, which makes admission strictest, and the test of the decode names
// the fault instead of letting it pass as a view with none.
var viewActions = sync.OnceValues(func() (map[string][]string, error) {
	var byView map[string][]string
	if err := json.Unmarshal(actionsJSON, &byView); err != nil {
		return nil, fmt.Errorf("crmapps: decoding the view actions: %w", err)
	}
	return byView, nil
})

// viewDir is the directory a view is authored in, which is its document's file
// name without the extension.
func viewDir(uri string) string {
	return strings.TrimSuffix(path.Base(uri), ".html")
}

// actions answers the tools this view may ask its host to run.
func (v view) actions() []string {
	byView, err := viewActions()
	if err != nil {
		return nil
	}
	return byView[viewDir(v.uri)]
}

// ActionTools names the tools some view may ask its host to run. The tool
// listing keeps exactly these reachable from a view, and a host that
// honours `visibility` refuses a view's call to any other.
func ActionTools() map[string]struct{} {
	out := map[string]struct{}{}
	for _, v := range catalog {
		for _, name := range v.actions() {
			out[name] = struct{}{}
		}
	}
	return out
}
