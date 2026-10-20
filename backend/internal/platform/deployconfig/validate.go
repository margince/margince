// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deployconfig

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
)

// ValueRule is what one bootstrap field admits, in the form a JSON schema
// states it. Each is read from the source validate checks the field with.
type ValueRule struct {
	Pattern string
	Enum    []string
}

// ValueRules are the bootstrap fields whose rule a schema can state, by key.
// A timezone's rule is the zone database and a country's the compiled-in
// packs, which no pattern holds.
func ValueRules() map[string]ValueRule {
	return map[string]ValueRule{
		"workspace.base_currency": {Pattern: values.CurrencyPattern},
		"workspace.base_language": {Enum: spokenLanguages()},
	}
}

func spokenLanguages() []string {
	out := make([]string, len(textlang.Shipped))
	for i, lang := range textlang.Shipped {
		out[i] = string(lang)
	}
	return out
}

// noPasswordHint is the one sentence for an admin with no password, from the
// check and from ResolvePassword alike.
const noPasswordHint = "bootstrap_admin names no password — set bootstrap_admin.password to " +
	"${file:/run/secrets/admin-password} or ${env:MARGINCE_ADMIN_PASSWORD}"

// validate returns every fault of the merged roots, never only the first.
func (c Bootstrap) validate() []fault {
	var out []fault
	add := func(key, problem string) { out = append(out, fault{key: key, problem: problem}) }
	if c.Version != 1 {
		add("version", fmt.Sprintf("unsupported version %d (this build supports version 1)", c.Version))
	}
	if c.Workspace.Timezone != "" {
		if _, err := values.ParseTimezone(c.Workspace.Timezone); err != nil {
			add("workspace.timezone", problemOf(err))
		}
	}
	if cur := c.Workspace.BaseCurrency; cur != "" && !values.ValidCurrency(cur) {
		add("workspace.base_currency", fmt.Sprintf("%q is not a 3-letter ISO 4217 code in capitals", cur))
	}
	if code := c.Workspace.Country; code != "" {
		if err := jurisdiction.Code(code).Validate(); err != nil {
			add("workspace.country", err.Error())
		}
	}
	if lang := c.Workspace.BaseLanguage; lang != "" && !textlang.Known(lang) {
		add("workspace.base_language", fmt.Sprintf("%q is not a language this build speaks (%s)", lang, strings.Join(spokenLanguages(), ", ")))
	}
	if c.BootstrapAdmin != nil {
		out = append(out, c.BootstrapAdmin.validate()...)
	}
	return append(out, c.Seeds.validate()...)
}

// validate takes either spelling of the password, and words its absence as
// ResolvePassword does, so the check and the resolver are one contract.
func (b BootstrapAdmin) validate() []fault {
	var out []fault
	if _, err := values.ParseEmail(b.Email); err != nil {
		out = append(out, fault{key: "bootstrap_admin.email", problem: problemOf(err)})
	}
	if b.DisplayName == "" {
		out = append(out, fault{key: "bootstrap_admin.display_name", problem: "required"})
	}
	if !b.Password.Configured() && b.PasswordFile == "" {
		out = append(out, fault{key: "bootstrap_admin", problem: noPasswordHint})
	}
	return out
}

func (s Seeds) validate() []fault {
	var out []fault
	if s.Pipeline != nil {
		out = append(out, s.Pipeline.validate()...)
	}
	if s.Retention != nil {
		out = append(out, s.Retention.validate()...)
	}
	seenKeys := map[string]bool{}
	for _, p := range s.ConsentPurposes {
		switch {
		case p.Key == "" || p.Label == "":
			out = append(out, fault{key: "seeds.consent_purposes", problem: "each entry needs key and label"})
		case seenKeys[p.Key]:
			out = append(out, fault{key: "seeds.consent_purposes", problem: fmt.Sprintf("key %q is listed twice", p.Key)})
		}
		seenKeys[p.Key] = true
	}
	return out
}

// validate refuses an unrecognized posture. A typo would otherwise fall back to
// `standard` in silence. For a deployment that set the key, that is the
// failure it was set to prevent.
func (r RetentionSeed) validate() []fault {
	switch r.DefaultPolicy {
	case "", RetentionStandardPosture, RetentionRetainOnlyPosture:
		return nil
	default:
		return []fault{{key: "seeds.retention.default_policy", problem: fmt.Sprintf("%q is not %s or %s",
			r.DefaultPolicy, RetentionStandardPosture, RetentionRetainOnlyPosture)}}
	}
}

func (p PipelineSeed) validate() []fault {
	var out []fault
	add := func(problem string) { out = append(out, fault{key: "seeds.pipeline", problem: problem}) }
	if p.Name == "" {
		add("name is required")
	}
	if len(p.Stages) == 0 {
		add("stages must name at least one open stage")
	}
	var seen []string
	for _, st := range p.Stages {
		switch {
		case st.Name == "":
			add("each stage needs a name")
		case slices.Contains(seen, st.Name):
			add(fmt.Sprintf("stage %q is listed twice", st.Name))
		}
		seen = append(seen, st.Name)
		if st.Probability < 0 || st.Probability > 100 {
			add(fmt.Sprintf("stage %q probability %d is outside 0–100", st.Name, st.Probability))
		}
	}
	return out
}

// problemOf drops the field a value parser names, since the fault names the key.
func problemOf(err error) string {
	var parse *values.ParseError
	if errors.As(err, &parse) {
		return parse.Message
	}
	return err.Error()
}
