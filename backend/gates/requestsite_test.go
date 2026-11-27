// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every site a model request names is one api/ai-tasks.yaml declares.
//
// The router refuses an undeclared site at call time. Without this gate, a typo in
// Request.Site is a feature that fails on its first call.

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai"
)

// declaredSites maps the site names the task contract declares, read from the
// table generated from it.
func declaredSites() map[string]bool {
	sites := map[string]bool{}
	for _, task := range ai.AllTasks() {
		for _, site := range ai.SitesFor(task) {
			sites[site.Name] = true
		}
	}
	return sites
}

func TestEveryRequestSiteIsDeclaredInTheTaskContract(t *testing.T) {
	t.Parallel()
	declared := declaredSites()
	named := 0
	for _, request := range everyModelRequest(t) {
		if !request.namesSite {
			continue
		}
		named++
		switch {
		case request.site == "":
			t.Errorf("%s: a model.Request names a Site this test cannot read; spell it as a string literal or a package constant", request.where)
		case !declared[request.site]:
			t.Errorf("%s: a model.Request names site %q, which no task in backend/api/ai-tasks.yaml declares", request.where, request.site)
		}
	}
	if named == 0 {
		t.Fatal("no model.Request in the tree names a Site, so this census read nothing")
	}
}

// The census recognises each shape a site may be spelled in, and reports the
// one it cannot read rather than passing over it.
func TestTheRequestSiteCensusSeesEveryShape(t *testing.T) {
	t.Parallel()
	const src = `package p
import port "github.com/margince/margince/backend/internal/shared/ports/model"
const named = "from_a_constant"
var _ = port.Request{Site: "literal"}
var _ = port.Request{Site: named}
var _ = port.Request{Site: computed()}
var _ = other.Request{Site: "not_a_model_request"}
`
	file := parseGateFixture(t, src)
	var got []string
	consts := func() map[string]string { return map[string]string{"named": "from_a_constant"} }
	for _, request := range requestSitesOf("planted.go", file, func() string { return src }, consts) {
		if request.namesSite {
			got = append(got, request.site)
		}
	}
	want := []string{"literal", "from_a_constant", ""}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("census read %q, want %q", got, want)
	}
}
