// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind falsification H3

package gates

// The label census, falsified.
//
// It passes on this tree, which is the state it exists to hold — and a gate
// that passes says nothing about what it would catch. These plant each of the
// three findings it reports, and each of the four image shapes it has to
// follow to see a key at all. The image shapes are the half that rots: a
// writer moving its map one line further from the call is an ordinary edit,
// and a reader that stopped following it would report the site as unreadable
// rather than wrong — a finding, but the wrong one, and one a ratification
// would then silence.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

func TestAKeyNoMapNamesIsReported(t *testing.T) {
	t.Parallel()
	findings := labelFindings(
		map[string]string{"stops_carried": "consent/stopcarry.go:CarryStopsTx"},
		map[string]bool{"name": true}, map[string]bool{"note": true}, neverRatified)

	if !mentionsAll(findings, "stops_carried", "no map names it") {
		t.Errorf("an unlabelled key was not reported: %v", findings)
	}
}

func TestAWordNoWriterEmitsIsReported(t *testing.T) {
	t.Parallel()
	findings := labelFindings(
		map[string]string{"note": "consent/qualifyingevent.go:RecordTx"},
		map[string]bool{}, map[string]bool{"note": true, "retired_key": true}, neverRatified)

	if !mentionsAll(findings, "retired_key", "no projected audit image carries it") {
		t.Errorf("a stale label was not reported: %v", findings)
	}
}

// A key both maps claim shows the CONTRACT column's word, which is right only
// while the two mean the same thing. Unratified, it is a finding; ratified, it
// is not — and the second half is what stops the register being decorative.
func TestAKeyBothMapsClaimIsReportedUntilItIsRatified(t *testing.T) {
	t.Parallel()
	emitted := map[string]string{"source": "consent/withdrawalpress.go:StopForCredentialTx"}
	contract := map[string]bool{"source": true}

	findings := labelFindings(emitted, contract, map[string]bool{}, neverRatified)
	if !mentionsAll(findings, "source", "already claims") {
		t.Errorf("an unratified collision was not reported: %v", findings)
	}

	if ratified := labelFindings(emitted, contract, map[string]bool{}, alwaysRatified); len(ratified) != 0 {
		t.Errorf("a ratified collision was still reported: %v", ratified)
	}
}

// The four shapes an audit image takes, each planted as the source a writer
// would actually have written.
func TestEveryImageShapeTheTreeUsesIsFollowed(t *testing.T) {
	t.Parallel()
	for _, shape := range []struct {
		name, body string
		want       string
	}{
		{
			name: "spelled at the call",
			body: `func w(ctx C, tx T) { storekit.AuditEvent(ctx, tx, "update", "contact", id, map[string]any{"spelled": 1}) }`,
			want: "spelled",
		},
		{
			name: "a local a few lines up",
			body: `func w(ctx C, tx T) {
				after := map[string]any{"local": 1}
				storekit.AuditEvent(ctx, tx, "update", "contact", id, after)
			}`,
			want: "local",
		},
		{
			name: "a parameter its own callers fill",
			body: `func w(ctx C, tx T, after map[string]any) {
				storekit.AuditEvent(ctx, tx, "update", "contact", id, after)
			}
			func caller(ctx C, tx T) { w(ctx, tx, map[string]any{"from_caller": 1}) }`,
			want: "from_caller",
		},
		{
			name: "a small builder's return",
			body: `func w(ctx C, tx T) { storekit.AuditEvent(ctx, tx, "update", "contact", id, image()) }
			func image() map[string]any { return map[string]any{"from_builder": 1} }`,
			want: "from_builder",
		},
	} {
		t.Run(shape.name, func(t *testing.T) {
			t.Parallel()
			if keys := keysOfPlantedWriter(t, shape.body); !slices.Contains(keys, shape.want) {
				t.Errorf("the reader did not follow this image: read %v, want %q", keys, shape.want)
			}
		})
	}
}

// keysOfPlantedWriter runs the gate's own reader over one planted source file.
func keysOfPlantedWriter(t *testing.T, body string) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "planted.go", "package planted\n\nimport \""+storekitPath+"\"\n\n"+body, 0)
	if err != nil {
		t.Fatalf("parsing the planted writer: %v", err)
	}
	index := callIndex{calls: map[string][]*ast.CallExpr{}, returns: map[string][]*ast.CompositeLit{}}
	ast.Inspect(file, func(n ast.Node) bool {
		if call, isCall := n.(*ast.CallExpr); isCall {
			if name, isPlain := call.Fun.(*ast.Ident); isPlain {
				index.calls[name.Name] = append(index.calls[name.Name], call)
			}
			return true
		}
		if fn, isFunc := n.(*ast.FuncDecl); isFunc && fn.Body != nil {
			index.returns[fn.Name.Name] = append(index.returns[fn.Name.Name], returnedComposites(fn.Body)...)
		}
		return true
	})
	var keys []string
	for _, site := range auditSitesIn(gatekit.ParsedFile{Path: "planted.go", File: file}) {
		keys = append(keys, site.imageKeys(t, nil, index)...)
	}
	return keys
}

func neverRatified(string) bool  { return false }
func alwaysRatified(string) bool { return true }

func mentionsAll(findings []string, parts ...string) bool {
	for _, finding := range findings {
		matched := true
		for _, part := range parts {
			matched = matched && strings.Contains(finding, part)
		}
		if matched {
			return true
		}
	}
	return false
}
