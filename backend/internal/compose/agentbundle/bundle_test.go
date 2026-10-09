// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agentbundle

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const testBase = "https://crm.test.example/v1"

// unzip builds the bundle for testBase and returns its files by name.
func unzip(t *testing.T) map[string]string {
	t.Helper()
	archive, err := (&Builder{}).Build(testBase)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatalf("the bundle is not a ZIP: %v", err)
	}
	files := map[string]string{}
	for _, f := range reader.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("opening %s: %v", f.Name, err)
		}
		body, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("reading %s: %v", f.Name, err)
		}
		if err := rc.Close(); err != nil {
			t.Fatalf("closing %s: %v", f.Name, err)
		}
		files[f.Name] = string(body)
	}
	return files
}

func TestTheBundleIsOneFolderOfFourFilesNamingTheBase(t *testing.T) {
	files := unzip(t)
	want := []string{"margince/README.md", "margince/SKILL.md", "margince/INDEX.md", "margince/openapi.yaml"}
	if len(files) != len(want) {
		t.Errorf("the bundle holds %d files, want %d: %v", len(files), len(want), files)
	}
	for _, name := range want {
		if _, ok := files[name]; !ok {
			t.Errorf("the bundle has no %s", name)
		}
	}
	for _, guide := range []string{"margince/README.md", "margince/SKILL.md"} {
		if !strings.Contains(files[guide], testBase) {
			t.Errorf("%s does not name the base URL %s", guide, testBase)
		}
		if strings.Contains(files[guide], "{{.") {
			t.Errorf("%s still carries template syntax", guide)
		}
	}
	for name, body := range files {
		if strings.Contains(body, "mgp_") {
			t.Errorf("%s carries a passport-shaped token; the bundle holds no credential", name)
		}
	}
}

// Claude Code refuses a Bash command that expands a shell variable it cannot
// check, so a skill telling the agent to write `$MARGINCE_PASSPORT` sends nothing.
func TestTheSkillHasCurlReadThePassportItself(t *testing.T) {
	skill := unzip(t)["margince/SKILL.md"]
	want := "--variable %MARGINCE_PASSPORT \\\n  --expand-header 'Authorization: Bearer {{MARGINCE_PASSPORT}}'"
	if !strings.Contains(skill, want) {
		t.Errorf("SKILL.md does not show curl reading the passport from the environment:\n%s", skill)
	}
	if strings.Contains(skill, "$MARGINCE_PASSPORT") {
		t.Error("SKILL.md tells the agent to expand $MARGINCE_PASSPORT in a command")
	}
}

func TestTheContractsServerIsTheBase(t *testing.T) {
	spec := unzip(t)["margince/openapi.yaml"]
	var doc struct {
		OpenAPI string `yaml:"openapi"`
		Servers []struct {
			URL string `yaml:"url"`
		} `yaml:"servers"`
	}
	if err := yaml.Unmarshal([]byte(spec), &doc); err != nil {
		t.Fatalf("openapi.yaml is not YAML: %v", err)
	}
	if len(doc.Servers) != 1 || doc.Servers[0].URL != testBase {
		t.Errorf("servers = %+v, want the one base %s", doc.Servers, testBase)
	}
	if !strings.HasPrefix(doc.OpenAPI, "3.1") {
		t.Errorf("openapi = %q, want 3.1", doc.OpenAPI)
	}
	if strings.Contains(spec, "DO NOT EDIT") {
		t.Error("openapi.yaml carries the generator's DO NOT EDIT line, which is for this tree only")
	}
}

func TestABuilderRebuildsForANewBase(t *testing.T) {
	builder := &Builder{}
	first, err := builder.Build(testBase)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	second, err := builder.Build("https://other.test.example/v1")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if bytes.Equal(first, second) {
		t.Error("a second base answered the first base's bundle")
	}
}

// The generated contract is the generator's output over the real crm.yaml,
// and `make drift` holds it current, so these read what an agent receives.

func TestTheEmbeddedContractHasNoAnchorAndEveryRefResolves(t *testing.T) {
	var doc yaml.Node
	if err := yaml.Unmarshal(contract, &doc); err != nil {
		t.Fatalf("the embedded contract is not YAML: %v", err)
	}
	var refs []string
	var walk func(*yaml.Node)
	walk = func(n *yaml.Node) {
		if n.Kind == yaml.AliasNode || n.Anchor != "" {
			t.Errorf("line %d carries an anchor or alias; a copy that dropped its anchor would point at nothing", n.Line)
		}
		for i := 0; i+1 < len(n.Content); i += 2 {
			if n.Kind == yaml.MappingNode && n.Content[i].Value == "$ref" {
				refs = append(refs, n.Content[i+1].Value)
			}
		}
		for _, child := range n.Content {
			walk(child)
		}
	}
	walk(&doc)
	if len(refs) == 0 {
		t.Fatal("found no $ref at all, so the walk is not reading the contract")
	}
	var wrapper struct {
		Components map[string]map[string]yaml.Node `yaml:"components"`
	}
	if err := yaml.Unmarshal(contract, &wrapper); err != nil {
		t.Fatalf("reading components: %v", err)
	}
	for _, ref := range refs {
		parts := strings.Split(strings.TrimPrefix(ref, "#/components/"), "/")
		if len(parts) != 2 {
			t.Errorf("$ref %q does not point into components", ref)
			continue
		}
		if _, ok := wrapper.Components[parts[0]][parts[1]]; !ok {
			t.Errorf("$ref %q names a component the contract does not carry", ref)
		}
	}
}

// developerNotes are the forms of build notes an agent must never be handed.
var developerNotes = regexp.MustCompile(`\bADR-\d+|features/|data-model|\bAAD-|\bIS (NOT )?NULL\b|x-mcp-tool|x-agent-access`)

func TestTheEmbeddedFilesCarryNoDeveloperNote(t *testing.T) {
	tables := storageTables(t)
	for name, body := range map[string][]byte{"agentcontract_gen.yaml": contract, "agentindex_gen.txt": index} {
		for i, line := range strings.Split(string(body), "\n") {
			if m := developerNotes.FindString(line); m != "" {
				t.Errorf("%s:%d carries %q, a note for the contract's developers", name, i+1, m)
			}
		}
	}
	// A table name is a wire name too (a field, an enum value). So only prose is
	// held to it: every description and summary, and the index's rows.
	for _, text := range append(proseOf(t, contract), strings.Split(string(index), "\n")...) {
		if m := tables.FindString(text); m != "" {
			t.Errorf("names the storage table %q: %s", m, text)
		}
	}
}

// proseOf returns every description and summary in a YAML document.
func proseOf(t *testing.T, src []byte) []string {
	t.Helper()
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		t.Fatalf("not YAML: %v", err)
	}
	var texts []string
	var walk func(*yaml.Node)
	walk = func(n *yaml.Node) {
		for i := 0; n.Kind == yaml.MappingNode && i+1 < len(n.Content); i += 2 {
			key, value := n.Content[i].Value, n.Content[i+1]
			if (key == "description" || key == "summary") && value.Kind == yaml.ScalarNode {
				texts = append(texts, value.Value)
			}
		}
		for _, child := range n.Content {
			walk(child)
		}
	}
	walk(&doc)
	if len(texts) < 100 {
		t.Fatalf("found %d descriptions; the contract carries hundreds, so the walk is not reading it", len(texts))
	}
	return texts
}

// storageTables reads the schema catalog's table names. Only names with an
// underscore are matched: "contact" and "deal" are also ordinary words.
func storageTables(t *testing.T) *regexp.Regexp {
	t.Helper()
	catalog, err := os.ReadFile("../../../migrations/testdata/head_catalog.txt")
	if err != nil {
		t.Fatalf("reading the schema catalog: %v", err)
	}
	var names []string
	for _, m := range regexp.MustCompile(`(?m)^public\.([a-z0-9]+_[a-z0-9_]+) acl=`).FindAllStringSubmatch(string(catalog), -1) {
		names = append(names, m[1])
	}
	if len(names) < 100 {
		t.Fatalf("read %d table names from the catalog; it holds hundreds, so the pattern no longer matches", len(names))
	}
	return regexp.MustCompile(`\b(` + strings.Join(names, "|") + `)\b`)
}
