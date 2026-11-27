// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

import "testing"

// TestProblemMocksKeepTheStatusAndCodeOfEachTableRowTogether checks the emitted Problem contract.
func TestProblemMocksKeepTheStatusAndCodeOfEachTableRowTogether(t *testing.T) {
	t.Parallel()
	source := `function answer(status: number, code: string) { return Response.json({code}, {status}); }
 for (const [status,code] of [[401,"unauthorized"],[404,"not_found"]]) {answer(status,code);}`
	tree, err := parseTS(source, "pairs.test.ts")
	if err != nil {
		t.Fatal(err)
	}
	scan := scanProblemFile("pairs.test.ts", source, tree, map[string]bool{})
	pairs := map[problemStatusPair]bool{}
	for _, mock := range scan.mocks {
		if mock.status != 0 {
			pairs[problemStatusPair{mock.status, mock.code}] = true
		}
	}
	if !pairs[problemStatusPair{401, "unauthorized"}] || !pairs[problemStatusPair{404, "not_found"}] {
		t.Fatalf("lost the table's pairs: %v", pairs)
	}
	if pairs[problemStatusPair{404, "unauthorized"}] || pairs[problemStatusPair{401, "not_found"}] {
		t.Fatalf("mixed unrelated table rows: %v", pairs)
	}
}

// TestOneHelperCanCarryABodyCodeAndAFieldCode checks the emitted Problem contract.
func TestOneHelperCanCarryABodyCodeAndAFieldCode(t *testing.T) {
	t.Parallel()
	source := `function answer(code: string, fieldCode: string) { return Response.json({code,details:{errors:[{field:"name",code:fieldCode}]}},{status:422}); }
 answer("validation_error","too_long");`
	tree, err := parseTS(source, "fields.test.ts")
	if err != nil {
		t.Fatal(err)
	}
	scan := scanProblemFile("fields.test.ts", source, tree, map[string]bool{})
	body, field := false, false
	for _, mock := range scan.mocks {
		body = body || (!mock.field && mock.code == "validation_error")
		field = field || (mock.field && mock.code == "too_long")
	}
	if !body || !field {
		t.Fatalf("helper lost one kind of code: %#v", scan.mocks)
	}
}

// TestTheHTTPStatusWinsOverAProblemBodyStatus checks the emitted Problem contract.
func TestTheHTTPStatusWinsOverAProblemBodyStatus(t *testing.T) {
	t.Parallel()
	source := `Response.json({status:404,code:"unauthorized"},{status:401});`
	tree, err := parseTS(source, "transport.test.ts")
	if err != nil {
		t.Fatal(err)
	}
	scan := scanProblemFile("transport.test.ts", source, tree, map[string]bool{})
	if len(scan.mocks) != 1 || scan.mocks[0].status != 401 {
		t.Fatalf("HTTP status was lost: %#v", scan.mocks)
	}
}

func TestComputedStatusCallbacksKeepTheirCodeBranchesTogether(t *testing.T) {
	for name, source := range map[string]string{
		"callback":    `function statusFor(code:string){if(code==="unauthorized"){return 401;}return 404;} for(const code of ["unauthorized","not_found"]){Response.json({code},{status:statusFor(code)});}`,
		"conditional": `const status = missing ? 404 : 401; const code = missing ? "not_found" : "unauthorized"; Response.json({code},{status});`,
	} {
		t.Run(name, func(t *testing.T) {
			tree, err := parseTS(source, "computed.test.ts")
			if err != nil {
				t.Fatal(err)
			}
			scan := scanProblemFile("computed.test.ts", source, tree, map[string]bool{})
			pairs := map[problemStatusPair]bool{}
			for _, mock := range scan.mocks {
				if mock.statusUnknown {
					t.Fatalf("finite status was unresolved: %+v", mock)
				}
				pairs[problemStatusPair{mock.status, mock.code}] = true
			}
			if !pairs[problemStatusPair{401, "unauthorized"}] || !pairs[problemStatusPair{404, "not_found"}] || pairs[problemStatusPair{404, "unauthorized"}] || pairs[problemStatusPair{401, "not_found"}] {
				t.Fatalf("mixed computed branches: %v", pairs)
			}
		})
	}
}

func TestComputedProblemBranchesExposeBothWrongPairs(t *testing.T) {
	source := `const status = missing ? 404 : 401; const code = missing ? "unauthorized" : "not_found"; Response.json({code},{status});`
	tree, err := parseTS(source, "swapped.test.ts")
	if err != nil {
		t.Fatal(err)
	}
	scan := scanProblemFile("swapped.test.ts", source, tree, map[string]bool{})
	wrong := map[problemStatusPair]bool{}
	for _, mock := range scan.mocks {
		if mock.statusUnknown {
			t.Fatal("swapped branch was hidden as unresolved")
		}
		wrong[problemStatusPair{mock.status, mock.code}] = true
	}
	if !wrong[problemStatusPair{404, "unauthorized"}] || !wrong[problemStatusPair{401, "not_found"}] {
		t.Fatalf("wrong computed pairs were lost: %v", wrong)
	}
}
