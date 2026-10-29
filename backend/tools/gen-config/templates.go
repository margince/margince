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

const exampleHeader = `# yaml-language-server: $schema=./margince.schema.json
# Written by tools/gen-config; run ` + "`make config-gen`" + ` after a change.
#
# The bootstrap part only. Every setting at its default is in margince.defaults.yaml: copy the
# lines you change. What each setting does is in docs/reference/configuration.md, and an editor
# that reads the schema shows it on hover. A secret takes ${file:/path} or ${env:NAME}, never a
# literal.

`

// exampleYAML is the header, then the bootstrap template.
func exampleYAML(bootstrap string) string {
	return exampleHeader + strings.TrimRight(bootstrap, "\n") + "\n"
}

const defaultsHeader = `# yaml-language-server: $schema=./margince.schema.json
# Written by tools/gen-config; every setting at its default. Do not edit: copy from it.

version: 1
`

// defaultsYAML writes each setting at its default as nested YAML a loader
// reads. A secret is a file reference, and a setting with no default is null.
func defaultsYAML(settings []setting) (string, error) {
	var b strings.Builder
	b.WriteString(defaultsHeader)
	for _, group := range groups(settings) {
		b.WriteString("\n")
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
			value, err := defaultsValue(s)
			if err != nil {
				return "", err
			}
			b.WriteString(yamlLine(len(parents), parts[len(parts)-1]+": "+value))
		}
	}
	return b.String(), nil
}

func defaultsValue(s setting) (string, error) {
	if s.Secret() {
		return secretReference(s), nil
	}
	value, ok, err := yamlDefault(s)
	switch {
	case err != nil:
		return "", err
	case !ok:
		return "null", nil
	}
	return value, nil
}

// secretReference is the file reference both templates show for a secret, so
// a copied line points at a file instead of carrying the value itself.
func secretReference(s setting) string {
	return "${file:/run/secrets/" + s.Key() + "}"
}

// yamlLine is one line at a nesting depth.
func yamlLine(depth int, text string) string {
	return strings.Repeat("  ", depth) + text + "\n"
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

const envHeader = `# Written by tools/gen-config; run ` + "`make config-gen`" + ` after a change.
#
# Every variable a Margince program or image entrypoint reads, commented out at its default. Copy
# a line to .env.local (git-ignored) and remove its "# " to set it. A secret shows a file
# reference instead of a default: point it at the file that holds the value, or write the value
# itself. What each variable does is in docs/reference/configuration.md.
`

// envInputs are the variables .env.example lists beside the settings: those
// the loader reads itself, and those the image entrypoint scripts read.
type envInputs struct {
	loader     []config.LoaderVar
	entrypoint []deployconfig.EntrypointVar
}

// envExample writes the variables an entrypoint stops without, then the
// loader's, then the entrypoint's other own ones, then the settings section by
// section. Each variable is one line, under one heading only.
func envExample(settings []setting, in envInputs) string {
	defaults := map[string]string{}
	for _, s := range settings {
		defaults[s.Env()] = envDefault(s)
	}
	var b strings.Builder
	b.WriteString(envHeader)
	written := map[string]bool{}
	section := func(title string, names []string) {
		var lines []string
		for _, name := range names {
			if name != "" && !written[name] {
				written[name] = true
				lines = append(lines, "# "+name+"="+defaults[name]+"\n")
			}
		}
		if len(lines) > 0 {
			b.WriteString("\n# ---- " + title + " ----\n" + strings.Join(lines, ""))
		}
	}
	var required, entrypoint []string
	for _, v := range in.entrypoint {
		if v.Required {
			required = append(required, v.Name)
		}
		if _, isSetting := defaults[v.Name]; !isSetting {
			entrypoint = append(entrypoint, v.Name)
		}
	}
	section("required by the image entrypoint", required)
	loader := make([]string, len(in.loader))
	for i, v := range in.loader {
		loader[i] = v.Env
	}
	section("loader", loader)
	section("image entrypoint", entrypoint)
	for _, group := range groups(settings) {
		names := make([]string, len(group))
		for i, s := range group {
			names[i] = s.Env()
		}
		section(groupTitle(group), names)
	}
	return b.String()
}

// envDefault is the default as the variable spells it, quoted where the .env
// reader would otherwise trim or cut it. A secret shows its file reference.
func envDefault(s setting) string {
	if s.Secret() {
		return secretReference(s)
	}
	text, ok := s.DefaultText()
	if !ok {
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
