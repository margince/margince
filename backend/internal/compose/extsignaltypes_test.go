// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
)

func TestAJobTickCannotFileASignal(t *testing.T) {
	_, _, err := extensionCore{unattended: true, unit: "tick-unit"}.Signals().Create(
		context.Background(), "loss", extension.SignalSubject{}, extension.SignalEvidence{}, "fp")
	if !errors.Is(err, extension.ErrForbidden) {
		t.Errorf("a tick's Create answered %v, want ErrForbidden", err)
	}
}

func TestPreflightSignalTypesRefusesWhatCoreCannotCarry(t *testing.T) {
	loss := extension.SignalTypeDeclaration{Kind: "loss", Label: "Loss"}
	cases := map[string]struct {
		types []extension.SignalTypeDeclaration
		want  string
	}{
		"fine":            {[]extension.SignalTypeDeclaration{loss}, ""},
		"a bad name":      {[]extension.SignalTypeDeclaration{{Kind: "Loss", Label: "Loss"}}, "not a valid name"},
		"a type twice":    {[]extension.SignalTypeDeclaration{loss, loss}, "twice"},
		"a bad evidence":  {[]extension.SignalTypeDeclaration{{Kind: "loss", Label: "L", Evidence: []extension.SignalEvidenceField{{Name: "a", Label: "A", Type: "blob"}}}}, "unknown type"},
		"two fields once": {[]extension.SignalTypeDeclaration{{Kind: "loss", Label: "L", Evidence: []extension.SignalEvidenceField{{Name: "a", Label: "A", Type: extension.SignalFieldText}, {Name: "a", Label: "B", Type: extension.SignalFieldText}}}}, "twice"},
	}
	for name, c := range cases {
		err := preflightSignalTypes([]extension.Extension{{Name: "demo", SignalTypes: c.types}})
		switch {
		case c.want == "" && err != nil:
			t.Errorf("%s: %v", name, err)
		case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
			t.Errorf("%s: err = %v, want one naming %q", name, err, c.want)
		}
	}
}
