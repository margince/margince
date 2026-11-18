// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// Every text field the contract declares `x-single-line: true` must reach the
// request decoders as a generated field carrying the tag
// datasource.RejectUnsafeSingleLine reads. A declaration with no tag is a name
// that still accepts line breaks.
func TestEverySingleLineFieldIsTaggedForTheDecoders(t *testing.T) {
	t.Parallel()
	contract, err := gatekit.ReadFile(contractPath)
	if err != nil {
		t.Fatal(err)
	}
	generated, err := gatekit.ReadFile("internal/contracts/api_gen.go")
	if err != nil {
		t.Fatal(err)
	}
	declared := strings.Count(string(contract), "x-single-line: true")
	tagged := strings.Count(string(generated), datasource.SingleLineTag+`:\"true\"`) +
		strings.Count(string(generated), datasource.SingleLineTag+`:"true"`)
	if declared < 30 {
		t.Fatalf("the contract declares %d single-line fields; the scan no longer reads them", declared)
	}
	if tagged < declared {
		t.Errorf("the contract declares %d single-line fields and the generated structs tag %d: "+
			"a declaration on a schema the generator cannot tag is a promise no decoder keeps", declared, tagged)
	}
}
