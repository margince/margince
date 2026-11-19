// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

// The app surface: a unit's own screen offered as a destination in the home
// side drawer, at `#/app/<unit>`. The unit names the glyph, the words, the
// group and the component. The core decides where the entry sits and fences
// it by the unit's RBAC object, so the unit cannot restyle the drawer.

import (
	"fmt"
	"slices"
	"strings"
)

// AppGroup names a heading of the home side drawer a unit's app is listed
// under. These are the headings the drawer already has.
type AppGroup string

// The drawer headings an app may sit under.
const (
	AppGroupRecords      AppGroup = "records"
	AppGroupWork         AppGroup = "work"
	AppGroupIntelligence AppGroup = "intelligence"
)

// AppGroups is every heading the drawer offers. The frontend's list mirrors
// it.
//
// Held by: TestTheFrontendAppGroupsMatchTheGoOnes (backend/gates/frontendentryslots_test.go)
func AppGroups() []AppGroup {
	return []AppGroup{AppGroupRecords, AppGroupWork, AppGroupIntelligence}
}

// Valid reports whether the drawer has this heading.
func (g AppGroup) Valid() bool {
	return slices.Contains(AppGroups(), g)
}

// App is the unit's own screen as a destination in the home side drawer. The
// unit's frontend package exports the component by name. The unit's screen
// under Settings (`#/ext/<unit>`, the default export) is unchanged.
type App struct {
	// Icon is a lucide glyph from the same curated list an Entry draws from.
	Icon EntryIcon
	// Label is a copy key from the unit's own frontend/i18n files, the name
	// shown beside the icon. The generator refuses one that is missing from
	// any locale.
	Label string
	Group AppGroup
	// Object is the unit's RBAC object a reader needs read on to be shown the
	// entry and open the route. The generator refuses one that none of the
	// unit's operations gates on.
	Object string
	// Export is the component's named export in the unit's frontend package.
	Export string
}

// Validate checks what one declaration can show on its own. The copy, export
// and operation checks need the unit's files, so the generator runs them.
func (a App) Validate(unit Name) error {
	if !a.Icon.Valid() {
		return fmt.Errorf("%w: app icon %q is not one the design system offers", ErrInvalid, a.Icon)
	}
	if !strings.HasPrefix(a.Label, "ext") || strings.ContainsAny(a.Label, " \t\n") {
		return fmt.Errorf("%w: app label %q must be a copy key from the unit's own i18n files", ErrInvalid, a.Label)
	}
	if !a.Group.Valid() {
		return fmt.Errorf("%w: app group %q is not a drawer heading (%s)", ErrInvalid, a.Group, joinAppGroups())
	}
	if !panelExport.MatchString(a.Export) {
		return fmt.Errorf("%w: app export %q must be a component name: a capital letter, then letters and digits", ErrInvalid, a.Export)
	}
	namespace, err := unit.Namespace()
	if err != nil {
		return err
	}
	if !strings.HasPrefix(a.Object, namespace+"_") {
		return fmt.Errorf("%w: app object %q must be one of the unit's own RBAC objects (%s_<object>)", ErrInvalid, a.Object, namespace)
	}
	return nil
}

func joinAppGroups() string {
	names := make([]string, 0, len(AppGroups()))
	for _, g := range AppGroups() {
		names = append(names, string(g))
	}
	return strings.Join(names, ", ")
}
