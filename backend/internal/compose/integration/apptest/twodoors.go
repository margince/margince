// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package apptest

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/platform/blobstore"
)

// TwoDoors is one workspace with a signed-in admin and one passport. REST calls
// present the passport as a bearer, and the MCP client calls tools with it.
type TwoDoors struct {
	E      *AppEnv
	Bearer map[string]string
	MCP    *MCPClient
}

// SetupTwoDoors boots an app that serves MCP, with more options for a feature
// an operator switches on. It mints slug's admin a passport with scopes.
func SetupTwoDoors(t *testing.T, slug string, more []compose.Option, scopes ...string) *TwoDoors {
	t.Helper()
	e := SetupAppWithOriginOptions(t, func(origin string) []compose.Option {
		return append([]compose.Option{
			compose.WithMCPConnector(), compose.WithMCPResource(origin + "/mcp"),
			compose.WithBlobstore(blobstore.NewMemory()),
		}, more...)
	})
	BootstrapWorkspaceSession(t, e, "Two Doors", slug+"@fable.test", "Admin")
	bearer, _ := PassportWithID(t, e, slug+" agent", scopes...)
	return &TwoDoors{E: e, Bearer: bearer, MCP: NewMCPClient(e, strings.TrimPrefix(bearer["Authorization"], "Bearer "))}
}
