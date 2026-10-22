// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The configuration files written from the settings registry are the ones its
// generator writes now. A setting declared, renamed or redocumented without
// `make config-gen` leaves the reference page, the templates and the editor
// schema describing a product that no longer exists.

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/deployconfig"
)

// The generator lives in the tools module, so it runs under the root
// workspace, as `make config-gen` runs it, whatever workspace the lane set.
func TestGeneratedConfigurationFilesAreCurrent(t *testing.T) {
	t.Parallel()
	work, err := filepath.Abs("../go.work")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "run", "./tools/gen-config", "-check")
	cmd.Env = append(os.Environ(), "GOWORK="+work)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generated configuration files are stale; run `make config-gen`:\n%s", out)
	}
}

// Every config this repo ships validates against the schema an editor checks
// it with. An operator copies these, so a schema that flags one tells them
// their starting point is wrong.
func TestEveryShippedConfigValidatesAgainstTheSchema(t *testing.T) {
	t.Parallel()
	schema := compiledConfigSchema(t)
	paths, err := filepath.Glob("../config/margince*.yaml")
	if err != nil {
		t.Fatalf("globbing the shipped configs: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no config/margince*.yaml found; the corpus moved and this gate checks nothing")
	}
	for _, path := range paths {
		raw, err := os.ReadFile(path) // #nosec G304 -- a fixed glob in the trusted source tree
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if err := schema.Validate(asJSONValue(t, path, raw)); err != nil {
			t.Errorf("%s does not validate against the schema an editor will check it with:\n%v", path, err)
		}
	}
}

// The loader reads an explicit null, or a key left bare, as unwritten. That is
// how an operator comments a section out without deleting it, so the schema
// accepts it at every level the loader does. Both loaders read the probe
// first, so a change to that premise fails here and not in the schema half.
func TestTheSchemaAcceptsAnExplicitNullWhereTheLoaderDoes(t *testing.T) {
	t.Parallel()
	const withNulls = "version: 1\nbootstrap_admin: null\nuploads:\n  attachment_mb: null\nemail: null\nlog_level: null\nai:\n  routing: null\n"
	path := filepath.Join(t.TempDir(), "margince.yaml")
	if err := os.WriteFile(path, []byte(withNulls), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := deployconfig.LoadBootstrap(path); err != nil {
		t.Fatalf("the bootstrap loader refuses an explicit null; this test's premise is gone: %v", err)
	}
	reg, err := compose.ProgramRegistry(config.RoleAPI)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(reg, config.Sources{
		Args: []string{"--config", path}, Environ: []string{},
		Known: compose.AllSettingNames(), BootstrapRoots: deployconfig.Roots(),
	}); err != nil {
		t.Fatalf("the settings loader refuses an explicit null; this test's premise is gone: %v", err)
	}
	if err := compiledConfigSchema(t).Validate(asJSONValue(t, "probe", []byte(withNulls))); err != nil {
		t.Errorf("the schema rejects a null the loader accepts:\n%v", err)
	}
}

// Uncommented, every setting line of the generated example validates against
// the generated schema, the bootstrap part included. So the schema takes each
// default it shows. Secrets stay commented: their reference names a file.
func TestTheUncommentedExampleValidatesAgainstTheSchema(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../config/margince.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	head, body, found := strings.Cut(string(raw), "\n# ---- ")
	if !found {
		t.Fatal("the example has no setting group; the generator's layout moved and this test checks nothing")
	}
	lines, uncommented := []string{head}, 0
	for line := range strings.SplitSeq(body, "\n") {
		text, commented := strings.CutPrefix(line, "# ")
		if !commented || strings.HasPrefix(text, "---- ") || strings.Contains(text, "${file:/run/secrets/") {
			continue
		}
		lines = append(lines, text)
		uncommented++
	}
	if uncommented == 0 {
		t.Fatal("no setting line was uncommented; this test checks nothing")
	}
	doc := []byte(strings.Join(lines, "\n") + "\n")
	if err := compiledConfigSchema(t).Validate(asJSONValue(t, "the uncommented example", doc)); err != nil {
		t.Errorf("the schema refuses the example's own defaults:\n%v", err)
	}
}

func compiledConfigSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	raw, err := os.Open("../config/margince.schema.json")
	if err != nil {
		t.Fatalf("open schema: %v", err)
	}
	defer func() {
		if cerr := raw.Close(); cerr != nil {
			t.Errorf("close schema: %v", cerr)
		}
	}()
	doc, err := jsonschema.UnmarshalJSON(raw)
	if err != nil {
		t.Fatalf("parse schema: %v", err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("margince.json", doc); err != nil {
		t.Fatalf("add schema: %v", err)
	}
	sch, err := c.Compile("margince.json")
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}
	return sch
}

// asJSONValue reads YAML as the value a JSON Schema validator walks. The JSON
// round trip turns yaml's integer types into the numbers the validator takes.
//
//craft:ignore naked-any jsonschema.Validate takes any; this is the library's seam, not a shape of ours
func asJSONValue(t *testing.T, name string, raw []byte) any {
	t.Helper()
	var doc any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("%s is not parseable yaml: %v", name, err)
	}
	encoded, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("%s will not round-trip to json: %v", name, err)
	}
	var value any
	if err := json.Unmarshal(encoded, &value); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return value
}
