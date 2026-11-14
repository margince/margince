// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"strings"
	"testing"
)

// appUnitSource is a unit declaring the given App literal.
func appUnitSource(app string) string {
	return `package x

import "github.com/margince/margince/backend/pkg/extension"

func New() extension.Extension {
	return extension.Extension{
		Name:        "x",
		Version:     "0.1.0",
		Description: "A unit composed by a test.",
		App: ` + app + `,
	}
}
`
}

func appLiteral(icon, label, group, object, export string) string {
	return "&extension.App{Icon: extension." + icon + ", Label: \"" + label + "\", Group: extension." + group +
		", Object: \"" + object + "\", Export: \"" + export + "\"}"
}

func TestAnAppDerivesIntoTheManifest(t *testing.T) {
	derived, err := deriveSynthetic(t, "x", appUnitSource(appLiteral("IconPlug", "extX.app", "AppGroupWork", "ext_x_visit", "XApp")), gatedVerb("ext_x_visit"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(derived)
	for _, want := range []string{`"icon": "plug"`, `"label": "extX.app"`, `"group": "work"`, `"object": "ext_x_visit"`, `"export": "XApp"`} {
		if !strings.Contains(s, want) {
			t.Errorf("the manifest misses %s:\n%s", want, s)
		}
	}
}

// Generation refuses what the boot would, through the same published Validate.
func TestAnAppTheBootWouldRefuseIsRefusedAtGeneration(t *testing.T) {
	for name, app := range map[string]string{
		"an ungated object":      appLiteral("IconPlug", "extX.app", "AppGroupWork", "ext_x_other", "XApp"),
		"another unit's object":  appLiteral("IconPlug", "extX.app", "AppGroupWork", "ext_y_visit", "XApp"),
		"a lower-case export":    appLiteral("IconPlug", "extX.app", "AppGroupWork", "ext_x_visit", "xApp"),
		"an unknown field":       `&extension.App{Icon: extension.IconPlug, Label: "extX.app", Group: extension.AppGroupWork, Object: "ext_x_visit", Export: "XApp", Width: "wide"}`,
		"a value, not a pointer": `extension.App{Icon: extension.IconPlug}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := deriveSynthetic(t, "x", appUnitSource(app), gatedVerb("ext_x_visit"), gatedVerb("ext_y_visit")); err == nil {
				t.Fatal("generation accepted an app the boot refuses")
			}
		})
	}
}

func TestAnAppNeedsTheUnitToShipAFrontend(t *testing.T) {
	manifests := []derivedManifest{{Unit: extensionUnit{Name: "x"}, Manifest: unitManifest{
		appDecl: appDecl{App: &declaredApp{Icon: "plug", Label: "extX.app", Group: "work", Object: "ext_x_visit", Export: "XApp"}},
	}}}
	if err := checkAppFrontends(manifests); err == nil || !strings.Contains(err.Error(), "ships no frontend/package.json") {
		t.Fatalf("err = %v, want the missing-frontend refusal", err)
	}
}

func TestAnAppLabelMustExistInEveryLocale(t *testing.T) {
	manifests := []derivedManifest{{Unit: extensionUnit{Name: "x"}, Manifest: unitManifest{
		appDecl: appDecl{App: &declaredApp{Icon: "plug", Label: "extX.app", Group: "work", Object: "ext_x_visit", Export: "XApp"}},
	}}}
	full := map[string]map[string]string{}
	for _, locale := range composedLocales {
		full[locale] = map[string]string{"extX.app": "X"}
	}
	if err := checkAppLabels(manifests, full); err != nil {
		t.Fatalf("a label present in every locale was refused: %v", err)
	}
	delete(full[composedLocales[len(composedLocales)-1]], "extX.app")
	if err := checkAppLabels(manifests, full); err == nil {
		t.Fatal("a label missing from one locale was accepted")
	}
}
