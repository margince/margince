// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readdirSync, readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

// Fitness function for a claim that refreshes a list nobody is reading.
//
// `useClaimRecord` invalidates the list its record is filed under. It used to
// derive that key as `${recordType}s`, which is right for three of the four
// claimable kinds and wrong for the one whose plural is not its singular plus
// an s: contacts are filed under "contacts", so claiming a contact asked React
// Query to invalidate "contacts".
//
// Nothing failed. `invalidateQueries` matching no cache is not an error — it
// finds nothing and returns. So the claim was written, the control settled,
// and the list in front of the reader went on showing the record as unowned
// until some other event happened to refetch it. A refresh that silently
// refreshes nothing is the shape this holds.
//
// The census reads the KEYS the module declares and looks for each one in the
// screens that actually query, rather than pinning the four names here. A
// fifth claimable kind is therefore covered the day it is added, and a key
// renamed on the reading side fails here rather than going quiet.

const here = dirname(fileURLToPath(import.meta.url));

function declaredListKeys(): string[] {
  const source = readFileSync(resolve(here, "claimrecord.ts"), "utf8");
  const table =
    /const LIST_KEY: Record<ClaimableRecordType, string> = \{([^}]*)\}/s.exec(
      source,
    );
  if (!table) {
    throw new Error(
      "claimrecord.ts no longer declares LIST_KEY as an object literal — this census reads it textually",
    );
  }
  return [...table[1].matchAll(/"([^"]+)"/g)].map((m) => m[1]);
}

// Every .ts/.tsx under src, because the reader of a list key is not always a
// screen: a rail, a hook or an app-level prefetch files under the same name.
function sourcesUnder(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const full = resolve(dir, entry.name);
    if (entry.isDirectory()) return sourcesUnder(full);
    if (!/\.tsx?$/.test(entry.name) || /\.d\.ts$/.test(entry.name)) return [];
    return [full];
  });
}

describe("the key a claim invalidates is a key something reads", () => {
  const keys = declaredListKeys();
  const corpus = sourcesUnder(resolve(here, ".."))
    .filter(
      (f) =>
        !f.endsWith("claimrecord.ts") &&
        !f.endsWith("claim-list-key-coverage.test.ts"),
    )
    .map((f) => readFileSync(f, "utf8"))
    .join("\n");

  it("declares one key per claimable kind", () => {
    expect(keys.length).toBeGreaterThanOrEqual(4);
  });

  it.each(keys)("%s is queried somewhere", (key) => {
    expect(corpus).toContain(`"${key}"`);
  });
});
