// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Every TypeScript file the repository keeps under a frontend is a ROOT of a
// tsconfig project that a lane actually compiles. vitest, Storybook and
// Playwright transpile without typechecking, so a file no built project lists
// runs green whatever its types say.
//
// "Built" is read from the lanes: each `tsc` invocation in a tracked Makefile or
// in a frontend/package.json script a Makefile runs, with `tsc -b` followed
// through its references. A config
// on disk that no lane builds is a finding too — its include would otherwise
// look like coverage while nothing ever compiles it.
//
// Roots, not files a program reaches: a test is imported by nothing, so
// "reached" would pass exactly the files this exists to find. They come from
// TypeScript's own parse of each config, which applies include, exclude and the
// extension priority that keeps one of `x.ts`, `x.tsx`, `x.d.ts` when several
// match — so no program is built. A declaration file is held like any other.

import { execFileSync } from "node:child_process";
import { existsSync, readFileSync, statSync } from "node:fs";
import { dirname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { beforeAll, describe, expect, it } from "vitest";

const repo = resolve(dirname(fileURLToPath(import.meta.url)), "..", "..");

const TYPESCRIPT_FILE = /\.(ts|tsx|mts|cts)$/;
const TSCONFIG = /(^|\/)tsconfig[^/]*\.json$/;
const MAKEFILE = /(^|\/)(Makefile|[^/]+\.mk)$/;
// The core frontend, and every extension's frontend layer at any depth.
const FRONTEND_PATH = /^(frontend|extensions\/(.+\/)?frontend)\//;

// Tracked files and new ones git would stage, so a file is caught before its
// first commit; .gitignore keeps build output out.
function repoFiles(): string[] {
  const listed = execFileSync(
    "git",
    ["ls-files", "-z", "--cached", "--others", "--exclude-standard"],
    { cwd: repo, encoding: "utf8" },
  );
  return [...new Set(listed.split("\0"))]
    .filter((path) => path !== "" && existsSync(join(repo, path)))
    .sort();
}

const messageOf = (diagnostic: ts.Diagnostic) =>
  ts.flattenDiagnosticMessageText(diagnostic.messageText, "\n");

function parseConfig(configPath: string): ts.ParsedCommandLine {
  const parsed = ts.getParsedCommandLineOfConfigFile(configPath, undefined, {
    ...ts.sys,
    onUnRecoverableConfigFileDiagnostic: (diagnostic) => {
      throw new Error(messageOf(diagnostic));
    },
  });
  if (parsed === undefined) throw new Error(`${configPath} did not parse`);
  const errors = parsed.errors.map(messageOf);
  if (errors.length > 0) throw new Error(`${configPath}: ${errors.join("; ")}`);
  return parsed;
}

// A project argument may name a directory, which tsc reads as its tsconfig.json.
const configAt = (path: string) =>
  existsSync(path) && statSync(path).isDirectory()
    ? join(path, "tsconfig.json")
    : path;

type Invocation = { config: string; build: boolean };

// The configs one shell command compiles, resolved against `cwd`. A leading
// `cd <dir> &&` moves it, which is how the Makefile reaches frontend/.
function configsInvoked(command: string, cwd: string): Invocation[] {
  const moved = /^[@-]*cd\s+(\S+)\s*&&/.exec(command.trim());
  const dir = moved ? resolve(cwd, moved[1]) : cwd;
  const calls = command.matchAll(/\btsc(?=[ \t]|$)((?:[ \t]+[^\s&|;]+)*)/gm);
  return [...calls].flatMap((call) => {
    const args = call[1].trim().split(/\s+/).filter(Boolean);
    const build = args.some((a) => a === "-b" || a === "--build");
    const project = args.findIndex((a) => a === "-p" || a === "--project");
    const named =
      project >= 0
        ? [args[project + 1]]
        : args.filter((a) => !a.startsWith("-"));
    return (named.length > 0 ? named : ["."]).map((path) => ({
      config: configAt(resolve(dir, path)),
      build,
    }));
  });
}

function scriptsOf(packageJson: string): Map<string, string> {
  const manifest: unknown = JSON.parse(readFileSync(packageJson, "utf8"));
  if (typeof manifest !== "object" || manifest === null) return new Map();
  const scripts: unknown = Reflect.get(manifest, "scripts");
  if (typeof scripts !== "object" || scripts === null) return new Map();
  return new Map(
    Object.entries(scripts).filter(
      (entry): entry is [string, string] => typeof entry[1] === "string",
    ),
  );
}

// The package scripts a lane runs: those a Makefile line names as `pnpm <name>`,
// and those they name in turn. A script nothing runs compiles nothing.
function scriptsRun(lines: string[], scripts: Map<string, string>): string[] {
  const run = new Set<string>();
  const pending = [...lines];
  for (let line = pending.pop(); line !== undefined; line = pending.pop()) {
    for (const [, name] of line.matchAll(/\bpnpm\s+(?:run\s+)?([\w:-]+)/g)) {
      const body = scripts.get(name);
      if (body === undefined || run.has(name)) continue;
      run.add(name);
      pending.push(body);
    }
  }
  return [...run].flatMap((name) => scripts.get(name) ?? []);
}

// Every config a lane compiles: each `tsc` in a Makefile recipe or a script
// one runs, and under `tsc -b` every project its references reach.
function builtConfigs(files: string[]): Map<string, ts.ParsedCommandLine> {
  const commands = files
    .filter((path) => MAKEFILE.test(path))
    .flatMap((path) =>
      readFileSync(join(repo, path), "utf8")
        .split("\n")
        .filter((line) => !line.trim().startsWith("#"))
        .map((line) => ({ line, cwd: dirname(join(repo, path)) })),
    );
  const pkg = join(repo, "frontend", "package.json");
  const scripts = scriptsRun(
    commands.map(({ line }) => line),
    scriptsOf(pkg),
  );
  for (const line of scripts) commands.push({ line, cwd: dirname(pkg) });

  const built = new Map<string, ts.ParsedCommandLine>();
  const expanded = new Set<string>();
  const pending = commands.flatMap(({ line, cwd }) =>
    configsInvoked(line, cwd),
  );
  for (let next = pending.pop(); next !== undefined; next = pending.pop()) {
    const parsed = built.get(next.config) ?? parseConfig(next.config);
    built.set(next.config, parsed);
    if (!next.build || expanded.has(next.config)) continue;
    expanded.add(next.config);
    for (const ref of parsed.projectReferences ?? [])
      pending.push({
        config: ts.resolveProjectReferencePath(ref),
        build: true,
      });
  }
  return built;
}

// Names the sibling that shadows `path`, because the fix for a dropped
// basename twin is a rename, not an include.
function shadowedBy(path: string, corpus: Set<string>): string | undefined {
  const stem = path.replace(/(\.d\.ts|\.tsx)$/, "");
  if (stem === path) return undefined;
  const rivals = path.endsWith(".d.ts") ? [".ts", ".tsx"] : [".ts"];
  return rivals.map((ext) => stem + ext).find((twin) => corpus.has(twin));
}

describe("the frontend's tsconfig projects", () => {
  let files: string[] = [];
  let built = new Map<string, ts.ParsedCommandLine>();
  let builtPaths: string[] = [];
  const inFrontend = (pattern: RegExp) =>
    files.filter((path) => FRONTEND_PATH.test(path) && pattern.test(path));

  beforeAll(() => {
    files = repoFiles();
    built = builtConfigs(files);
    builtPaths = [...built.keys()].map((path) => relative(repo, path));
  });

  it("are each compiled by a lane", () => {
    // A config listing that came back short would pass vacuously.
    expect(inFrontend(TSCONFIG)).toContain("frontend/tsconfig.json");
    const unbuilt = inFrontend(TSCONFIG)
      .filter((path) => !builtPaths.includes(path))
      .map(
        (path) =>
          `${path} — reference it from frontend/tsconfig.json or name it in a lane's tsc -p, or delete it`,
      );
    expect(
      unbuilt,
      `no lane compiles these configs, so what they include is typechecked by nothing:\n${unbuilt.join("\n")}`,
    ).toEqual([]);
  });

  it("list every frontend TypeScript file as a root", () => {
    const corpus = inFrontend(TYPESCRIPT_FILE);
    expect(corpus).toContain("frontend/scripts/tsconfig-roots.test.ts");
    const roots = new Set(
      [...built.values()].flatMap((project) => project.fileNames),
    );
    const known = new Set(corpus);
    const untyped = corpus
      .filter((path) => !roots.has(join(repo, path)))
      .map((path) => {
        const twin = shadowedBy(path, known);
        return twin === undefined
          ? `${path} — add it to the include of a built project that owns its kind (tsconfig.node.json for tool-side files)`
          : `${path} — dropped because ${twin} shares its basename; rename one of them`;
      });
    expect(
      untyped,
      `no built tsconfig project lists these files, so nothing typechecks them:\n${untyped.join("\n")}`,
    ).toEqual([]);
  });
});
