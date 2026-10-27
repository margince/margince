// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

package gates

// The MARGINCE_* variables the code reads, the reference pages name and an
// entrypoint requires agree with each other. Three obligations, all derived
// from the tree rather than a maintained list:
//
//  1. every MARGINCE_* var the Go code reads is named in
//     docs/reference/configuration.md (the settings), docs/reference/programs.md
//     or docs/reference/worker-diagnostics.md (every other variable);
//  2. every MARGINCE_* var those pages name is still part of the product;
//  3. every MARGINCE_* var a deploy entrypoint hard-requires is named in
//     .env.example — that file is the template
//     docs/how-to/deploy-margince.md hands an operator, so a var the container
//     refuses to boot without must not be discoverable only by reading the
//     entrypoint script.
//
// tools/gen-config writes .env.example and the settings tables of
// configuration.md from the registry, and configgenerated_test.go fails a
// stale copy. So no name in .env.example can be dead, and every documented
// default is the declared one; these obligations cover what the generator does
// not write.
//
// What these gates do NOT cover, stated rather than implied:
//
//   - Obligation 1 covers Go readers, in the licensed trees only. A var read
//     solely by deploy shell or a workflow is documented in programs.md
//     by hand, and this sweep walks the backend tree only.
//     Obligation 2 does count those non-Go readers, because there the
//     question is merely whether a name is still real.
//   - MARGINCE_-prefixed names only. The BYOK keys (GEMINI_API_KEY,
//     OPENAI_API_KEY, ANTHROPIC_API_KEY, OPENAI_COMPATIBLE_API_KEY) carry
//     provider-conventional names and are ungated. Widening the pattern to any
//     env-shaped literal would match unrelated constants and trade a sharp
//     gate for a noisy one.
//   - Whole double-quoted literals only. A name assembled at run time
//     ("MARGINCE_" + provider + "_KEY"), or written as a backquoted raw string,
//     is invisible to every obligation here, so adding one has to be a
//     deliberate act rather than a silent gap.
//     internal/platform/config/unknown.go is the one deliberate case: it
//     assembles the names of variables it deliberately does NOT read, so a
//     whole literal there would demand a documentation row for a variable no
//     process consults. Its own comment records the reason. Every backquoted
//     occurrence remains prose in a comment or an error message.
//   - Presence, not truth. A var being named in a document does not make the
//     prose around it accurate. These gates stop a var disappearing; they
//     cannot stop a description going stale.

import (
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/deployconfig"
)

// envVarName matches one MARGINCE_* name in prose — the reference doc and the
// template, where names appear unquoted. Applied to those files it is why
// .env.example may not write a `MARGINCE_FOO_*` glob: the glob yields the name
// `MARGINCE_FOO_`, which nothing reads.
var envVarName = regexp.MustCompile(`MARGINCE_[A-Z0-9_]+`)

// quotedEnvVarName matches one MARGINCE_* name as a Go string literal. Keying
// on the literal rather than on os.Getenv is what makes the Go sweep complete.
// Most vars reach the process through config settings and config.Lookup. Keying on unquoted text
// instead would harvest globs out of comments and invent vars that do not exist.
var quotedEnvVarName = regexp.MustCompile(`"(MARGINCE_[A-Z0-9_]+)"`)

const (
	configurationDoc = "../docs/reference/configuration.md"
	programsDoc      = "../docs/reference/programs.md"
	workerDoc        = "../docs/reference/worker-diagnostics.md"
	envExample       = "../.env.example"
)

// deploySurfaceRoots are the non-Go paths that configure a deployment: the
// entrypoints and helper scripts, the dev compose stack, and the CI definitions.
// A variable read only here is as real as one Go reads, so obligation 2
// must accept it. MARGINCE_ADMIN_PASSWORD is the current example: the entrypoint
// turns it into the file reference margince.yaml names — OPS-CFG-3 keeps secret
// VALUES out of every config layer the product reads — so a Go-only definition of
// "live" would report it as dead in a file whose purpose is to offer it to an
// operator.
var deploySurfaceRoots = []string{"../scripts", "../docker-compose.dev.yml", "../.github/workflows"}

// envVarsReadByGoCode maps each MARGINCE_* name a Go string literal spells to
// the first file spelling it. The trees are the ones the license sweep covers
// (licensedTrees, license_test.go): extensions/ and fixtures/ are separate
// modules a `./...` from here never reaches, and a var a unit reads is as real
// as one the backend reads. Sharing that list means a new tree enrolls in both
// gates at once instead of silently in only one.
func envVarsReadByGoCode(t *testing.T) map[string]string {
	t.Helper()
	sites := map[string]string{}
	for _, tree := range licensedTrees {
		walkHandWrittenGoFiles(t, tree.root, func(path, text string) {
			for _, m := range quotedEnvVarName.FindAllStringSubmatch(text, -1) {
				if _, seen := sites[m[1]]; !seen {
					sites[m[1]] = path
				}
			}
		})
	}
	if len(sites) == 0 {
		t.Fatal("no MARGINCE_* env var found in any Go tree — a sweep that scans nothing passes exactly like a clean one")
	}
	return sites
}

// liveEnvVars is every MARGINCE_* name the product still mentions anywhere it
// configures itself: Go literals plus the deploy surface. The set is
// intentionally over-inclusive — a script that sets a variable for a child
// process is counted the same as one that reads it — because it is only ever
// used to permit a name to appear in a document. Obligation 1, which requires
// documentation, stays Go-only for the opposite reason: there the same
// imprecision would demand rows for variables no operator configures.
func liveEnvVars(t *testing.T) map[string]string {
	t.Helper()
	live := envVarsReadByGoCode(t)
	for _, root := range deploySurfaceRoots {
		walkTextFiles(t, root, func(path, text string) {
			for _, name := range envVarName.FindAllString(text, -1) {
				if _, seen := live[name]; !seen {
					live[name] = path
				}
			}
		})
	}
	images, err := filepath.Glob("../Dockerfile*")
	if err != nil {
		t.Fatalf("globbing Dockerfiles: %v", err)
	}
	for _, path := range images {
		b, err := os.ReadFile(path) // #nosec G304 -- path comes from a fixed glob in the trusted source tree
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		for _, name := range envVarName.FindAllString(string(b), -1) {
			if _, seen := live[name]; !seen {
				live[name] = filepath.ToSlash(path)
			}
		}
	}
	return live
}

// namesIn returns the set of MARGINCE_* names a document spells out. Matching
// whole names rather than searching for substrings is what makes the gates
// exact in both directions: a doc mentioning only MARGINCE_AICERT_MODEL must
// not be read as covering the separate MARGINCE_AICERT, and an abbreviation
// like `MARGINCE_GMAIL_CLIENT_ID` / `…_SECRET` covers only the name it actually
// writes out. A variable name a reader cannot grep for is not documented.
func namesIn(t *testing.T, path string) map[string]bool {
	t.Helper()
	b, err := os.ReadFile(path) // #nosec G304 -- fixed path in the trusted source tree
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	names := map[string]bool{}
	for _, name := range envVarName.FindAllString(string(b), -1) {
		names[name] = true
	}
	if len(names) == 0 {
		t.Fatalf("%s names no MARGINCE_* var at all — a file that documents nothing cannot be the table of record", path)
	}
	return names
}

// TestEveryEnvVarIsDocumented: shipping a var the reference doc has never heard
// of leaves an operator guessing, and a secret nobody wrote down cannot be
// provisioned at all.
func TestEveryEnvVarIsDocumented(t *testing.T) {
	t.Parallel()
	documented := namesIn(t, configurationDoc)
	maps.Copy(documented, namesIn(t, programsDoc))
	maps.Copy(documented, namesIn(t, workerDoc))
	sites := envVarsReadByGoCode(t)

	var undocumented []string
	for _, name := range slices.Sorted(maps.Keys(sites)) {
		if !documented[name] {
			undocumented = append(undocumented, name+" (read in "+sites[name]+")")
		}
	}
	if len(undocumented) > 0 {
		t.Errorf("%d env var(s) read by Go code but not named in %s, %s or %s — add a row to the settings or to programs.md, spelling the name out in full (an abbreviated suffix like `…_SECRET` does not count):\n\t%s",
			len(undocumented), configurationDoc, programsDoc, workerDoc, strings.Join(undocumented, "\n\t"))
	}
}

// TestConfigurationDocNamesOnlyLiveVars holds the table of record to the same
// bar as the template it now absorbs. Without it the more authoritative file is
// the one free to keep a row for a var that no longer exists.
func TestConfigurationDocNamesOnlyLiveVars(t *testing.T) {
	t.Parallel()
	assertNamesOnlyLiveVars(t, configurationDoc)
	assertNamesOnlyLiveVars(t, programsDoc)
	assertNamesOnlyLiveVars(t, workerDoc)
}

// entrypointRequired is each var a deploy entrypoint stops without, mapped to
// the script that reads it, from deployconfig.EntrypointVars. The generated
// .env.example lists the same set first, under its own heading, from the same call.
// Its stated limits are these gates' limits too: only the `${VAR:?}` and
// `${VAR?}` forms are seen, never an explicit guard or `set -u`.
func entrypointRequired(t *testing.T) map[string]string {
	t.Helper()
	vars, err := deployconfig.EntrypointVars("../scripts/deploy")
	if err != nil {
		t.Fatal(err)
	}
	required := map[string]string{}
	for _, v := range vars {
		if v.Required {
			required[v.Name] = "../scripts/deploy/" + v.Script
		}
	}
	if len(required) == 0 {
		t.Fatal("no `${MARGINCE_…:?}` requirement found under ../scripts/deploy — a sweep that scans nothing passes exactly like a clean one")
	}
	return required
}

// TestEntrypointRequiredVarsAreInTheEnvExample: a var the container refuses to
// boot without belongs in the file an operator is handed, not only in the
// script that rejects them. docs/how-to/deploy-margince.md names .env.example
// as that file, which is what makes this an obligation rather than a nicety.
func TestEntrypointRequiredVarsAreInTheEnvExample(t *testing.T) {
	t.Parallel()
	offered := namesIn(t, envExample)

	required := entrypointRequired(t)
	var missing []string
	for _, name := range slices.Sorted(maps.Keys(required)) {
		if !offered[name] {
			missing = append(missing, name+" (required by "+required[name]+")")
		}
	}
	if len(missing) > 0 {
		t.Errorf("%d var(s) a deploy entrypoint refuses to boot without, absent from %s — add them, commented, with the value shape an operator must replace:\n\t%s",
			len(missing), envExample, strings.Join(missing, "\n\t"))
	}
}

// TestEntrypointRequiredVarsAreDocumented is obligation 1 for the half it never
// reached: a var the deploy surface refuses to boot without has to be written
// down somewhere an operator reads.
//
// Obligation 1 covers Go readers only, and says so. Obligation 2 counts the
// deploy surface, but only in the permissive direction. It asks whether a name
// a document mentions is still real, never whether a real name is mentioned.
// So a var an entrypoint hard-requires was documented if the author
// remembered, which is the failure mode obligation 1 removed for Go.
// margince/margince#566.
//
// PROGRAMS.MD, the table of record for variables that are not settings, in the
// section it gives the entrypoint variables. Splitting variables across two documents by which process happens
// to read them is how an operator ends up checking the wrong one. The section,
// not the whole file, is read. `MARGINCE_DSN` is named in a dozen other rows, so
// a whole-file match would pass with the entrypoint section deleted.
//
// It reuses obligation 3's `${VAR:?}` set rather than sweeping for mentions,
// and inherits that set's stated limits. The `:-default` form is not a
// requirement, and an explicit guard or `set -u` is invisible to both. A var
// with a fallback is not one an operator must supply; requiring those would
// admit most of the deploy surface and leave the label meaningless.
func TestEntrypointRequiredVarsAreDocumented(t *testing.T) {
	t.Parallel()
	documented := namesInSection(t, programsDoc, entrypointSection)

	required := entrypointRequired(t)
	var missing []string
	for _, name := range slices.Sorted(maps.Keys(required)) {
		if !documented[name] {
			missing = append(missing, name+" (required by "+required[name]+")")
		}
	}
	if len(missing) > 0 {
		t.Errorf("%d var(s) a deploy entrypoint refuses to boot without, absent from %s — an operator "+
			"provisioning this deployment has no way to learn they must supply them:\n\t%s",
			len(missing), programsDoc+" "+entrypointSection, strings.Join(missing, "\n\t"))
	}
}

// entrypointSection is the programs.md heading that owns the variables the
// deploy entrypoints read before any binary starts.
const entrypointSection = "## What the image entrypoint reads"

// namesInSection is namesIn narrowed to one `## ` section: from the line that
// starts with heading to the next `## ` line.
func namesInSection(t *testing.T, path, heading string) map[string]bool {
	t.Helper()
	b, err := os.ReadFile(path) // #nosec G304 -- fixed path in the trusted source tree
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	_, after, found := strings.Cut(string(b), "\n"+heading)
	if !found {
		t.Fatalf("%s has no %q section — the entrypoint variables have no home an operator can find", path, heading)
	}
	section, _, _ := strings.Cut(after, "\n## ")
	names := map[string]bool{}
	for _, name := range envVarName.FindAllString(section, -1) {
		names[name] = true
	}
	if len(names) == 0 {
		t.Fatalf("%s %q names no MARGINCE_* var at all — a section that documents nothing cannot be the table of record", path, heading)
	}
	return names
}

func assertNamesOnlyLiveVars(t *testing.T, path string) {
	t.Helper()
	live := liveEnvVars(t)

	var dead []string
	for name := range namesIn(t, path) {
		if _, ok := live[name]; !ok {
			dead = append(dead, name)
		}
	}
	slices.Sort(dead)
	if len(dead) > 0 {
		t.Errorf("%d var(s) named in %s that nothing in the product reads — delete them. A `MARGINCE_FOO_*` glob counts as the name `MARGINCE_FOO_`, which nothing reads: spell such vars out instead of globbing them:\n\t%s",
			len(dead), path, strings.Join(dead, "\n\t"))
	}
}
