import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { LANGUAGE_DEPENDENT_QUERY_PREFIXES } from "./aicaches";

/**
 * A list of caches to invalidate is a thing to forget to add to, and forgetting
 * is the whole failure it prevents: a new model-written surface would render
 * its old language after the setting changed, silently, on a page the admin is
 * looking at while they change it.
 *
 * So this derives the candidates from the tree instead. Every query key whose
 * name reads like a model-written surface must be named in the list, or say why
 * it is not one.
 */

const SCREENS = join(__dirname, "..", "screens");

/**
 * The names that mark a surface as model-written. Matched against the FIRST
 * segment of a query key, which is what identifies the resource — the segments
 * after it are ids.
 */
const MODEL_WRITTEN = /^(.*brief|.*dossier|.*growth-fit|deal-status)$/i;

/**
 * Query keys that match the name pattern and are NOT model-written prose, each
 * with the reason. A bare exclusion list would let somebody silence a real
 * finding by adding a line, so each entry has to say what the surface returns.
 */
const NOT_PROSE: Record<string, string> = {};

// Walks INTO subdirectories, because screens have them: record360/ and
// meetingbrief/ both hold components that query. A reader that stopped at the
// top level saw a smaller tree, reported PASS, and had no failing assertion to
// notice — which is the one way a census must not break.
function everyQueryKeyFirstSegment(
  dir: string = SCREENS,
  prefix = "",
): {
  key: string;
  file: string;
}[] {
  const found: { key: string; file: string }[] = [];
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const name = entry.name;
    if (entry.isDirectory()) {
      found.push(
        ...everyQueryKeyFirstSegment(join(dir, name), `${prefix}${name}/`),
      );
      continue;
    }
    if (!name.endsWith(".tsx") && !name.endsWith(".ts")) continue;
    if (name.endsWith(".test.tsx") || name.endsWith(".test.ts")) continue;
    const source = readFileSync(join(dir, name), "utf8");
    // `queryKey: ["name", …]` — the first segment only, which is the one that
    // names the resource.
    for (const match of source.matchAll(/queryKey:\s*\[\s*"([^"]+)"/g)) {
      found.push({ key: match[1], file: `${prefix}${name}` });
    }
  }
  return found;
}

describe("the language-dependent cache list", () => {
  it("names every model-written surface the screens query", () => {
    const named = new Set<string>(
      LANGUAGE_DEPENDENT_QUERY_PREFIXES.map((prefix) => prefix[0]),
    );
    const missing = everyQueryKeyFirstSegment().filter(
      ({ key }) =>
        MODEL_WRITTEN.test(key) && !named.has(key) && !(key in NOT_PROSE),
    );

    expect(
      missing,
      `these query keys look like model-written surfaces and are not in LANGUAGE_DEPENDENT_QUERY_PREFIXES, ` +
        `so changing the installation's base language would leave them rendering the old one: ` +
        missing.map((m) => `${m.key} (${m.file})`).join(", "),
    ).toEqual([]);
  });

  it("does not name a surface the screens never query", () => {
    // The other direction. A prefix nobody queries invalidates nothing, and
    // reads as coverage that is not there — which is worse than an obviously
    // short list, because it looks complete.
    const queried = new Set(everyQueryKeyFirstSegment().map(({ key }) => key));
    const stale = LANGUAGE_DEPENDENT_QUERY_PREFIXES.map(
      (prefix) => prefix[0],
    ).filter((key) => !queried.has(key));

    expect(
      stale,
      `these prefixes are invalidated but no screen queries them: ${stale.join(", ")}`,
    ).toEqual([]);
  });
});
