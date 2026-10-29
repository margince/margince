// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/deployconfig"
)

// twoSettings is a bounded int the api reads, and a secret both programs read.
func twoSettings(t *testing.T) []setting {
	t.Helper()
	limit := config.Define(config.Spec[int]{
		Key: "uploads.attachment_mb", Env: "MARGINCE_UPLOADS_ATTACHMENT_MB", Flag: "attachment-mb", Type: config.Int,
		Default: config.Some(25), Min: config.Some(1), Max: config.Some(100),
		Roles: []config.Role{config.RoleAPI}, Doc: "Largest attachment, in decimal MB.",
	})
	token := config.Define(config.Spec[string]{
		Key: "license.token", Env: "MARGINCE_LICENSE", Type: config.String, Secret: true, AllowNone: true,
		Roles: []config.Role{config.RoleAPI, config.RoleWorker}, Doc: "Licence token.",
	})
	api, err := config.NewRegistry([]config.Any{limit, token})
	if err != nil {
		t.Fatal(err)
	}
	worker, err := config.NewRegistry([]config.Any{token})
	if err != nil {
		t.Fatal(err)
	}
	settings, err := collect(map[config.Role]*config.Registry{config.RoleAPI: api, config.RoleWorker: worker})
	if err != nil {
		t.Fatal(err)
	}
	return settings
}

func TestTheTablesNameEachSettingOnceWithItsPrograms(t *testing.T) {
	want := "<!-- Written by tools/gen-config from the settings registry: edit a setting's declaration, then run `make config-gen`. -->\n" +
		"\n### `license`\n\n" +
		"| Key | Env | Flag | Type | Default | Programs | Doc |\n|---|---|---|---|---|---|---|\n" +
		"| `license.token` | `MARGINCE_LICENSE` | — | `string`, secret | — | api, worker | Licence token. |\n" +
		"\n### `uploads`\n\n" +
		"| Key | Env | Flag | Type | Default | Programs | Doc |\n|---|---|---|---|---|---|---|\n" +
		"| `uploads.attachment_mb` | `MARGINCE_UPLOADS_ATTACHMENT_MB` | `--attachment-mb` | `int`, 1 to 100 | `25` | api | Largest attachment, in decimal MB. |\n" +
		"\n"
	if got := settingsTables(twoSettings(t)); got != want {
		t.Errorf("tables:\n%s\nwant:\n%s", got, want)
	}
}

func TestTheExampleIsTheBootstrapPartAlone(t *testing.T) {
	if got, want := exampleYAML("version: 1\n\n"), exampleHeader+"version: 1\n"; got != want {
		t.Errorf("example:\n%s\nwant:\n%s", got, want)
	}
}

func TestTheDefaultsFileNestsEachSettingAtItsDefault(t *testing.T) {
	want := defaultsHeader +
		"\nlicense:\n  token: ${file:/run/secrets/license.token}\n" +
		"\nuploads:\n  attachment_mb: 25\n"
	got, err := defaultsYAML(twoSettings(t))
	if err != nil || got != want {
		t.Errorf("defaults (%v):\n%s\nwant:\n%s", err, got, want)
	}
}

func TestTheEnvTemplateWritesEachVariableOnceOnOneLine(t *testing.T) {
	in := envInputs{
		loader: []config.LoaderVar{{Env: "MARGINCE_ENV", Doc: "The posture."}},
		entrypoint: []deployconfig.EntrypointVar{
			{Name: "MARGINCE_ADMIN_PASSWORD", Script: "api-entrypoint.sh"},
			{Name: "MARGINCE_LICENSE", Required: true, Script: "api-entrypoint.sh"},
		},
	}
	want := envHeader +
		"\n# ---- required by the image entrypoint ----\n# MARGINCE_LICENSE=${file:/run/secrets/license.token}\n" +
		"\n# ---- loader ----\n# MARGINCE_ENV=\n" +
		"\n# ---- image entrypoint ----\n# MARGINCE_ADMIN_PASSWORD=\n" +
		"\n# ---- uploads ----\n# MARGINCE_UPLOADS_ATTACHMENT_MB=25\n"
	if got := envExample(twoSettings(t), in); got != want {
		t.Errorf("env template:\n%s\nwant:\n%s", got, want)
	}
}

func TestTheSchemaClosesEachSectionAndCarriesTheBounds(t *testing.T) {
	pattern, err := json.Marshal(deployconfig.SecretRefPattern + `|^\s*\$\{none\}\s*$`)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"license":{"type":["object","null"],"additionalProperties":false,"properties":{"token":{` +
		`"description":"Licence token.","type":["string","null"],"pattern":` + string(pattern) +
		`,"examples":["${env:VARIABLE_NAME}","${file:/run/secrets/name}","${none}"]}}},` +
		`"uploads":{"type":["object","null"],"additionalProperties":false,"properties":{"attachment_mb":{` +
		`"description":"Largest attachment, in decimal MB.","type":["integer","null"],"minimum":1,"maximum":100,"default":25}}}}`
	props, err := withSettings(nil, twoSettings(t))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(props.raw()); got != want {
		t.Errorf("schema:\n%s\nwant:\n%s", got, want)
	}
}

// The generator reads no process value, so a secret set in the environment
// reaches no output, and each output marks the secret without a default.
func TestASecretRendersNoValueAndNoDefaultInAnyOutput(t *testing.T) {
	settings, err := collect(compose.ProgramRegistries())
	if err != nil {
		t.Fatal(err)
	}
	const sentinel = "do-not-print-this-secret"
	secrets := 0
	for _, s := range settings {
		if s.Secret() {
			secrets++
			t.Setenv(s.Env(), sentinel)
		}
	}
	if secrets == 0 {
		t.Fatal("no registry setting is a secret; this test checks nothing")
	}
	entrypoint, err := deployconfig.EntrypointVars("../../../scripts/deploy")
	if err != nil {
		t.Fatal(err)
	}
	files, err := render(settings, docsBegin+"\n"+docsEnd+"\n", envInputs{loader: config.LoaderVars(), entrypoint: entrypoint}, fieldDocs{})
	if err != nil {
		t.Fatal(err)
	}
	for path, body := range files {
		if bytes.Contains(body, []byte(sentinel)) {
			t.Errorf("%s prints a secret's value", path)
		}
	}
	for _, s := range settings {
		if !s.Secret() {
			continue
		}
		if !strings.Contains(string(files[envPath]), "\n# "+s.Env()+"=${file:/run/secrets/"+s.Key()+"}\n") {
			t.Errorf("%s: .env.example does not write the secret as a file reference", s.Key())
		}
		if !strings.Contains(string(files[defaultsPath]), ": ${file:/run/secrets/"+s.Key()+"}\n") {
			t.Errorf("%s: the defaults file does not write the secret as a file reference", s.Key())
		}
		row := tableRow(s)
		if cells := strings.Split(row, " | "); !strings.Contains(string(files[docsPath]), row) || cells[4] != emptyCell {
			t.Errorf("%s: the reference page shows no row, or a default, for a secret: %s", s.Key(), row)
		}
		node, err := settingSchema(s)
		if err != nil || bytes.Contains(node, []byte(`"default"`)) {
			t.Errorf("%s: the schema gives the secret a default (%v): %s", s.Key(), err, node)
		}
	}
}

// -check writes nothing, names each file that differs, and fails.
func TestCheckNamesTheFilesThatDiffer(t *testing.T) {
	root := t.TempDir()
	page := filepath.Join(root, docsPath)
	if err := os.MkdirAll(filepath.Dir(page), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(page, []byte("# Configuration\n\n"+docsBegin+"\n"+docsEnd+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	scripts := filepath.Join(root, entrypointDir)
	if err := os.MkdirAll(scripts, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scripts, "api-entrypoint.sh"), []byte(`: "${MARGINCE_DSN:?}"`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	pkg := filepath.Join(root, bootstrapDir)
	if err := os.MkdirAll(pkg, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkg, "doc.go"), []byte("package deployconfig\n\n// Bootstrap is the roots.\ntype Bootstrap struct{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run(root, true, &out); !errors.Is(err, errStale) || !strings.Contains(out.String(), envPath+"\n") {
		t.Fatalf("a tree with no generated files passed -check (%v):\n%s", err, out.String())
	}
	if err := run(root, false, &out); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := run(root, true, &out); err != nil || out.Len() != 0 {
		t.Fatalf("a freshly generated tree fails -check (%v):\n%s", err, out.String())
	}
	if err := os.WriteFile(filepath.Join(root, envPath), []byte("# edited by hand\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(root, envPath))
	if err != nil {
		t.Fatal(err)
	}
	if err := run(root, true, &out); !errors.Is(err, errStale) || out.String() != envPath+"\n" {
		t.Fatalf("-check after one edit = %v, listing %q; want only %s", err, out.String(), envPath)
	}
	after, err := os.ReadFile(filepath.Join(root, envPath))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("-check rewrote %s (%v)", envPath, err)
	}
}

// A key, flag or path in a doc reads as code on the page; a plain word does not.
func TestADocSetsWhatOneTypesInCode(t *testing.T) {
	got := docText("Read by POST /v1/attachments (see license.token_file) on the Pub/Sub push, topic projects/<p>/topics/<t>; defaults to --public-base-url.")
	want := "Read by POST `/v1/attachments` (see `license.token_file`) on the Pub/Sub push, topic `projects/<p>/topics/<t>`; defaults to `--public-base-url`."
	if got != want {
		t.Errorf("docText = %q, want %q", got, want)
	}
}

// A default the schema node of its setting refuses stops generation.
func TestADefaultItsOwnSchemaRefusesStopsGeneration(t *testing.T) {
	settings := twoSettings(t)
	node := func(kind string) json.RawMessage {
		return json.RawMessage(`{"properties":{"uploads":{"properties":{"attachment_mb":{"type":"` + kind + `"}}}}}`)
	}
	if err := checkDefaults(node("integer"), settings); err != nil {
		t.Fatalf("a default its node takes was refused: %v", err)
	}
	if err := checkDefaults(node("string"), settings); err == nil || !strings.Contains(err.Error(), "uploads.attachment_mb") {
		t.Fatalf("a default its node refuses passed, or the error names no setting: %v", err)
	}
}

// The .env reader trims a value and cuts it at " #", so such a default is
// quoted, with the quote the value does not hold.
func TestAnEnvDefaultIsQuotedWhereTheReaderWouldChangeIt(t *testing.T) {
	for text, want := range map[string]string{
		"plain":        "plain",
		" padded":      "' padded'",
		"a #hash":      "'a #hash'",
		"it's padded ": `"it's padded "`,
		`"quoted"`:     `'"quoted"'`,
	} {
		s := config.Define(config.Spec[string]{Key: "x", Env: "X_ENV", Type: config.String, Default: config.Some(text), Doc: "d"})
		shape, err := s.Shape()
		if err != nil {
			t.Fatal(err)
		}
		if got := envDefault(setting{Any: s, shape: shape}); got != want {
			t.Errorf("envDefault(%q) = %s, want %s", text, got, want)
		}
	}
}
