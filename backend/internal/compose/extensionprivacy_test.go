// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
	"github.com/margince/margince/backend/pkg/extension/jurisdiction"
	"github.com/margince/margince/backend/pkg/extension/privacy"
)

func TestPrivacyDeclarationMustStayInTheUnitsNamespace(t *testing.T) {
	declared := func(tables ...privacy.Table) extension.Extension {
		return composable(extension.Extension{Name: "demo", Privacy: tables})
	}
	own := privacy.Table{Name: "ext_demo_note", Retention: jurisdiction.CommercialCorrespondence}
	foreign := privacy.Table{Name: "ext_other_note", Retention: jurisdiction.CommercialCorrespondence}

	if err := validateExtensionSet([]extension.Extension{declared(own)}); err != nil {
		t.Fatalf("a table in the unit's own namespace was refused: %v", err)
	}
	if err := validateExtensionSet([]extension.Extension{declared(foreign)}); err == nil {
		t.Error("a table in another unit's namespace was accepted")
	}
	if err := validateExtensionSet([]extension.Extension{declared(own, own)}); err == nil {
		t.Error("a table declared twice was accepted")
	}
}
