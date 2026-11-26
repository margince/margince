// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// Reading a unit's declared REPORT COUNTS and BREAKDOWNS into the manifest: the
// named numbers the unit adds to Reports and Home. The boot checks the
// population and field against the unit's record kinds. This reader meets those
// declarations in another order, so it cannot.

import (
	"go/ast"
	"sort"
)

// declaredReport is one count or breakdown as the manifest carries it. By is
// empty on a count.
type declaredReport struct {
	Name       string `json:"name"`
	Label      string `json:"label"`
	Population string `json:"population"`
	By         string `json:"by,omitempty"`
}

// reportDecls are the named numbers a unit adds to Reports and Home. They sit
// in the manifest's top level through embedding.
type reportDecls struct {
	ReportCounts     []declaredReport `json:"report_counts,omitempty"`
	ReportBreakdowns []declaredReport `json:"report_breakdowns,omitempty"`
	// SignalTypes ride here because the manifest struct is at its size limit.
	SignalTypes []declaredSignalType `json:"signal_types,omitempty"`
}

// readReportsInto reads the one of the two slice fields that field names.
func (r *unitReader) readReportsInto(m *unitManifest, field string, expr ast.Expr, file *ast.File) error {
	var err error
	if field == "ReportCounts" {
		m.ReportCounts, err = r.readReports(expr, file, "ReportCount", false)
		return err
	}
	m.ReportBreakdowns, err = r.readReports(expr, file, "ReportBreakdown", true)
	return err
}

// readReports reads a ReportCounts or ReportBreakdowns slice literal. typeName
// is the extension type each entry must be, and allowBy says whether it may
// carry a By field.
func (r *unitReader) readReports(expr ast.Expr, file *ast.File, typeName string, allowBy bool) ([]declaredReport, error) {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, r.errAt(expr, "%ss must be a slice literal", typeName)
	}
	ext := importAlias(file, extensionPkgPath)
	out := make([]declaredReport, 0, len(lit.Elts))
	seen := map[string]bool{}
	for _, elt := range lit.Elts {
		rep, err := r.readReport(elt, ext, typeName, allowBy)
		if err != nil {
			return nil, err
		}
		if seen[rep.Name] {
			return nil, r.errPos(elt, "two %ss named %q", typeName, rep.Name)
		}
		seen[rep.Name] = true
		out = append(out, rep)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (r *unitReader) readReport(elt ast.Expr, ext, typeName string, allowBy bool) (declaredReport, error) {
	lit, ok := elt.(*ast.CompositeLit)
	if !ok || (lit.Type != nil && !isSelector(lit.Type, ext, typeName)) {
		return declaredReport{}, r.errAt(elt, "an entry must be an extension.%s literal", typeName)
	}
	var rep declaredReport
	for _, e := range lit.Elts {
		kv, ok := e.(*ast.KeyValueExpr)
		if !ok {
			return declaredReport{}, r.errAt(e, "%s fields must be keyed", typeName)
		}
		k, ok := kv.Key.(*ast.Ident)
		if !ok {
			return declaredReport{}, r.errAt(kv.Key, "%s fields must be keyed by name", typeName)
		}
		var err error
		switch {
		case k.Name == "Name":
			rep.Name, err = r.stringLit(kv.Value, typeName+".Name")
		case k.Name == "Label":
			rep.Label, err = r.stringLit(kv.Value, typeName+".Label")
		case k.Name == "Population":
			rep.Population, err = r.stringLit(kv.Value, typeName+".Population")
		case k.Name == "By" && allowBy:
			rep.By, err = r.stringLit(kv.Value, typeName+".By")
		default:
			err = r.errAt(kv, "%s field %s is not derivable by this generator", typeName, k.Name)
		}
		if err != nil {
			return declaredReport{}, err
		}
	}
	return rep, nil
}
