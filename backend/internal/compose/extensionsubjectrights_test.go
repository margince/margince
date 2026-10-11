// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/compliancedocs"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
	"github.com/margince/margince/backend/internal/shared/ports/subjectrightsrules"
	"github.com/margince/margince/backend/pkg/extension"
	pubdocs "github.com/margince/margince/backend/pkg/extension/compliancedocs"
	"github.com/margince/margince/backend/pkg/extension/messaging"
)

func TestRegisterExtensionsAppliesDeadlinesAndDocuments(t *testing.T) {
	err := RegisterExtensions(composableAll([]extension.Extension{{
		Name:          "rights-ok",
		Version:       "0.0.1",
		Jurisdictions: []jurisdiction.Pack{fakePack{code: "zn"}},
		SubjectRights: []subjectrightsrules.Rules{{
			Jurisdiction: "zn", Version: 2,
			Instruments: []messaging.Instrument{{Name: "Test Act 1/2026"}},
			Deadlines:   map[subjectrightsrules.RequestKind]subjectrightsrules.Deadline{"access": {Hours: 72}},
		}},
		Documents: []pubdocs.Document{{
			Jurisdiction: "zn", ID: "staff-notice", Title: "Staff notice", Language: "en",
			Audience: pubdocs.Employee, Kind: pubdocs.Template, Body: []byte("# Notice"), MediaType: "text/markdown",
		}},
	}}), nil, nil)
	if err != nil {
		t.Fatalf("RegisterExtensions: %v", err)
	}
	rules, ok := subjectrightsrules.For("zn")
	if !ok || rules.Version != 2 || rules.Deadlines["access"].Hours != 72 {
		t.Fatalf("subjectrightsrules.For(zn) = %+v, %v; want the declared version-2 set with a 72-hour access deadline", rules, ok)
	}
	if docs := compliancedocs.For("zn"); len(docs) != 1 || docs[0].ID != "staff-notice" {
		t.Fatalf("compliancedocs.For(zn) = %+v, want the one declared document", docs)
	}
}

// One unit owns everything for one code. A declaration for a code the unit
// carries no pack for is refused, whichever seam it comes through.
func TestADeclarationForAnotherUnitsJurisdictionIsRefused(t *testing.T) {
	for _, c := range []struct {
		name string
		unit extension.Extension
		want string
	}{
		{"messaging", extension.Extension{Name: "stray-msg", Version: "0.0.1",
			Messaging: []messagingrules.Rules{{Jurisdiction: "zo", Version: 1}}}, "messaging rules"},
		{"subject rights", extension.Extension{Name: "stray-rights", Version: "0.0.1",
			SubjectRights: []subjectrightsrules.Rules{{Jurisdiction: "zo", Version: 1}}}, "subject-rights rules"},
		{"documents", extension.Extension{Name: "stray-docs", Version: "0.0.1",
			Documents: []pubdocs.Document{{Jurisdiction: "zo", ID: "x", Title: "X", Language: "en",
				Audience: pubdocs.Operator, Kind: pubdocs.Record, Body: []byte("x"), MediaType: "text/markdown"}}}, "document x"},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := RegisterExtensions(composableAll([]extension.Extension{c.unit}), nil, nil)
			if err == nil || !strings.Contains(err.Error(), c.want) || !strings.Contains(err.Error(), "carries no jurisdiction pack") {
				t.Fatalf("err = %v, want the %s refused for carrying no pack of its own", err, c.want)
			}
		})
	}
	if _, ok := subjectrightsrules.For("zo"); ok {
		t.Fatal("a refused set's deadlines reached the registry")
	}
}
