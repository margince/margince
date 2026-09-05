import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

// `settingscatalog.ts` spells `UnitSecretScope` a second time rather than
// importing it, because the extensions registry reaches `@composition/screens`
// and would drag React and a build alias into a module whose whole purpose is
// being importable from anywhere — including a plain node script and a test with
// no alias configured.
//
// A mirrored type needs a gate that fails in BOTH directions, or the copy drifts
// silently: a scope added to the registry and not here makes a page's units arm
// unreachable, and one added here and not there makes it unsatisfiable. Neither
// shows up as a type error, because the two are separate declarations.
//
// Compared as source text rather than as types: a type identity cannot be
// asserted at runtime, and the point is precisely that these are two
// declarations rather than one.

const here = dirname(fileURLToPath(import.meta.url));

function scopesDeclaredIn(file: string, name: string): string[] {
  const source = readFileSync(file, "utf8");
  const match = source.match(new RegExp(`export type ${name} =([^;]+);`));
  if (match === null) {
    throw new Error(`no \`export type ${name}\` in ${file}`);
  }
  return [...match[1].matchAll(/"([a-z_]+)"/g)].map((m) => m[1]).sort();
}

describe("the mirrored UnitSecretScope", () => {
  it("names exactly the scopes the registry names", () => {
    const registry = scopesDeclaredIn(
      join(here, "..", "app", "extensions.ts"),
      "UnitSecretScope",
    );
    const catalog = scopesDeclaredIn(
      join(here, "settingscatalog.ts"),
      "UnitSecretScope",
    );
    expect(catalog).toEqual(registry);
  });

  it("finds a non-empty set in each, so the comparison cannot pass vacuously", () => {
    // Two empty arrays are equal. Without this, a regex that stopped matching
    // would report agreement between nothing and nothing.
    expect(
      scopesDeclaredIn(
        join(here, "..", "app", "extensions.ts"),
        "UnitSecretScope",
      ),
    ).toContain("workspace");
    expect(
      scopesDeclaredIn(join(here, "settingscatalog.ts"), "UnitSecretScope"),
    ).toContain("workspace");
  });
});
