import { existsSync, readdirSync } from "node:fs";
import { dirname, join, relative, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";
import { stable } from "../format/collate";

/** The frontend's `src/`, which every path below is relative to. */
export const SRC_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), "..");

// The folders screen code can live in: the shared parts, a fork's screens and
// the features. A guard with a shorter list would miss some and still pass.
export const SCREEN_ROOTS = ["components", "custom", "features"];

// screenFiles lists every `.tsx` under the screen folders at any depth, minus
// tests, testkits and stories, as paths relative to `src/` with forward slashes.
export function screenFiles(): string[] {
  return SCREEN_ROOTS.flatMap((root) => filesUnder(join(SRC_ROOT, root)))
    .filter((path) => path.endsWith(".tsx"))
    .filter((path) => !/\.(test|stories|testkit)\.tsx$/.test(path))
    .map((path) => relative(SRC_ROOT, path).split(sep).join("/"))
    .sort(stable);
}

function filesUnder(dir: string): string[] {
  // A root that is gone would read as a smaller tree and pass every guard.
  if (!existsSync(dir)) {
    throw new Error(`${dir} does not exist; update SCREEN_ROOTS`);
  }
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    return entry.isDirectory() ? filesUnder(path) : [path];
  });
}
