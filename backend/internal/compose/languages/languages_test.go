// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package languages_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/languages"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/pkg/extension"
)

func unit(name string, declared ...extension.Language) extension.Extension {
	return extension.Extension{Name: extension.Name(name), Languages: declared}
}

func TestPreflightRefusesALanguageTheTableCannotHold(t *testing.T) {
	for _, c := range []struct {
		name     string
		language extension.Language
		wantErr  string
	}{
		{"an upper-case code", extension.Language{Code: "AR", Label: "Arabic"}, `"AR"`},
		{"a spelled-out name", extension.Language{Code: "arabic", Label: "Arabic"}, `"arabic"`},
		{"no label", extension.Language{Code: "ar"}, "no label"},
		{"a core language", extension.Language{Code: "de", Label: "Deutsch"}, "core already ships"},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := languages.Preflight([]extension.Extension{unit("egypt-pack", c.language)})
			if err == nil {
				t.Fatalf("a unit declaring %+v was admitted", c.language)
			}
			if !strings.Contains(err.Error(), `"egypt-pack"`) || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("the refusal %q does not name the unit and %q", err, c.wantErr)
			}
		})
	}
}

func TestPreflightAdmitsTwoUnitsDeclaringOneLanguage(t *testing.T) {
	arabic := extension.Language{Code: "ar", Label: "Arabic"}
	if err := languages.Preflight([]extension.Extension{unit("egypt-pack", arabic), unit("saudi-pack", arabic)}); err != nil {
		t.Fatalf("two jurisdiction packs that both write Arabic were refused: %v", err)
	}
}

func TestTheLanguageReportIsARegisterableTaskOverEveryCheckedColumn(t *testing.T) {
	tasks, err := datatask.Catalog([]datatask.Task{languages.DataTask()})
	if err != nil {
		t.Fatalf("the report is not a valid data task: %v", err)
	}
	if got, want := len(tasks[0].Steps), len(languages.Checked()); got != want {
		t.Errorf("the report has %d steps for %d checked columns", got, want)
	}
	if !slices.Contains(languages.Checked(), "consent_text_version.locale") {
		t.Error("the consent notice language is not among the checked columns")
	}
}
