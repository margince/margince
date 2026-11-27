// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind falsification H2

//go:build !integration

package gates

// Each read form the frontend read census claims to see is seen, and each shape
// that only looks like a read is not.

import (
	"slices"
	"testing"
)

func plantedTree() repoTree {
	return newRepoTree([]string{
		"docs/reference/copy.md",
		"docs/handbook/a.md",
		"backend/internal/a/cases.json",
		"backend/internal/a/one.go",
		"backend/internal/a/two.go",
		"scripts/check.sh",
		"frontend/census/tree.ts",
		"frontend/src/testing/docs.ts",
		"frontend/src/testing/paths.ts",
		"frontend/src/x.ts",
	})
}

const (
	plantedCensusTree = `export const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), "..");
export const repoRoot = join(frontendRoot, "..");`
	rootFromScripts = `const repoRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");` + "\n"
	rootFromSrcA    = `const repoRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..", "..");` + "\n"
	spawnImport     = `import { execFileSync, execSync } from "node:child_process";` + "\n"
)

type readFormCase struct {
	name   string
	reader string
	source string
	// others are the further sources the reader imports or is imported by.
	others map[string]string
	want   []string
}

func readFormCases() []readFormCase {
	copyMD := []string{"docs/reference/copy.md"}
	both := []string{"backend/internal/a/one.go", "backend/internal/a/two.go"}
	return []readFormCase{
		{name: "join on a root declared in the file", reader: "frontend/scripts/r.ts",
			source: rootFromScripts + `readFileSync(join(repoRoot, "docs", "reference", "copy.md"));`, want: copyMD},
		{name: "resolve from the file's own directory", reader: "frontend/src/i18n/r.test.ts",
			source: `const here = dirname(fileURLToPath(import.meta.url));
readFileSync(resolve(here, "..", "..", "..", "docs", "reference", "copy.md"));`, want: copyMD},
		{name: "a root imported from another module", reader: "frontend/census/rules/r.census.ts",
			source: `import { repoRoot } from "../tree";
readFileSync(join(repoRoot, "docs/reference/copy.md"));`, want: copyMD},
		{name: "a root through a namespace import", reader: "frontend/src/a/r.test.ts",
			source: `import * as paths from "../testing/paths";
readFileSync(join(paths.repoRoot, "docs/reference/copy.md"));`,
			others: map[string]string{"frontend/src/testing/paths.ts": `export const repoRoot = join(import.meta.dirname, "..", "..", "..");`}, want: copyMD},
		{name: "a root a function returns", reader: "frontend/src/a/r.test.ts",
			source: `function root() {
  return join(import.meta.dirname, "..", "..", "..");
}
readFileSync(join(root(), "docs/reference/copy.md"));`, want: copyMD},
		{name: "import.meta.dirname", reader: "frontend/src/a/r.test.ts",
			source: `readFileSync(join(import.meta.dirname, "../../../docs/reference/copy.md"));`, want: copyMD},
		{name: "a default-imported path module", reader: "frontend/src/a/r.test.ts",
			source: `import nodePath from "node:path";
readFileSync(nodePath.join(import.meta.dirname, "../../../docs/reference/copy.md"));`, want: copyMD},
		{name: "path.posix.join", reader: "frontend/src/a/r.test.ts",
			source: `import path from "node:path";
readFileSync(path.posix.join(import.meta.dirname, "..", "..", "..", "docs/reference/copy.md"));`, want: copyMD},
		{name: "a + chain on a root", reader: "frontend/src/a/r.test.ts",
			source: rootFromSrcA + `readFileSync(repoRoot + "/docs/reference/copy.md");`, want: copyMD},
		{name: "a spread of Array(n).fill", reader: "frontend/src/features/deals/r.test.ts",
			source: `const here = dirname(fileURLToPath(import.meta.url));
readFileSync(resolve(here, ...Array(4).fill(".."), "docs", "reference", "copy.md"));`, want: copyMD},
		{name: "a spread of a named list", reader: "frontend/scripts/r.ts",
			source: rootFromScripts + `const parts = ["docs", "reference", "copy.md"];
readFileSync(join(repoRoot, ...parts));`, want: copyMD},
		{name: "a module specifier that escapes frontend/", reader: "frontend/src/a/r.test.ts",
			source: `import cases from "../../../backend/internal/a/cases.json";`, want: []string{"backend/internal/a/cases.json"}},
		{name: "a ?raw import reads the file", reader: "frontend/src/a/r.test.ts",
			source: `import page from "../../../docs/reference/copy.md?raw";`, want: copyMD},
		{name: "new URL on import.meta.url", reader: "frontend/src/a/r.test.ts",
			source: `readFileSync(new URL("../../../docs/reference/copy.md", import.meta.url));`, want: copyMD},
		{name: "a template filled from the callers", reader: "frontend/src/a/r.test.ts",
			source: "function source(name: string) {\n  return readFileSync(new URL(`../../../backend/internal/a/${name}`, import.meta.url));\n}\nsource(\"one.go\");\nsource(\"two.go\");",
			want:   both},
		{name: "an arrow helper", reader: "frontend/src/a/r.test.ts",
			source: `const root = join(import.meta.dirname, "..", "..", "..");
const read = (f: string) => readFileSync(join(root, f), "utf8");
read("docs/reference/copy.md");`, want: copyMD},
		{name: "a helper imported by its caller", reader: "frontend/src/testing/docs.ts",
			source: `export function readDoc(name: string) {
  return readFileSync(join(import.meta.dirname, "../../../docs/reference", name), "utf8");
}`,
			others: map[string]string{"frontend/src/a/r.test.ts": `import { readDoc } from "../testing/docs";
readDoc("copy.md");`}, want: copyMD},
		{name: "an arrow helper imported by its caller", reader: "frontend/src/testing/docs.ts",
			source: `export const readDoc = (name: string) => readFileSync(join(import.meta.dirname, "../../../docs/reference", name));`,
			others: map[string]string{"frontend/src/a/r.test.ts": `import { readDoc } from "../testing/docs";
readDoc("copy.md");`}, want: copyMD},
		{name: "a for-of over a list of names", reader: "frontend/src/a/r.test.ts",
			source: `const dir = join(import.meta.dirname, "../../../backend/internal/a");
for (const file of ["one.go", "two.go"]) readFileSync(join(dir, file));`, want: both},
		{name: "a for-of over a named list", reader: "frontend/src/a/r.test.ts",
			source: `const dir = join(import.meta.dirname, "../../../backend/internal/a");
const names = ["one.go", "two.go"];
for (const file of names) readFileSync(join(dir, file));`, want: both},
		{name: "a map over a named list", reader: "frontend/src/a/r.test.ts",
			source: `const dir = join(import.meta.dirname, "../../../backend/internal/a");
const names = ["one.go", "two.go"];
names.map((name) => readFileSync(join(dir, name)));`, want: both},
		{name: "an it.each table", reader: "frontend/src/a/r.test.ts",
			source: "const dir = join(import.meta.dirname, \"../../../backend/internal/a\");\nit.each([[\"one.go\", 1], [\"two.go\", 2]])(\"%s\", (file, n) => {\n  readFileSync(`${dir}/${file}`);\n});",
			want:   both},
		{name: "a describe.each list", reader: "frontend/src/a/r.test.ts",
			source: `const dir = join(import.meta.dirname, "../../../backend/internal/a");
describe.each(["one.go", "two.go"])("%s", (file) => { readFileSync(join(dir, file)); });`, want: both},
		{name: "a directory it walks", reader: "frontend/src/a/r.test.ts",
			source: `const handbook = join(import.meta.dirname, "../../../docs/handbook");
readdirSync(handbook);`, want: []string{"docs/handbook"}},
		{name: "an fs call on a path relative to frontend/", reader: "frontend/scripts/r.ts",
			source: `readFileSync("../docs/reference/copy.md");`, want: copyMD},
		{name: "a glob", reader: "frontend/src/a/r.ts",
			source: `import.meta.glob("../../../docs/reference/*.md");`, want: []string{"docs/reference"}},
		{name: "a fast-glob call with a cwd", reader: "frontend/scripts/r.ts",
			source: rootFromScripts + `fg.sync("reference/*.md", { cwd: join(repoRoot, "docs") });`, want: []string{"docs/reference"}},
		{name: "a path it reads and writes", reader: "frontend/scripts/r.mjs",
			source: `const out = join(import.meta.dirname, "../../docs/reference/copy.md");
if (check) readFile(out); else writeFile(out, text);`, want: copyMD},
		{name: "git ls-files with a pathspec", reader: "frontend/scripts/r.ts",
			source: spawnImport + rootFromScripts + `execFileSync("git", ["ls-files", "--", "docs/handbook"], { cwd: repoRoot });`,
			want:   []string{"docs/handbook"}},
		{name: "git ls-files in a command string", reader: "frontend/scripts/r.ts",
			source: spawnImport + rootFromScripts + `execSync("git ls-files docs/handbook", { cwd: repoRoot });`,
			want:   []string{"docs/handbook"}},
		{name: "git ls-files of the whole repository", reader: "frontend/scripts/r.ts",
			source: spawnImport + rootFromScripts + `execFileSync("git", ["ls-files", "-z"], { cwd: repoRoot });`,
			want:   []string{"."}},
		{name: "a subprocess's directory, beside a read under it", reader: "frontend/scripts/r.ts",
			source: spawnImport + rootFromScripts + `readFileSync(join(repoRoot, "backend/internal/a/cases.json"));
execFileSync("go", ["run", "./internal/a"], { cwd: join(repoRoot, "backend") });`,
			want: []string{"backend", "backend/internal/a", "backend/internal/a/cases.json"}},
		{name: "a script a subprocess runs", reader: "frontend/scripts/r.test.ts",
			source: spawnImport + `execFileSync("bash", ["../scripts/check.sh", "-"]);`, want: []string{"scripts/check.sh"}},
		{name: "a JSON config escape", reader: "frontend/package.json",
			source: `{"scripts": {"gen": "tool ../docs/reference/copy.md -o src/x.ts"}}`, want: copyMD},
		{name: "a YAML config escape", reader: "frontend/config.yaml",
			source: "# ../docs/handbook/a.md\nsource: ../docs/reference/copy.md\n", want: copyMD},
		{name: "a shell path after a variable", reader: "frontend/scripts/r.sh",
			source: `cat "$ROOT/docs/reference/copy.md"`, want: copyMD},
		{name: "a sourced shell file", reader: "frontend/scripts/r.sh",
			source: `source "$ROOT/scripts/check.sh"`, want: []string{"scripts/check.sh"}},
		{name: "a script with no ending and a shebang", reader: "frontend/scripts/tool",
			source: "#!/usr/bin/env bash\ncat ../docs/reference/copy.md\n", want: copyMD},
		{name: "a call after a regular expression that holds a backtick", reader: "frontend/src/a/r.test.ts",
			source: "const quoted = /" + "`" + "([^" + "`" + "]+)" + "`" + "/g;\nreadFileSync(join(import.meta.dirname, \"../../../docs/reference/copy.md\"));", want: copyMD},
		{name: "a generic arrow in a .tsx file", reader: "frontend/src/a/r.test.tsx",
			source: `const pick = <T,>(value: T) => value;
readFileSync(join(import.meta.dirname, "../../../docs/reference/copy.md"));`, want: copyMD},
		{name: "an arrow with a return type", reader: "frontend/src/a/r.test.ts",
			source: `const root = join(import.meta.dirname, "..", "..", "..");
const read = (f: string): string => readFileSync(join(root, f), "utf8");
read("docs/reference/copy.md");`, want: copyMD},
		{name: "a stylesheet @import", reader: "frontend/src/a/r.css",
			source: `@import "../../../docs/reference/copy.md";`, want: copyMD},
		{name: "a page's script src", reader: "frontend/r.html",
			source: `<script type="module" src="../docs/reference/copy.md"></script>`, want: copyMD},
		{name: "a heredoc a shell runs", reader: "frontend/scripts/r.sh",
			source: "bash <<'EOF'\ncat ../docs/reference/copy.md\nEOF\n", want: copyMD},
		{name: "a heredoc no shell runs", reader: "frontend/scripts/r.sh",
			source: "cat <<EOF\n../docs/reference/copy.md\nEOF\n"},
		{name: "mkdir in a shell script", reader: "frontend/scripts/r.sh",
			source: "mkdir -p ../docs/handbook\n"},
		{name: "a write command a subprocess runs", reader: "frontend/scripts/r.ts",
			source: spawnImport + rootFromScripts + `execFileSync("mkdir", ["-p", join(repoRoot, "docs/handbook")]);`},
		{name: "a script path a subprocess is handed", reader: "frontend/scripts/r.ts",
			source: spawnImport + rootFromScripts + `execFileSync("bash", [join(repoRoot, "scripts/check.sh")]);`, want: []string{"scripts/check.sh"}},
		{name: "a comment", reader: "frontend/src/a/r.ts",
			source: `// readFileSync(join(repoRoot, "docs/reference/copy.md"))`},
		{name: "a path literal no call reads", reader: "frontend/src/a/r.ts",
			source: `const page = "../../../docs/reference/copy.md";`},
		{name: "a path it only writes", reader: "frontend/e2e/r.spec.ts",
			source: `const dir = path.join("..", "docs", "reference");
fs.mkdirSync(dir);
fs.writeFileSync(path.join(dir, "copy.md"), text);`},
		{name: "RegExp exec, which spawns nothing", reader: "frontend/src/a/r.ts",
			source: spawnImport + `/x/.exec("git ls-files docs/handbook");`},
		{name: "an exec no child_process import provides", reader: "frontend/scripts/r.ts",
			source: rootFromScripts + `exec("git ls-files docs/handbook", { cwd: repoRoot });`},
		{name: "a path inside frontend/", reader: "frontend/src/a/r.ts",
			source: `readFileSync(join(import.meta.dirname, "../x.ts"));`},
	}
}

func TestTheFrontendReadCensusSeesEachReadForm(t *testing.T) {
	t.Parallel()
	for _, c := range readFormCases() {
		sources := map[string]string{"frontend/census/tree.ts": plantedCensusTree}
		if readerKindOf(c.reader, c.source) == scriptReader {
			sources[c.reader] = c.source
		}
		for file, source := range c.others {
			sources[file] = source
		}
		ts, err := newTSReader(plantedTree(), sources)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		got := readsOf(ts, c.reader, c.source)
		slices.Sort(got)
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: the census read %v from %s, want %v", c.name, got, c.reader, c.want)
		}
	}
}

func TestAJobReadsItsActionsAndTheCommandsItRuns(t *testing.T) {
	t.Parallel()
	var jobs laneJobs
	steps := []laneStep{
		{Uses: "./.github/actions/probe"},
		{Run: "cd frontend && \\\n  cat ../docs/reference/copy.md\nmake -k -C backend lint", WorkingDirectory: "."},
		{Run: "cat ../docs/handbook/a.md"},
	}
	jobs.walkSteps(t, plantedTree(), "job probe", steps, "backend")
	var reads []string
	for _, read := range jobs.reads {
		reads = append(reads, read.path)
	}
	slices.Sort(reads)
	if want := []string{".github/actions/probe", "docs/handbook/a.md", "docs/reference/copy.md"}; !slices.Equal(reads, want) {
		t.Errorf("the planted job read %v, want %v", reads, want)
	}
	if !slices.Equal(jobs.makes, []makeCall{{"backend", "lint"}}) {
		t.Errorf("the planted job makes %v, want backend lint", jobs.makes)
	}
}

func TestARecipeReadIsPlacedOnTheDirectoryItsCdLeft(t *testing.T) {
	t.Parallel()
	cases := []struct {
		dir, line string
		want      []string
	}{
		{".", "\tcd frontend && node scripts/x.mjs ../docs/reference/copy.md", []string{"docs/reference/copy.md"}},
		{".", "\t@bash scripts/check.sh docs/handbook/a.md", []string{"docs/handbook/a.md", "scripts/check.sh"}},
		{".", "\t@./scripts/check.sh", []string{"scripts/check.sh"}},
		{"backend", "\tgo run ./internal/a", []string{"backend/internal/a"}},
		{".", "\t(cd backend && go run ./internal/a) && cat docs/handbook/a.md", []string{"backend/internal/a", "docs/handbook/a.md"}},
		{"frontend", "\tcat $(CURDIR)/../docs/reference/copy.md", []string{"docs/reference/copy.md"}},
		{".", "\tcd frontend && $(MAKE) -C .. x && cat $(CURDIR)/scripts/check.sh", []string{"scripts/check.sh"}},
		{".", "\tmkdir -p docs/handbook && rm -f docs/reference/copy.md", nil},
		{".", "\tcd frontend && pnpm exec vitest run src/x.ts", nil},
	}
	for _, c := range cases {
		got := recipeReads(plantedTree(), c.dir, c.line)
		slices.Sort(got)
		if !slices.Equal(got, c.want) {
			t.Errorf("recipeReads(%q, %q) = %v, want %v", c.dir, c.line, got, c.want)
		}
	}
	joined := commandLines([]string{"\tcd frontend && \\", "\t  cat ../docs/reference/copy.md"})
	if got := recipeReads(plantedTree(), ".", joined[0]); len(joined) != 1 || !slices.Equal(got, []string{"docs/reference/copy.md"}) {
		t.Errorf("a cd carried over a backslash continuation read %v from %q, want [docs/reference/copy.md]", got, joined)
	}
	if calls := makeCalls(".", "\tPHASE=1 make -k -C backend lint FE=1 && $(MAKE) fe-unit"); !slices.Equal(calls, []makeCall{{"backend", "lint"}, {".", "fe-unit"}}) {
		t.Errorf("makeCalls read %v, want backend lint and fe-unit", calls)
	}
}
