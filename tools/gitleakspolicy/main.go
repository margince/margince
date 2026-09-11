// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Command gitleakspolicy decodes .gitleaks.toml and prints the allowlist
// entries as a flat record stream, one field per line:
//
//	<index><US><kind><US><value>
//
// where <US> is ASCII 0x1F and kind is one of desc, rule, path, regex.
//
// It exists because scripts/test-secret-scan.sh derives its plants from the
// policy rather than from a hand-kept list — every allowlist owes a planted
// token, so an entry the suite cannot see is an exemption nobody proved is
// narrow. That derivation was an awk parser, and a hand parser reads only the
// shapes it was taught: an indented table header, a `[[rules]]` block, a
// mixed-quote array and four other legal TOML forms each hid a real credential
// while the suite printed ok. A decoder has no such list.
//
// What it adds beyond the awk it replaces:
//
//   - `[[rules.allowlists]]`, which the awk could not see at all. A custom rule
//     carries its own exemption channel, and an exemption nothing enumerates is
//     one nothing plants against.
//   - every legal spelling of a string and an array, because the decoder
//     implements TOML rather than a subset of it.
//
// The INDEX is monotonic across the whole file and never resets, so no two
// allowlists can merge into one record set — the awk's own failure mode when a
// `[[rules]]` header reset its counter. Order is file order: `[[allowlists]]`
// as they appear, then each rule's nested allowlists with the rule's own
// position, so a diff of this output reads like a diff of the policy.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// fieldSep is the separator between an emitted record's fields: a unit
// separator, not a pipe. `|` is ordinary inside a path regex — (^|/)vendor/ —
// and a delimiter that can occur in the data truncates it into a different,
// usually invalid, pattern. The consuming script splits on the same byte.
const fieldSep = "\x1f"

// allowlist is the slice of gitleaks' allowlist this suite plants against.
// Deliberately not the whole type: `condition` and `regexTarget` shape how an
// entry matches, and the script asks what the entry NAMES.
type allowlist struct {
	Description string   `toml:"description"`
	TargetRules []string `toml:"targetRules"`
	Paths       []string `toml:"paths"`
	Regexes     []string `toml:"regexes"`
}

// policyDoc is the file, to the depth exemptions can hide in. A rule's nested
// allowlists are read for the reason the top-level ones are: they excuse
// matches, and an excuse nobody plants against is an excuse nobody checked.
type policyDoc struct {
	Allowlists []allowlist `toml:"allowlists"`
	Rules      []struct {
		Allowlists []allowlist `toml:"allowlists"`
	} `toml:"rules"`
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: gitleakspolicy <path to .gitleaks.toml>")
		os.Exit(2)
	}
	// The path is this tool's own argument, passed by the Makefile target that
	// runs it; there is no remote caller to traverse anywhere. A build tool
	// that refused to read the file it was told to read would be refusing its
	// own invocation.
	//nolint:gosec // G703: the argument is the invoking script's, not a request's
	raw, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "gitleakspolicy: reading the policy: %v\n", err)
		os.Exit(1)
	}
	var p policyDoc
	if err := unmarshalPolicy(raw, &p); err != nil {
		// A policy that does not decode is not a policy this suite may plant
		// against, and carrying on would report full coverage of nothing.
		fmt.Fprintf(os.Stderr, "gitleakspolicy: %s is not readable TOML: %v\n", os.Args[1], err)
		os.Exit(1)
	}
	// Written to stdout rather than printed: the consumer is a shell script
	// reading a record stream, and a refused write has to reach it as a
	// failure. A truncated stream is the one outcome this tool must never
	// produce quietly — the suite would plant against the entries it received
	// and report full coverage of a policy it read half of.
	if _, err := os.Stdout.WriteString(render(&p)); err != nil {
		fmt.Fprintf(os.Stderr, "gitleakspolicy: writing the record stream: %v\n", err)
		os.Exit(1)
	}
}

// unmarshalPolicy is the decode, named so the cases beside it exercise the
// same call the command makes rather than a second reading of the file.
func unmarshalPolicy(raw []byte, p *policyDoc) error {
	return toml.Unmarshal(raw, p)
}

// render walks the policy in FILE order — the top-level allowlists, then each
// rule's nested ones — and emits one record per field.
//
// The index is monotonic across the whole walk and never resets, so no two
// allowlists can merge into one record set. That was the awk's own failure
// mode: a `[[rules]]` header reset its counter, and one planted token then
// vouched for two exemptions.
func render(p *policyDoc) string {
	var out strings.Builder
	index := 0
	emit := func(a allowlist) {
		index++
		write(&out, index, "desc", a.Description)
		for _, r := range a.TargetRules {
			write(&out, index, "rule", r)
		}
		for _, path := range a.Paths {
			write(&out, index, "path", path)
		}
		for _, re := range a.Regexes {
			write(&out, index, "regex", re)
		}
	}
	for _, a := range p.Allowlists {
		emit(a)
	}
	for _, rule := range p.Rules {
		for _, a := range rule.Allowlists {
			emit(a)
		}
	}
	return out.String()
}

// write emits one record, refusing a value carrying the separator. Such a
// value would split into fields the consumer reads as a different record, and
// a path or regex read wrong is an entry planted against wrong.
func write(out *strings.Builder, index int, kind, value string) {
	if value == "" {
		return
	}
	if strings.ContainsAny(value, fieldSep+"\n") {
		fmt.Fprintf(os.Stderr,
			"gitleakspolicy: allowlist %d's %s carries a separator or a newline, which the record stream cannot express: %q\n",
			index, kind, value)
		os.Exit(1)
	}
	fmt.Fprintf(out, "%d%s%s%s%s\n", index, fieldSep, kind, fieldSep, value)
}
