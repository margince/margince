// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// Reading a unit's declared OAuth apps into the manifest.
//
// An operator should see an OAuth app before enabling the unit. It names the
// vendor an admin is sent to, what the consent asks for, and where the core
// sends the client secret. The extra consent parameters and OnGrant stay in
// the source.

import (
	"go/ast"
	"slices"
	"sort"

	"github.com/margince/margince/backend/pkg/extension"
)

// declaredOAuthApp is one OAuth app as the manifest publishes it, its scopes
// sorted so the encoding does not depend on declaration order.
type declaredOAuthApp struct {
	Provider string   `json:"provider"`
	Title    string   `json:"title"`
	AuthURL  string   `json:"auth_url"`
	TokenURL string   `json:"token_url"`
	Scopes   []string `json:"scopes"`
}

// readOAuthApps reads the OAuthApps slice literal, sorted by provider.
func (r *unitReader) readOAuthApps(expr ast.Expr, file *ast.File) ([]declaredOAuthApp, error) {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, r.errAt(expr, "OAuthApps must be a slice literal")
	}
	ext := importAlias(file, extensionPkgPath)
	out := make([]declaredOAuthApp, 0, len(lit.Elts))
	seen := map[string]bool{}
	for _, elt := range lit.Elts {
		app, err := r.readOAuthApp(elt, ext)
		if err != nil {
			return nil, err
		}
		if seen[app.Provider] {
			return nil, r.errAt(elt, "OAuth app provider %q declared twice", app.Provider)
		}
		seen[app.Provider] = true
		out = append(out, app)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Provider < out[j].Provider })
	return out, nil
}

// readOAuthApp reads one extension.OAuthApp literal and refuses what the
// published Validate refuses, so generation and boot accept the same apps.
func (r *unitReader) readOAuthApp(elt ast.Expr, ext string) (declaredOAuthApp, error) {
	lit, ok := elt.(*ast.CompositeLit)
	if !ok || (lit.Type != nil && !isSelector(lit.Type, ext, "OAuthApp")) {
		return declaredOAuthApp{}, r.errAt(elt, "an OAuthApps entry must be an extension.OAuthApp literal")
	}
	app := declaredOAuthApp{Scopes: []string{}}
	for _, e := range lit.Elts {
		kv, ok := e.(*ast.KeyValueExpr)
		if !ok {
			return declaredOAuthApp{}, r.errAt(e, "OAuthApp fields must be keyed")
		}
		k, ok := kv.Key.(*ast.Ident)
		if !ok {
			return declaredOAuthApp{}, r.errAt(kv.Key, "OAuthApp fields must be keyed by name")
		}
		var err error
		switch k.Name {
		case "Provider":
			app.Provider, err = r.stringLit(kv.Value, "OAuthApp.Provider")
		case "Title":
			app.Title, err = r.stringLit(kv.Value, "OAuthApp.Title")
		case "AuthURL":
			app.AuthURL, err = r.stringLit(kv.Value, "OAuthApp.AuthURL")
		case "TokenURL":
			app.TokenURL, err = r.stringLit(kv.Value, "OAuthApp.TokenURL")
		case "Scopes":
			app.Scopes, err = r.readScopes(kv.Value)
		case "AuthParams", "OnGrant":
			// Consent detail and behaviour: present in the source, absent here.
		default:
			err = r.errAt(kv, "OAuthApp field %s is not derivable by this generator", k.Name)
		}
		if err != nil {
			return declaredOAuthApp{}, err
		}
	}
	declared := extension.OAuthApp{Provider: app.Provider, Title: app.Title, AuthURL: app.AuthURL, TokenURL: app.TokenURL}
	if err := declared.Validate(); err != nil {
		return declaredOAuthApp{}, r.errPos(lit, "%v", err)
	}
	return app, nil
}

// readScopes reads the Scopes slice literal of plain strings, sorted.
func (r *unitReader) readScopes(expr ast.Expr) ([]string, error) {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, r.errAt(expr, "OAuthApp.Scopes must be a slice literal")
	}
	out := make([]string, 0, len(lit.Elts))
	for _, elt := range lit.Elts {
		scope, err := r.stringLit(elt, "OAuthApp.Scopes entry")
		if err != nil {
			return nil, err
		}
		out = append(out, scope)
	}
	slices.Sort(out)
	return out, nil
}
