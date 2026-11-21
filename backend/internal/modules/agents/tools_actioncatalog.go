// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"fmt"
)

// ActionCatalog is what read_workspace topic=actions answers for router, read
// through the registered tool's own handler. docs/reference/mcp-info.json
// publishes it, and the MCP end-to-end stamps hash it per router.
func (r *Registry) ActionCatalog(router string) (json.RawMessage, error) {
	r.mu.RLock()
	tool, ok := r.tools[describeRouterActionsName]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("no route verb is registered, so %s is not served", describeRouterActionsName)
	}
	args, err := json.Marshal(map[string]string{"router": router})
	if err != nil {
		return nil, err
	}
	return tool.Handle(context.Background(), args)
}

// ActionCatalogRouters names the routers ActionCatalog answers for.
func (r *Registry) ActionCatalogRouters() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if describe, ok := r.tools[describeRouterActionsName].(describeRouterActions); ok {
		return describe.routers()
	}
	return nil
}
