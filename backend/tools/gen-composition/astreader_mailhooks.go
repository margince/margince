// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"context"
	"go/ast"
	"slices"

	"github.com/margince/margince/backend/pkg/extension"
)

// declaredMailHooks is the mail a unit's hooks read, as the manifest records it.
// A hook reads part of every captured mail, and the unit may send it on to its
// provider. So an operator is told before the unit runs.
type declaredMailHooks struct {
	Parties        bool     `json:"parties"`
	Deal           bool     `json:"deal"`
	Reads          []string `json:"reads"`
	BodyCharacters int      `json:"body_characters"`
	Reasons        []string `json:"reasons"`
}

// mailHooksDecl is embedded in unitManifest: what of each captured mail the
// unit's hooks read, nil for a unit with no hook.
type mailHooksDecl struct {
	MailHooks *declaredMailHooks `json:"mail_hooks,omitempty"`
}

// readMailHooks reads the MailHooks literal and holds it to the published
// Validate the boot runs. A unit that sets no hook gets no manifest entry.
func (r *unitReader) readMailHooks(expr ast.Expr, file *ast.File) (*declaredMailHooks, error) {
	ext := importAlias(file, extensionPkgPath)
	lit, ok := expr.(*ast.CompositeLit)
	if !ok || (lit.Type != nil && !isSelector(lit.Type, ext, "MailHooks")) {
		return nil, r.errAt(expr, "MailHooks must be an extension.MailHooks literal")
	}
	var hooks extension.MailHooks
	for _, e := range lit.Elts {
		if err := r.readMailHooksField(&hooks, e); err != nil {
			return nil, err
		}
	}
	if err := hooks.Validate(); err != nil {
		return nil, r.errPos(expr, "%v", err)
	}
	if hooks.Parties == nil && hooks.Deal == nil {
		return nil, nil
	}
	reasons := slices.Clone(hooks.Reasons)
	slices.Sort(reasons)
	return &declaredMailHooks{
		Parties: hooks.Parties != nil, Deal: hooks.Deal != nil,
		Reads: extension.MailViewReads(), BodyCharacters: extension.MailBodyCharacters,
		Reasons: reasons,
	}, nil
}

// readMailHooksField reads one keyed field. A hook is set when its value is
// anything but nil; the reader records that it is there, not what it does.
func (r *unitReader) readMailHooksField(hooks *extension.MailHooks, e ast.Expr) error {
	kv, ok := e.(*ast.KeyValueExpr)
	if !ok {
		return r.errAt(e, "MailHooks fields must be keyed")
	}
	k, ok := kv.Key.(*ast.Ident)
	if !ok {
		return r.errAt(kv.Key, "MailHooks fields must be keyed by name")
	}
	set := !isNilIdent(kv.Value)
	switch k.Name {
	case "Parties":
		if set {
			hooks.Parties = func(context.Context, extension.MailView, extension.MailParties) (extension.MailParties, error) {
				return extension.MailParties{}, nil
			}
		}
	case "Deal":
		if set {
			hooks.Deal = func(context.Context, extension.MailView, extension.MailDeal) (extension.MailDeal, error) {
				return extension.MailDeal{}, nil
			}
		}
	case "Reasons":
		reasons, err := r.readStringSlice(kv.Value, "MailHooks.Reasons")
		if err != nil {
			return err
		}
		hooks.Reasons = reasons
	default:
		return r.errAt(kv, "MailHooks field %s is not derivable by this generator", k.Name)
	}
	return nil
}
