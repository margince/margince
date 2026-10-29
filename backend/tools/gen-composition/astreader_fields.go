// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"go/ast"

	"github.com/margince/margince/backend/pkg/extension"
)

// readDeclaredField reads the Extension fields that hold plain declarations,
// those with no join to the contract. readExtensionField hands it every field
// it does not read itself.
func (r *unitReader) readDeclaredField(name string, kv *ast.KeyValueExpr, file *ast.File, m *unitManifest) error {
	var err error
	switch name {
	case "Entries":
		m.Entries, err = r.readEntries(kv.Value, file)
	case "Settings":
		m.Settings, err = r.readSettings(kv.Value, file)
	case "CompanyTypes":
		m.CompanyTypes, err = r.readCompanyTypes(kv.Value, file)
	case "RecordKinds":
		m.RecordKinds, err = r.readRecordKindDecls(kv.Value, file, extension.Name(r.unit))
	case "LinkTypes":
		m.LinkTypes, err = r.readLinkTypes(kv.Value, file)
	case "ReportCounts", "ReportBreakdowns":
		err = r.readReportsInto(m, name, kv.Value, file)
	case "SignalTypes":
		m.SignalTypes, err = r.readSignalTypes(kv.Value, file)
	case "Panels":
		m.Panels, err = r.readPanels(kv.Value, file)
	case "Inbound":
		m.Inbound, err = r.readInbound(kv.Value, file)
	case "SystemReads":
		m.SystemReads, err = r.readSystemReads(kv.Value, file)
	case "OAuthApps":
		m.OAuthApps, err = r.readOAuthApps(kv.Value, file)
	default:
		// Fail closed. An unknown field could be a future governed capability.
		// A manifest that silently omitted it would hide a request from the
		// operator.
		err = r.errAt(kv, "Extension field %s is not derivable by this generator — teach the manifest reader before declaring it", name)
	}
	return err
}
