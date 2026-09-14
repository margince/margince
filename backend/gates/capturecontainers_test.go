// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every mail connector tells the sink where the provider FILED a message.
//
// A capture exclusion may name a container — a Gmail label, a Graph folder, an
// IMAP mailbox — and the rule is matched against NormalizedRecord.Containers on
// the pre-store path. A connector that does not fill it does not fail: it
// captures exactly as before, and the owner's rule quietly matches nothing.
// That is the shape this census exists for, because the contact who set the rule
// sees mail keep arriving and has no way to tell a rule that did not match from
// a connector that never asked.
//
// The corpus is derived from the tree rather than listed: a mail connector is a
// package that parses RFC822 through mailmap and writes through a Sink, so a
// fourth one is judged the day it lands. The obligation is that its capture
// path assigns Containers on the record it upserts.
//
// What this cannot see, stated so the next reader does not assume otherwise: it
// does not check that the value is RIGHT — that Gmail's label ids and not its
// display names reach the field, or that the provider prefix matches the
// connector. Those are each connector's own tests. What it holds is that the
// field is filled at all, which is the half that fails silently.

import (
	"go/ast"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	// mailParser is what makes a package a MAIL connector: it turns RFC822
	// bytes into the shared message model. A connector with no mail to parse —
	// a chat, a calendar — has no container notion and is not judged.
	mailParser = "mailmap"
	// containersField is the record member a connector fills.
	containersField = "Containers"
)

// connectorsWithoutContainers ratifies a mail connector that fills nothing.
// Each entry says why the provider offers no container, not merely that the
// work is unfinished.
var connectorsWithoutContainers = gatekit.Waive(map[string]string{})

func TestEveryMailConnectorReportsWhereTheProviderFiledTheMessage(t *testing.T) {
	t.Parallel()
	defer connectorsWithoutContainers.AssertAllMatched(t)

	byDir := map[string]bool{}
	fills := map[string]bool{}
	for _, src := range tierFiles(t, filepath.Join("internal", "modules", "capture")) {
		dir := filepath.ToSlash(filepath.Dir(src.Path))
		if !importsMailParser(src) {
			continue
		}
		byDir[dir] = true
		if assignsField(src, containersField) {
			fills[dir] = true
		}
	}
	// A census that can fail short has already failed: three mail connectors
	// ship today, and a corpus that found none would report a clean tree.
	if len(byDir) < wantMinimumMailConnectors {
		t.Fatalf("only %d mail connector package(s) found under internal/modules/capture, want at "+
			"least %d — the parser moved or was renamed, and this census is judging nothing",
			len(byDir), wantMinimumMailConnectors)
	}
	for dir := range byDir {
		if fills[dir] || connectorsWithoutContainers.Waived(t, dir) {
			continue
		}
		t.Errorf("%s parses mail and never sets NormalizedRecord.%s, so a capture exclusion naming "+
			"one of this provider's containers matches nothing. Fill it on the capture path from "+
			"the provider's own metadata (connector.Container qualifies it), or ratify the package "+
			"in connectorsWithoutContainers with the reason this provider has no container to name",
			dir, containersField)
	}
}

// wantMinimumMailConnectors is the anti-vacuity floor: gmail, graph and imap.
const wantMinimumMailConnectors = 3

// importsMailParser answers whether this file reaches the shared RFC822 model.
func importsMailParser(src tierFile) bool {
	for _, spec := range src.File.Imports {
		if strings.HasSuffix(strings.Trim(spec.Path.Value, `"`), "/"+mailParser) {
			return true
		}
	}
	return false
}

// assignsField answers whether any statement in the file assigns the named
// field, by any receiver — `rec.Containers = …`. The receiver is deliberately
// not matched: a connector may call its record anything, and a census that
// insisted on one name would report the others as missing.
func assignsField(src tierFile, field string) bool {
	found := false
	ast.Inspect(src.File, func(node ast.Node) bool {
		assign, isAssign := node.(*ast.AssignStmt)
		if !isAssign {
			return true
		}
		for _, target := range assign.Lhs {
			sel, isSel := target.(*ast.SelectorExpr)
			if isSel && sel.Sel.Name == field {
				found = true
				return false
			}
		}
		return true
	})
	return found
}
