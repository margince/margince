// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind falsification H2

package gates

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// plantedColleagueJob is the shape forColleague has, with the bookkeeping id
// written down wrong. A system principal is named by an agent id, two hops
// from the literal that binds it.
const plantedColleagueJob = `package planted

import "github.com/margince/margince/backend/internal/shared/kernel/principal"

type colleagueJob struct{ work, books string }

var planted = colleagueJob{work: "agent:planted", books: "agent:planted-books"}

func bind(job colleagueJob) principal.Principal {
	return principal.Principal{Type: principal.PrincipalSystem, ID: job.books}
}

func forColleague(job colleagueJob) principal.Principal { return bind(job) }

func run() { forColleague(planted) }
`

// The census follows an id through a field of a declared struct value. With
// the field unread, the wrong id above passes; read, it is the mismatch.
func TestTheActorCensusReadsAnIDFromAColleagueJobsField(t *testing.T) {
	t.Parallel()
	const dir = "internal/plantedfixture"
	file, err := parser.ParseFile(token.NewFileSet(), "planted.go", plantedColleagueJob, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]*ast.File{dir + "/planted.go": file}
	binders := bindersAmong(candidatesIn(modulePath+"/"+dir, files))
	constants := map[string]string{}
	mismatched := false
	for _, named := range binderCalls(dir, files, constants, boundKinds(files, constants), binders) {
		if named.typ != "" && named.kind != "" && !kindAgreesWithType(named.kind, named.typ) {
			mismatched = true
		}
	}
	if !mismatched {
		t.Fatal("the census did not see a system principal bound with the agent id a colleagueJob value carries " +
			"in its books field: a wrong id there would pass the gate")
	}
}
