// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// Reading a unit's declared record kinds into the manifest. A kind is one the
// unit adds beside core's own, under the short name it declared.

import (
	"go/ast"
	"go/token"
	"sort"

	"github.com/margince/margince/backend/pkg/extension"
)

// declaredRecordKind is one record kind as the manifest carries it.
type declaredRecordKind struct {
	Kind         string                      `json:"kind"`
	DisplayName  string                      `json:"display_name"`
	Table        string                      `json:"table"`
	Fields       []declaredRecordKindField   `json:"fields"`
	NaturalKey   []string                    `json:"natural_key"`
	Defaults     []declaredRecordKindDefault `json:"defaults,omitempty"`
	DeletePolicy extension.DeletePolicy      `json:"delete_policy,omitempty"`
	Timeline     *declaredKindTimeline       `json:"timeline,omitempty"`
}

type declaredKindTimeline struct {
	LabelField string                `json:"label_field"`
	AtColumn   string                `json:"at_column"`
	Subjects   []declaredKindSubject `json:"subjects"`
}

type declaredKindSubject struct {
	Entity string `json:"entity"`
	Column string `json:"column"`
}

type declaredRecordKindField struct {
	Name    string   `json:"name"`
	Label   string   `json:"label"`
	Options []string `json:"options,omitempty"`
}

type declaredRecordKindDefault struct {
	Role    string   `json:"role"`
	Actions []string `json:"actions"`
}

// readRecordKindDecls reads the RecordKinds slice literal. Each kind passes the
// published RecordKindDeclaration.Validate the boot runs.
func (r *unitReader) readRecordKindDecls(expr ast.Expr, file *ast.File, unit extension.Name) ([]declaredRecordKind, error) {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, r.errAt(expr, "RecordKinds must be a slice literal")
	}
	ext := importAlias(file, extensionPkgPath)
	out := make([]declaredRecordKind, 0, len(lit.Elts))
	seen := map[string]bool{}
	for _, elt := range lit.Elts {
		k, err := r.readRecordKindDecl(elt, ext)
		if err != nil {
			return nil, err
		}
		if err := k.declaration().Validate(unit); err != nil {
			return nil, r.errPos(elt, "%v", err)
		}
		if seen[k.Kind] {
			return nil, r.errPos(elt, "two record kinds named %q", k.Kind)
		}
		seen[k.Kind] = true
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Kind < out[j].Kind })
	return out, nil
}

// declaration rebuilds the published type from what was read, so the generator
// runs the unit's own grammar rather than a second copy of it.
func (k declaredRecordKind) declaration() extension.RecordKindDeclaration {
	d := extension.RecordKindDeclaration{
		Kind: k.Kind, DisplayName: k.DisplayName, Table: k.Table,
		NaturalKey: k.NaturalKey, DeletePolicy: k.DeletePolicy,
	}
	for _, f := range k.Fields {
		d.Fields = append(d.Fields, extension.RecordKindField{Name: f.Name, Label: f.Label, Options: f.Options})
	}
	if k.Timeline != nil {
		d.Timeline = &extension.RecordKindTimeline{LabelField: k.Timeline.LabelField, AtColumn: k.Timeline.AtColumn}
		for _, s := range k.Timeline.Subjects {
			d.Timeline.Subjects = append(d.Timeline.Subjects, extension.TimelineSubject{Entity: s.Entity, Column: s.Column})
		}
	}
	for _, def := range k.Defaults {
		actions := make([]extension.RbacAction, 0, len(def.Actions))
		for _, a := range def.Actions {
			actions = append(actions, extension.RbacAction(a))
		}
		d.Defaults = append(d.Defaults, extension.RbacDefault{Role: def.Role, Actions: actions})
	}
	return d
}

// readRecordKindDecl reads one extension.RecordKindDeclaration literal.
func (r *unitReader) readRecordKindDecl(elt ast.Expr, ext string) (declaredRecordKind, error) {
	lit, ok := elt.(*ast.CompositeLit)
	if !ok || (lit.Type != nil && !isSelector(lit.Type, ext, "RecordKindDeclaration")) {
		return declaredRecordKind{}, r.errAt(elt, "a RecordKinds entry must be an extension.RecordKindDeclaration literal")
	}
	var k declaredRecordKind
	for _, e := range lit.Elts {
		kv, ok := e.(*ast.KeyValueExpr)
		if !ok {
			return k, r.errAt(e, "RecordKindDeclaration fields must be keyed")
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			return k, r.errAt(kv.Key, "RecordKindDeclaration fields must be keyed by name")
		}
		if err := r.readRecordKindField(&k, key.Name, kv, ext); err != nil {
			return k, err
		}
	}
	return k, nil
}

func (r *unitReader) readRecordKindField(k *declaredRecordKind, name string, kv *ast.KeyValueExpr, ext string) error {
	var err error
	switch name {
	case "Kind":
		k.Kind, err = r.stringLit(kv.Value, "RecordKindDeclaration.Kind")
	case "DisplayName":
		k.DisplayName, err = r.stringLit(kv.Value, "RecordKindDeclaration.DisplayName")
	case "Table":
		k.Table, err = r.stringLit(kv.Value, "RecordKindDeclaration.Table")
	case "Fields":
		k.Fields, err = r.readKindFields(kv.Value, ext)
	case "NaturalKey":
		k.NaturalKey, err = r.readStringSlice(kv.Value, "RecordKindDeclaration.NaturalKey")
	case "Defaults":
		k.Defaults, err = r.readKindDefaults(kv.Value, ext)
	case "DeletePolicy":
		var policy string
		policy, err = r.constValue(kv.Value, ext)
		k.DeletePolicy = extension.DeletePolicy(policy)
	case "Timeline":
		k.Timeline, err = r.readKindTimeline(kv.Value, ext)
	default:
		err = r.errAt(kv, "RecordKindDeclaration field %s is not derivable by this generator", name)
	}
	return err
}

// readKindTimeline reads `&extension.RecordKindTimeline{...}`.
func (r *unitReader) readKindTimeline(expr ast.Expr, ext string) (*declaredKindTimeline, error) {
	unary, ok := expr.(*ast.UnaryExpr)
	if !ok || unary.Op != token.AND {
		return nil, r.errAt(expr, "RecordKindDeclaration.Timeline must be &extension.RecordKindTimeline{...}")
	}
	lit, ok := unary.X.(*ast.CompositeLit)
	if !ok || (lit.Type != nil && !isSelector(lit.Type, ext, "RecordKindTimeline")) {
		return nil, r.errAt(expr, "RecordKindDeclaration.Timeline must be an extension.RecordKindTimeline literal")
	}
	tl := &declaredKindTimeline{}
	for _, e := range lit.Elts {
		kv, ok := e.(*ast.KeyValueExpr)
		key, named := kv.Key.(*ast.Ident)
		if !ok || !named {
			return nil, r.errAt(e, "RecordKindTimeline fields must be keyed by name")
		}
		var err error
		switch key.Name {
		case "LabelField":
			tl.LabelField, err = r.stringLit(kv.Value, "RecordKindTimeline.LabelField")
		case "AtColumn":
			tl.AtColumn, err = r.stringLit(kv.Value, "RecordKindTimeline.AtColumn")
		case "Subjects":
			tl.Subjects, err = r.readTimelineSubjects(kv.Value, ext)
		default:
			err = r.errAt(kv, "RecordKindTimeline field %s is not derivable by this generator", key.Name)
		}
		if err != nil {
			return nil, err
		}
	}
	return tl, nil
}

func (r *unitReader) readTimelineSubjects(expr ast.Expr, ext string) ([]declaredKindSubject, error) {
	list, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, r.errAt(expr, "RecordKindTimeline.Subjects must be a slice literal")
	}
	out := make([]declaredKindSubject, 0, len(list.Elts))
	for _, e := range list.Elts {
		lit, ok := e.(*ast.CompositeLit)
		if !ok || (lit.Type != nil && !isSelector(lit.Type, ext, "TimelineSubject")) {
			return nil, r.errAt(e, "a Subjects entry must be an extension.TimelineSubject literal")
		}
		var s declaredKindSubject
		for _, f := range lit.Elts {
			kv, ok := f.(*ast.KeyValueExpr)
			key, named := kv.Key.(*ast.Ident)
			if !ok || !named {
				return nil, r.errAt(f, "TimelineSubject fields must be keyed by name")
			}
			var err error
			switch key.Name {
			case "Entity":
				s.Entity, err = r.stringLit(kv.Value, "TimelineSubject.Entity")
			case "Column":
				s.Column, err = r.stringLit(kv.Value, "TimelineSubject.Column")
			default:
				err = r.errAt(kv, "TimelineSubject field %s is not derivable by this generator", key.Name)
			}
			if err != nil {
				return nil, err
			}
		}
		out = append(out, s)
	}
	return out, nil
}

func (r *unitReader) readStringSlice(expr ast.Expr, field string) ([]string, error) {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, r.errAt(expr, "%s must be a slice literal", field)
	}
	out := make([]string, 0, len(lit.Elts))
	for _, e := range lit.Elts {
		s, err := r.stringLit(e, field)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func (r *unitReader) readKindFields(expr ast.Expr, ext string) ([]declaredRecordKindField, error) {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, r.errAt(expr, "RecordKindDeclaration.Fields must be a slice literal")
	}
	out := make([]declaredRecordKindField, 0, len(lit.Elts))
	for _, e := range lit.Elts {
		f, err := r.readKindField(e, ext)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

func (r *unitReader) readKindField(elt ast.Expr, ext string) (declaredRecordKindField, error) {
	var f declaredRecordKindField
	lit, ok := elt.(*ast.CompositeLit)
	if !ok || (lit.Type != nil && !isSelector(lit.Type, ext, "RecordKindField")) {
		return f, r.errAt(elt, "a Fields entry must be an extension.RecordKindField literal")
	}
	for _, e := range lit.Elts {
		kv, ok := e.(*ast.KeyValueExpr)
		if !ok {
			return f, r.errAt(e, "RecordKindField fields must be keyed")
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			return f, r.errAt(kv.Key, "RecordKindField fields must be keyed by name")
		}
		var err error
		switch key.Name {
		case "Name":
			f.Name, err = r.stringLit(kv.Value, "RecordKindField.Name")
		case "Label":
			f.Label, err = r.stringLit(kv.Value, "RecordKindField.Label")
		case "Options":
			f.Options, err = r.readStringSlice(kv.Value, "RecordKindField.Options")
		default:
			err = r.errAt(kv, "RecordKindField field %s is not derivable by this generator", key.Name)
		}
		if err != nil {
			return f, err
		}
	}
	return f, nil
}

func (r *unitReader) readKindDefaults(expr ast.Expr, ext string) ([]declaredRecordKindDefault, error) {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, r.errAt(expr, "RecordKindDeclaration.Defaults must be a slice literal")
	}
	out := make([]declaredRecordKindDefault, 0, len(lit.Elts))
	for _, e := range lit.Elts {
		d, err := r.readKindDefault(e, ext)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

func (r *unitReader) readKindDefault(elt ast.Expr, ext string) (declaredRecordKindDefault, error) {
	var d declaredRecordKindDefault
	lit, ok := elt.(*ast.CompositeLit)
	if !ok || (lit.Type != nil && !isSelector(lit.Type, ext, "RbacDefault")) {
		return d, r.errAt(elt, "a Defaults entry must be an extension.RbacDefault literal")
	}
	for _, e := range lit.Elts {
		kv, ok := e.(*ast.KeyValueExpr)
		if !ok {
			return d, r.errAt(e, "RbacDefault fields must be keyed")
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			return d, r.errAt(kv.Key, "RbacDefault fields must be keyed by name")
		}
		var err error
		switch key.Name {
		case "Role":
			d.Role, err = r.stringLit(kv.Value, "RbacDefault.Role")
		case "Actions":
			d.Actions, err = r.readActionNames(kv.Value, ext)
		default:
			err = r.errAt(kv, "RbacDefault field %s is not derivable by this generator", key.Name)
		}
		if err != nil {
			return d, err
		}
	}
	return d, nil
}

func (r *unitReader) readActionNames(expr ast.Expr, ext string) ([]string, error) {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, r.errAt(expr, "RbacDefault.Actions must be a slice literal")
	}
	out := make([]string, 0, len(lit.Elts))
	for _, e := range lit.Elts {
		a, err := r.constValue(e, ext)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}
