// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// The shapes the hand parser could not read.
//
// Each one is legal TOML that gitleaks honours, and each was demonstrated to
// hide a real credential while the suite printed ok — because an allowlist the
// derivation cannot SEE owes no planted token, and the coverage ledger then
// reports full coverage of a policy it read half of. They are listed here as
// cases rather than as prose so a decoder that regresses on one says so.

import (
	"strings"
	"testing"
)

// decode is the tool's own reading of a policy, as the record stream the
// consuming script splits on.
func decode(t *testing.T, policy string) []string {
	t.Helper()
	var p policyDoc
	if err := unmarshalPolicy([]byte(policy), &p); err != nil {
		t.Fatalf("decoding the policy: %v", err)
	}
	out, err := render(&p)
	if err != nil {
		t.Fatalf("rendering the policy: %v", err)
	}
	if out == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(out, "\n"), "\n")
}

// paths answers every path entry the decoder found, whatever allowlist it sits
// in — what the plant loop walks.
func paths(records []string) []string {
	var out []string
	for _, r := range records {
		f := strings.Split(r, fieldSep)
		if len(f) == 3 && f[1] == "path" {
			out = append(out, f[2])
		}
	}
	return out
}

func TestTheShapesTheHandParserCouldNotRead(t *testing.T) {
	for _, c := range []struct {
		name   string
		policy string
		want   string
	}{{
		name: "an indented table header",
		// The awk anchored every pattern at column 0, so this exempted a whole
		// directory while the suite reported full coverage.
		policy: "  [[allowlists]]\n  description = \"indented\"\n  paths = ['''secrets/''']\n",
		want:   "secrets/",
	}, {
		name: "an allowlist with no description and no targetRules",
		// Nothing here is optional to gitleaks; the awk keyed its record on
		// fields it assumed were present.
		policy: "[[allowlists]]\npaths = ['''bare/''']\n",
		want:   "bare/",
	}, {
		name: "an allowlist nested in a rule",
		// The channel the awk could not see at all: a custom rule carries its
		// own exemptions, and a `[[rules]]` header also reset the awk's index,
		// merging two allowlists into one record set.
		policy: "[[rules]]\nid = \"local\"\nregex = '''x'''\n\n[[rules.allowlists]]\ndescription = \"nested\"\npaths = ['''nested/''']\n",
		want:   "nested/",
	}, {
		name: "a mixed-quote array",
		// paths were assumed triple-quoted; a "basic string" among them was
		// dropped, and a dropped path owes no plant.
		policy: "[[allowlists]]\ndescription = \"mixed\"\npaths = ['''first/''', \"second/\"]\n",
		want:   "second/",
	}, {
		name:   "an array written across lines with a trailing comma",
		policy: "[[allowlists]]\ndescription = \"multiline\"\npaths = [\n  '''one/''',\n  '''two/''',\n]\n",
		want:   "two/",
	}} {
		t.Run(c.name, func(t *testing.T) {
			got := paths(decode(t, c.policy))
			for _, p := range got {
				if p == c.want {
					return
				}
			}
			t.Errorf("the decoder read %v and not %q — an exemption it cannot see owes no plant, "+
				"and the coverage ledger cannot tell that apart from full coverage", got, c.want)
		})
	}
}

// A `[[rules]]` header used to reset the record index, so the allowlist after
// it merged into the one before. Two allowlists sharing an index means one
// plant vouches for both, which is the same hole with a subtler shape.
func TestEveryAllowlistKeepsItsOwnIndex(t *testing.T) {
	records := decode(t, strings.Join([]string{
		"[[allowlists]]", "description = \"first\"", "paths = ['''a/''']", "",
		"[[rules]]", "id = \"local\"", "regex = '''x'''", "",
		"[[rules.allowlists]]", "description = \"second\"", "paths = ['''b/''']", "",
	}, "\n"))
	seen := map[string]string{}
	for _, r := range records {
		f := strings.Split(r, fieldSep)
		if len(f) == 3 && f[1] == "path" {
			seen[f[2]] = f[0]
		}
	}
	if len(seen) != 2 {
		t.Fatalf("read %d path entries, want 2: %v", len(seen), seen)
	}
	if seen["a/"] == seen["b/"] {
		t.Errorf("both allowlists were recorded under index %s, so one planted token vouches for both", seen["a/"])
	}
}

// The file's own exemption channels — the ones belonging to no allowlist — are
// reported from the DECODED policy, which is the correction this tool is, one
// layer up: the consuming gate used to grep for these keys, and every shape
// below defeats a line matcher while gitleaks honours it.
func TestTheFileLevelExemptionsAreReadNotMatched(t *testing.T) {
	fileFact := func(records []string, kind string) string {
		for _, r := range records {
			f := strings.Split(r, fieldSep)
			if len(f) == 3 && f[0] == "0" && f[1] == kind {
				return f[2]
			}
		}
		return ""
	}
	for _, c := range []struct {
		name, policy, kind, want string
	}{{
		name:   "useDefault under another table does not count",
		policy: "[extend]\n\n[[allowlists]]\nuseDefault = true\npaths = ['''decoy/''']\n",
		kind:   "use_default", want: "false",
	}, {
		name:   "a dotted key at the top level is still [extend]",
		policy: "extend.useDefault = true\n",
		kind:   "use_default", want: "true",
	}, {
		name:   "a quoted key is the same key",
		policy: "[extend]\n\"useDefault\" = true\n",
		kind:   "use_default", want: "true",
	}, {
		name:   "disabled rules are counted, not matched",
		policy: "[extend]\nuseDefault = true\ndisabledRules = [\"generic-api-key\", \"github-pat\"]\n",
		kind:   "disabled_rules", want: "2",
	}, {
		name:   "a stopword nested in a rule's allowlist counts",
		policy: "[extend]\nuseDefault = true\n\n[[rules]]\nid = \"local\"\nregex = '''x'''\n\n[[rules.allowlists]]\nstopwords = [\"example\"]\n",
		kind:   "stopwords", want: "1",
	}, {
		name:   "a stopword on a top-level allowlist counts too",
		policy: "[extend]\nuseDefault = true\n\n[[allowlists]]\nstopwords = [\"fixture\", \"sample\"]\n",
		kind:   "stopwords", want: "2",
	}} {
		t.Run(c.name, func(t *testing.T) {
			if got := fileFact(decode(t, c.policy), c.kind); got != c.want {
				t.Errorf("%s = %q, want %q — a gate reading this would refuse, or admit, the wrong file",
					c.kind, got, c.want)
			}
		})
	}
}

// A policy that does not decode must fail loudly. Carrying on would report
// full coverage of nothing at all, which is the worst reading this suite can
// produce: green over a policy nobody read.
func TestAMalformedPolicyIsRefused(t *testing.T) {
	var p policyDoc
	if err := unmarshalPolicy([]byte("[[allowlists]\ndescription = "), &p); err == nil {
		t.Error("a malformed policy decoded without complaint")
	}
}

// A value the record stream cannot express must be refused, not encoded into
// something the consumer reads as a different record.
//
// TOML can carry both a newline and the separator — `"""…"""` spans lines and
// `"\u001f"` is a legal escape — and the consuming awk splits on newlines. A
// path smuggling one would arrive as an allowlist nobody wrote, and the suite
// would plant against it and report full coverage of a policy it read wrong.
// Refusing is the only reading that cannot be silently wrong, and this is what
// proves the refusal fires rather than the stream carrying on short.
func TestAPolicyTheRecordStreamCannotExpressIsRefused(t *testing.T) {
	for _, c := range []struct {
		name   string
		policy string
	}{
		{
			name:   "a path spanning two lines",
			policy: "[[allowlists]]\ndescription = \"multiline\"\npaths = [\"\"\"docs/\nnotes/\"\"\"]\n",
		},
		{
			name:   "a regex carrying the record separator",
			policy: "[[allowlists]]\ndescription = \"separator\"\nregexes = [\"AKIA\\u001fSECRET\"]\n",
		},
		{
			name:   "a nested allowlist's path spanning two lines",
			policy: "[[rules]]\nid = \"r\"\n[[rules.allowlists]]\ndescription = \"nested\"\npaths = [\"\"\"a\nb\"\"\"]\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			var p policyDoc
			if err := unmarshalPolicy([]byte(c.policy), &p); err != nil {
				t.Fatalf("this case must be legal TOML, or it proves nothing about the render: %v", err)
			}
			out, err := render(&p)
			if err == nil {
				t.Fatalf("the record stream accepted a value it cannot express, and answered:\n%q", out)
			}
			if out != "" {
				t.Errorf("a refused render answered %q as well as an error — a consumer reading both "+
					"would plant against a truncated policy", out)
			}
		})
	}
}
