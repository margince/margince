// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Every TypeScript file the repository keeps under a frontend is a ROOT of some
// tsconfig project. vitest, Storybook and Playwright transpile without
// typechecking, so a file no project lists runs green whatever its types say.
//
// Roots, not files a program reaches: a test is imported by nothing, so
// "reached" would pass exactly the files this exists to find. They come from
// TypeScript's own parse of each config, which applies include, exclude and the
// rule that drops `x.test.tsx` when `x.test.ts` matches the same include — so no
// program is built. The projects are every tsconfig*.json on disk, and a
// declaration file is held like any other: one no project lists declares nothing.

import { execFileSync } from "node:child_process";
import { existsSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";

const repo = resolve(dirname(fileURLToPath(import.meta.url)), "..", "..");

const TYPESCRIPT_FILE = /\.(ts|tsx|mts|cts)$/;
// The core frontend, and every extension's frontend layer at any depth.
const FRONTEND_PATH = /^(frontend|extensions\/(.+\/)?frontend)\//;

// Tracked files and new ones git would stage, so a file is caught before its
// first commit; .gitignore keeps build output out.
function frontendFiles(pattern: RegExp): string[] {
  const listed = execFileSync(
    "git",
    ["ls-files", "-z", "--cached", "--others", "--exclude-standard"],
    { cwd: repo, encoding: "utf8" },
  );
  return [...new Set(listed.split("\0"))]
    .filter((path) => FRONTEND_PATH.test(path) && pattern.test(path))
    .filter((path) => existsSync(join(repo, path)))
    .sort();
}

const messageOf = (diagnostic: ts.Diagnostic) =>
  ts.flattenDiagnosticMessageText(diagnostic.messageText, "\n");

function projectRoots(configPath: string): string[] {
  const parsed = ts.getParsedCommandLineOfConfigFile(configPath, undefined, {
    ...ts.sys,
    onUnRecoverableConfigFileDiagnostic: (diagnostic) => {
      throw new Error(messageOf(diagnostic));
    },
  });
  if (parsed === undefined) throw new Error(`${configPath} did not parse`);
  const errors = parsed.errors.map(messageOf);
  if (errors.length > 0) throw new Error(`${configPath}: ${errors.join("; ")}`);
  return parsed.fileNames;
}

// Names the sibling that shadows `path`, because the fix for a dropped
// basename twin is a rename, not an include.
function shadowedBy(path: string, corpus: Set<string>): string | undefined {
  const twin = path.replace(/\.tsx$/, ".ts");
  return twin !== path && corpus.has(twin) ? twin : undefined;
}

describe("the tsconfig projects", () => {
  it("list every frontend TypeScript file as a root", () => {
    const configs = frontendFiles(/(^|\/)tsconfig[^/]*\.json$/);
    const corpus = frontendFiles(TYPESCRIPT_FILE);
    // A listing that came back short would pass vacuously, so it must at
    // least reach this file and the configs that sit beside it.
    expect(corpus).toContain("frontend/scripts/tsconfig-roots.test.ts");
    expect(configs).toContain("frontend/tsconfig.node.json");

    const roots = new Set(
      configs.flatMap((config) => projectRoots(join(repo, config))),
    );
    const known = new Set(corpus);
    const untyped = corpus
      .filter((path) => !roots.has(join(repo, path)))
      .map((path) => {
        const twin = shadowedBy(path, known);
        return twin === undefined
          ? `${path} — add it to the include of the project that owns its kind (tsconfig.node.json for tool-side files)`
          : `${path} — dropped because ${twin} shares its basename; rename one of them`;
      });

    expect(
      untyped,
      `no tsconfig project lists these files, so nothing typechecks them:\n${untyped.join("\n")}`,
    ).toEqual([]);
  });
});
