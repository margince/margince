// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"fmt"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/deployconfig"
)

// commentWidth is where a doc comment wraps in the YAML and .env templates.
const commentWidth = 96

const exampleHeader = `# yaml-language-server: $schema=./margince.schema.json
# Written by tools/gen-config from the settings registry; run ` + "`make config-gen`" + ` after a change.
#
# The bootstrap part first, then every setting commented out at its default. To set one, remove
# the first "# " of its lines and of the section lines above it. A secret takes a reference and
# never a literal: ${file:/path} or ${env:NAME}. A secret that allows it takes ${none} to declare
# itself absent.

`

// exampleYAML is the bootstrap template, then each setting as nested YAML,
// commented out.
func exampleYAML(settings []setting, bootstrap string) (string, error) {
	var b strings.Builder
	b.WriteString(exampleHeader)
	b.WriteString(strings.TrimRight(bootstrap, "\n") + "\n")
	for _, group := range groups(settings) {
		b.WriteString("\n# ---- " + groupTitle(group) + " ----\n")
		var open []string
		for _, s := range group {
			parts := strings.Split(s.Key(), ".")
			parents := parts[:len(parts)-1]
			shared := 0
			for shared < len(open) && shared < len(parents) && open[shared] == parents[shared] {
				shared++
			}
			for depth := shared; depth < len(parents); depth++ {
				b.WriteString(yamlLine(depth, parents[depth]+":"))
			}
			open = parents
			if err := writeYAMLSetting(&b, len(parents), parts[len(parts)-1], s); err != nil {
				return "", err
			}
		}
	}
	return b.String(), nil
}

func writeYAMLSetting(b *strings.Builder, depth int, name string, s setting) error {
	for _, line := range wrap(s.Doc(), commentWidth) {
		b.WriteString(yamlLine(depth, "# "+line))
	}
	if s.Secret() {
		b.WriteString(yamlLine(depth, name+": ${file:/run/secrets/"+s.Key()+"}"))
		return nil
	}
	value, ok, err := yamlDefault(s)
	switch {
	case err != nil:
		return err
	case !ok:
		b.WriteString(yamlLine(depth, name+":"))
		return nil
	}
	b.WriteString(yamlLine(depth, "# default: "+value))
	b.WriteString(yamlLine(depth, name+": "+value))
	return nil
}

// yamlLine is one commented-out line at a nesting depth.
func yamlLine(depth int, text string) string {
	return "# " + strings.Repeat("  ", depth) + text + "\n"
}

// yamlDefault writes the default as YAML reads it back. A list, map or object
// is JSON flow. A text value is quoted where YAML would read something else.
func yamlDefault(s setting) (string, bool, error) {
	text, ok := s.DefaultText()
	switch {
	case !ok:
		return "", false, nil
	case s.shape.DefaultJSON != "":
		return s.shape.DefaultJSON, true, nil
	case textual(s):
		scalar, err := yamlScalar(text)
		if err != nil {
			return "", false, fmt.Errorf("gen-config: %s: writing the default as YAML: %w", s.Key(), err)
		}
		return scalar, true, nil
	default:
		return text, true, nil
	}
}

// textual is a value YAML must read as a string. That is a string kind, or a
// type whose text is no YAML scalar of its kind, such as a duration.
func textual(s setting) bool {
	return s.shape.Go.Kind() == reflect.String || s.TypeName() == config.Duration.Name()
}

// yamlScalar quotes text only where a plain scalar would not read back as it.
func yamlScalar(text string) (string, error) {
	var n yaml.Node
	n.SetString(text)
	out, err := yaml.Marshal(&n)
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(string(out), "\n"), nil
}

const envHeader = `# Written by tools/gen-config from the settings registry; run ` + "`make config-gen`" + ` after a change.
#
# Every variable a Margince program or image entrypoint reads, commented out at its default. Copy
# what you need to .env.local (gitignored); a deployment supplies these through its own environment
# or secret store. A secret has no default: write the value itself here, never in a committed file.
# [required] marks a variable the image entrypoint refuses to start without.
# docs/reference/configuration.md lists each setting with the key it sets in margince.yaml.
`

// envInputs are the variables .env.example lists beside the settings: those
// the loader reads itself, and those the image entrypoint scripts read.
type envInputs struct {
	loader     []config.LoaderVar
	entrypoint []deployconfig.EntrypointVar
}

// required is the set of variables an entrypoint stops without.
func (in envInputs) required() map[string]bool {
	out := map[string]bool{}
	for _, v := range in.entrypoint {
		out[v.Name] = out[v.Name] || v.Required
	}
	return out
}

// envExample writes the loader's variables, then the entrypoint's own, then
// the settings section by section, each commented out at its default.
func envExample(settings []setting, in envInputs) string {
	var b strings.Builder
	b.WriteString(envHeader)
	required := in.required()
	b.WriteString("\n# ---- loader ----\n")
	for _, v := range in.loader {
		if v.Env != "" {
			writeEnvVar(&b, v.Doc, v.Env, "", required[v.Env])
		}
	}
	settingEnvs := map[string]bool{}
	for _, s := range settings {
		settingEnvs[s.Env()] = true
	}
	var entrypointOnly []deployconfig.EntrypointVar
	for _, v := range in.entrypoint {
		if !settingEnvs[v.Name] {
			entrypointOnly = append(entrypointOnly, v)
		}
	}
	if len(entrypointOnly) > 0 {
		b.WriteString("\n# ---- image entrypoint ----\n")
		for _, v := range entrypointOnly {
			writeEnvVar(&b, "Read by scripts/deploy/"+v.Script+" before it starts a program.", v.Name, "", v.Required)
		}
	}
	for _, group := range groups(settings) {
		b.WriteString("\n# ---- " + groupTitle(group) + " ----\n")
		for _, s := range group {
			writeEnvVar(&b, s.Doc(), s.Env(), envDefault(s), required[s.Env()])
		}
	}
	return b.String()
}

func writeEnvVar(b *strings.Builder, doc, name, value string, required bool) {
	if required {
		doc = "[required] " + doc
	}
	b.WriteString("\n")
	for _, line := range wrap(doc, commentWidth) {
		b.WriteString("# " + line + "\n")
	}
	b.WriteString("# " + name + "=" + value + "\n")
}

// envDefault is the default as the variable spells it, quoted where the .env
// reader would otherwise trim or cut it. A secret shows nothing.
func envDefault(s setting) string {
	text, ok := s.DefaultText()
	if s.Secret() || !ok {
		return ""
	}
	plain := text == strings.TrimSpace(text) && !strings.Contains(text, " #") &&
		!strings.HasPrefix(text, `"`) && !strings.HasPrefix(text, "'")
	switch {
	case plain:
		return text
	case !strings.Contains(text, "'"):
		return "'" + text + "'"
	default:
		return `"` + text + `"`
	}
}

// wrap breaks text into lines of at most width runes, at spaces.
func wrap(text string, width int) []string {
	var lines []string
	line := ""
	for word := range strings.FieldsSeq(text) {
		switch {
		case line == "":
			line = word
		case len([]rune(line))+1+len([]rune(word)) > width:
			lines = append(lines, line)
			line = word
		default:
			line += " " + word
		}
	}
	return append(lines, line)
}
