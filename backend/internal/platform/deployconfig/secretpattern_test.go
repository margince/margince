// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deployconfig

// SecretRefPattern is published so margince.schema.json can carry it, which
// makes it a claim about what the LOADER accepts. This holds the two together.
//
// Both directions matter and they fail differently. A pattern LOOSER than the
// loader lets an editor bless a file that refuses to boot — the schema's whole
// job, done wrong. A pattern TIGHTER than the loader flags config the server
// reads happily, which is the kind of false positive that teaches somebody to
// ignore the squiggle, after which the schema may as well not exist.

import (
	"regexp"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestTheSecretPatternAcceptsWhatTheLoaderAccepts(t *testing.T) {
	schema := regexp.MustCompile(SecretRefPattern)
	for _, tc := range []struct {
		name, raw string
	}{
		{"an environment reference", "${env:MARGINCE_SMTP_PASSWORD}"},
		{"a file reference", "${file:/run/secrets/smtp}"},
		{"padding the loader trims", "${env:  MARGINCE_SMTP_PASSWORD  }"},
		// Empty is a legitimate value: it names no source, which is how a
		// deployment leaves an optional secret unset.
		{"the empty string", ""},
		// The one this pair was found by: the argument is only whitespace, so
		// the loader trims it to nothing and refuses.
		{"an argument of only spaces", "${env:   }"},
		{"an argument of nothing at all", "${env:}"},
		{"an unknown source", "${vault:thing}"},
		{"a literal secret", "hunter2"},
		{"a reference missing its braces", "env:MARGINCE_SMTP_PASSWORD"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Straight into a Secret, which is the path a config takes: the
			// custom unmarshaller is what decides, and reaching for it directly
			// would be this test staging its own version of the decode.
			var got Secret
			loaderAccepts := yaml.Unmarshal([]byte(quoted(tc.raw)), &got) == nil
			if schema.MatchString(tc.raw) != loaderAccepts {
				t.Errorf("the schema %s %q and the loader %s it — an editor and the server must agree",
					verdict(schema.MatchString(tc.raw)), tc.raw, verdict(loaderAccepts))
			}
		})
	}
}

func verdict(accepts bool) string {
	if accepts {
		return "accepts"
	}
	return "refuses"
}

// quoted keeps a value yaml would otherwise reinterpret — an empty scalar, or
// one that begins with a brace — as the literal string under test.
func quoted(raw string) string {
	out, err := yaml.Marshal(raw)
	if err != nil {
		return `""`
	}
	return string(out[:len(out)-1])
}
