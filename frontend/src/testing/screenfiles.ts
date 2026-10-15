import { existsSync, readdirSync } from "node:fs";
import { dirname, join, relative, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";
import { stable } from "../format/collate";

/** The frontend's `src/`, which every path below is relative to. */
export const SRC_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), "..");

// The folders screen code can live in. A guard that read screens/ alone would
// lose a part moved into components/ or a feature folder and still pass.
export const SCREEN_ROOTS = ["screens", "components", "features"];

// screenFiles lists every `.tsx` under the screen folders at any depth, minus
// tests and stories, as paths relative to `src/` with forward slashes.
export function screenFiles(): string[] {
  return SCREEN_ROOTS.flatMap((root) => filesUnder(join(SRC_ROOT, root)))
    .filter((path) => path.endsWith(".tsx"))
    .filter((path) => !/\.(test|stories)\.tsx$/.test(path))
    .map((path) => relative(SRC_ROOT, path).split(sep).join("/"))
    .sort(stable);
}

function filesUnder(dir: string): string[] {
  // features/ does not exist until the first feature moves into it.
  if (!existsSync(dir)) return [];
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    return entry.isDirectory() ? filesUnder(path) : [path];
  });
}
