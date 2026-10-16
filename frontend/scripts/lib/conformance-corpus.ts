import { readdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { extensionFilesMatching } from "./source-tree";

// The files the design-system conformance tests read: src/, index.html and
// every unit's frontend.

export const frontendRoot = join(
  dirname(fileURLToPath(import.meta.url)),
  "..",
  "..",
);

export function sourceFiles(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) {
      return entry.name === "node_modules" || entry.name === "dist"
        ? []
        : sourceFiles(path);
    }
    return /\.(css|tsx?|html)$/.test(entry.name) ? [path] : [];
  });
}

// A unit's screen is held to the same rules as core, so it is enrolled the same
// way. The shell gates beside this file already sweep the tier, but they are
// greps. They cannot see the inline camelCase spelling of a type family; only
// this corpus can. Without the tier, this census fails short: it reads a
// smaller tree, reports a pass, and nothing asserts.
export function extensionFrontends(): string[] {
  return extensionFilesMatching(
    join(frontendRoot, "..", "extensions"),
    /\.(css|tsx?|html)$/,
  );
}

export const files = sourceFiles(join(frontendRoot, "src"))
  .concat(join(frontendRoot, "index.html"))
  .concat(extensionFrontends());

// Every gate below reads the whole source tree, and most parse it as
// TypeScript. So each one costs more as the source grows and the runner gets
// busier. Vitest's 5s per-test default is sized for a unit test, not a repo
// sweep, and it left no margin. The heaviest leg takes ~1.1s locally under the
// coverage instrumentation `fe-unit` runs with. It has already passed 5s on a
// loaded CI runner. That job is named "vitest + coverage", so it read as a
// coverage failure. The budget below is ten times that worst observed cost,
// because the tree only grows. These scans are synchronous file I/O, so there
// is no hang for a tight timeout to catch, and a generous one costs nothing.
// It is declared on the suite so every gate this file grows inherits it.
export const scanBudget = { timeout: 60_000 };
