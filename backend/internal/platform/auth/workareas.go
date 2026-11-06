// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"fmt"
	"slices"

	"github.com/margince/margince/backend/internal/platform/workarea"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// AreasAdmit reports whether p's passport lets it act where spec's call lands.
// A call placed in several areas is admitted when any of them is on. The door
// that knows the one operation holds it to that one. A call placed nowhere,
// such as an extension's tool, runs only for a passport set to all areas. The
// gate and the tool list both ask this, so what is offered is what is admitted.
func AreasAdmit(p principal.Principal, spec mcp.ToolSpec) bool {
	if p.Type != principal.PrincipalAgent || spec.SelfDescribing || !p.Areas.Limited {
		return true
	}
	return slices.ContainsFunc(spec.Areas, p.Areas.Includes)
}

// AreaRefusal is the refusal for a call AreasAdmit turns away, nil otherwise.
func AreaRefusal(p principal.Principal, spec mcp.ToolSpec) error {
	if AreasAdmit(p, spec) {
		return nil
	}
	area := ""
	if len(spec.Areas) > 0 {
		area = spec.Areas[0]
	}
	return &AreaOffError{Tool: spec.Name, Area: area}
}

// areaOffCode is the sentinel's code, which a missing scope answers too.
const areaOffCode = "scope_exceeds_grantor"

// AreaOffError refuses an agent whose passport leaves the call's area off, and
// names the area and where its user turns it on.
type AreaOffError struct {
	// Tool names the tool that asked, empty where the refusal is for a route.
	Tool string
	// Area is the area's key, empty for a call placed in no area.
	Area string
}

func (e *AreaOffError) Error() string {
	subject := "this call"
	if e.Tool != "" {
		subject = e.Tool
	}
	return fmt.Sprintf("gate: %s is in area %q, which the passport leaves off: %s", subject, e.Area, apperrors.ErrScopeExceeded)
}

// Unwrap keeps the refusal on the sentinel table's 403 row, beside a missing scope.
func (e *AreaOffError) Unwrap() error { return apperrors.ErrScopeExceeded }

// MessageFault keeps the sentinel's code and says what the user can do.
func (e *AreaOffError) MessageFault() (code, message string) {
	const allAreas = "ask the user to set the passport to all areas in Settings, under Passports"
	if e.Area == "" {
		return areaOffCode, "this passport may work only in the areas its user chose, and this call is in none of them; " + allAreas
	}
	if !workarea.IsChoosable(e.Area) {
		return areaOffCode, fmt.Sprintf(
			"this call is in the %q area, which only a passport set to all areas reaches; %s", workarea.NameOf(e.Area), allAreas)
	}
	return areaOffCode, fmt.Sprintf(
		"this passport may not work in the %q area; ask the user to turn that area on for this passport in Settings, under Passports",
		workarea.NameOf(e.Area))
}
