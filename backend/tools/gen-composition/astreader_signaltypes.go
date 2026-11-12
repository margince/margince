// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// Reading a unit's declared SIGNAL TYPES into the manifest. These are the kinds
// of signal the unit adds. Each has the evidence layout core checks against.
// The boot checks names and layouts. This reader only holds the declarations
// to the literal shape it can read.

import (
	"go/ast"
	"sort"
)

type declaredSignalField struct {
	Name     string `json:"name"`
	Label    string `json:"label"`
	Type     string `json:"type"`
	Required bool   `json:"required,omitempty"`
	Personal bool   `json:"personal,omitempty"`
}

type declaredSignalType struct {
	Kind     string                `json:"kind"`
	Label    string                `json:"label"`
	Evidence []declaredSignalField `json:"evidence,omitempty"`
}

// signalFieldTypes maps the published constants to the values they hold.
var signalFieldTypes = map[string]string{
	"SignalFieldText": "text", "SignalFieldNumber": "number", "SignalFieldBool": "bool",
}

func (r *unitReader) readSignalTypes(expr ast.Expr, file *ast.File) ([]declaredSignalType, error) {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, r.errAt(expr, "SignalTypes must be a slice literal")
	}
	ext := importAlias(file, extensionPkgPath)
	out := make([]declaredSignalType, 0, len(lit.Elts))
	seen := map[string]bool{}
	for _, elt := range lit.Elts {
		t, err := r.readSignalType(elt, ext)
		if err != nil {
			return nil, err
		}
		if seen[t.Kind] {
			return nil, r.errPos(elt, "two signal types named %q", t.Kind)
		}
		seen[t.Kind] = true
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Kind < out[j].Kind })
	return out, nil
}

func (r *unitReader) readSignalType(elt ast.Expr, ext string) (declaredSignalType, error) {
	var t declaredSignalType
	lit, ok := elt.(*ast.CompositeLit)
	if !ok || (lit.Type != nil && !isSelector(lit.Type, ext, "SignalTypeDeclaration")) {
		return t, r.errAt(elt, "an entry must be an extension.SignalTypeDeclaration literal")
	}
	for _, e := range lit.Elts {
		kv, key, err := r.keyedField(e, "SignalTypeDeclaration")
		if err != nil {
			return t, err
		}
		switch key {
		case "Kind":
			t.Kind, err = r.stringLit(kv.Value, "SignalTypeDeclaration.Kind")
		case "Label":
			t.Label, err = r.stringLit(kv.Value, "SignalTypeDeclaration.Label")
		case "Evidence":
			t.Evidence, err = r.readSignalFields(kv.Value, ext)
		default:
			err = r.errAt(kv, "SignalTypeDeclaration field %s is not derivable by this generator", key)
		}
		if err != nil {
			return t, err
		}
	}
	return t, nil
}

func (r *unitReader) readSignalFields(expr ast.Expr, ext string) ([]declaredSignalField, error) {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, r.errAt(expr, "SignalTypeDeclaration.Evidence must be a slice literal")
	}
	out := make([]declaredSignalField, 0, len(lit.Elts))
	for _, e := range lit.Elts {
		f, err := r.readSignalField(e, ext)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

func (r *unitReader) readSignalField(elt ast.Expr, ext string) (declaredSignalField, error) {
	var f declaredSignalField
	lit, ok := elt.(*ast.CompositeLit)
	if !ok || (lit.Type != nil && !isSelector(lit.Type, ext, "SignalEvidenceField")) {
		return f, r.errAt(elt, "an Evidence entry must be an extension.SignalEvidenceField literal")
	}
	for _, e := range lit.Elts {
		kv, key, err := r.keyedField(e, "SignalEvidenceField")
		if err != nil {
			return f, err
		}
		switch key {
		case "Name":
			f.Name, err = r.stringLit(kv.Value, "SignalEvidenceField.Name")
		case "Label":
			f.Label, err = r.stringLit(kv.Value, "SignalEvidenceField.Label")
		case "Type":
			f.Type, err = r.signalFieldType(kv.Value, ext)
		case "Required":
			f.Required, err = r.boolLit(kv.Value, "SignalEvidenceField.Required")
		case "Personal":
			f.Personal, err = r.boolLit(kv.Value, "SignalEvidenceField.Personal")
		default:
			err = r.errAt(kv, "SignalEvidenceField field %s is not derivable by this generator", key)
		}
		if err != nil {
			return f, err
		}
	}
	return f, nil
}

func (r *unitReader) signalFieldType(expr ast.Expr, ext string) (string, error) {
	if sel, ok := expr.(*ast.SelectorExpr); ok && isSelector(sel, ext, sel.Sel.Name) {
		if v, ok := signalFieldTypes[sel.Sel.Name]; ok {
			return v, nil
		}
	}
	return "", r.errAt(expr, "SignalEvidenceField.Type must be one of extension.SignalFieldText, SignalFieldNumber or SignalFieldBool")
}

func (r *unitReader) boolLit(expr ast.Expr, field string) (bool, error) {
	if id, ok := expr.(*ast.Ident); ok && (id.Name == "true" || id.Name == "false") {
		return id.Name == "true", nil
	}
	return false, r.errAt(expr, "%s must be true or false", field)
}

// keyedField splits one element of a struct literal into its key and value.
func (r *unitReader) keyedField(e ast.Expr, typeName string) (*ast.KeyValueExpr, string, error) {
	kv, ok := e.(*ast.KeyValueExpr)
	if !ok {
		return nil, "", r.errAt(e, "%s fields must be keyed", typeName)
	}
	k, ok := kv.Key.(*ast.Ident)
	if !ok {
		return nil, "", r.errAt(kv.Key, "%s fields must be keyed by name", typeName)
	}
	return kv, k.Name, nil
}
