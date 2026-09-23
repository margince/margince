// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deployconfig

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// SeedSubtreeBytes re-encodes one seed subtree on its own, so a caller that
// owns the type it becomes can parse it without this package knowing that type.
//
// Aliases are resolved first. A deployment may define `&defaults` elsewhere in
// its config and reference it inside a seed, which is ordinary YAML — but
// marshalling the subtree alone emits the alias with no anchor in scope, and
// the re-parse dies with "unknown anchor 'd' referenced": an internal parser
// detail handed to an operator whose file is valid. Resolving covers both
// idioms, a plain `*ref` and a `<<:` merge.
//
// One spelling, because two readers need this subtree — bootstrap, which acts
// on it, and the preset reader, which reports what a file binds. A second
// unwrapper that skipped the resolution would call a preset unparseable that
// boots in production.
func SeedSubtreeBytes(declared yaml.Node) ([]byte, error) {
	resolveSeedAliases(&declared)
	raw, err := yaml.Marshal(&declared)
	if err != nil {
		return nil, fmt.Errorf("deployconfig: re-encoding a seed subtree: %w", err)
	}
	return raw, nil
}

// resolveSeedAliases replaces every alias in a node tree with a copy of what it
// points at, so a subtree can be re-encoded on its own.
//
// Anchors are cleared as it goes: an anchor left on a node the subtree no
// longer shares would be re-emitted as a definition nothing references, which
// is noise in an error message and a diff.
func resolveSeedAliases(n *yaml.Node) {
	if n == nil {
		return
	}
	if n.Kind == yaml.AliasNode && n.Alias != nil {
		// A copy, because the target may be shared with the rest of the
		// document and resolving in place would rewrite what the other
		// references see.
		target := *n.Alias
		resolveSeedAliases(&target)
		*n = target
	}
	n.Anchor = ""
	for _, child := range n.Content {
		resolveSeedAliases(child)
	}
}
