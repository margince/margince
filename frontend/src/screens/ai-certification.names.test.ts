// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import { en } from "../i18n/en";

// The card names every AI job in plain words, and a name only reaches a reader
// if it exists in BOTH places: the locale catalogue, and the lookup map in
// ai-certification.tsx that turns a task id into a catalogue key.
//
// backend/gates/aicertificationnames_test.go derives the required set from the
// shipped-site census and checks the catalogue. It cannot see the map — so a
// job added to the catalogue but forgotten in the map passes that gate, passes
// i18n parity, and is silently dropped into "N newer jobs this version cannot
// name" on a build that DOES have the wording. This closes the other half.
//
// Read as text rather than imported, because the maps are module-private: the
// point is that the two lists agree, and exporting them for a test would invite
// a third caller to reach for the wrong one.
const source = readFileSync(
  new URL("./ai-certification.tsx", import.meta.url),
  "utf8",
);

function mapKeys(mapName: string): Set<string> {
  const block = source.slice(source.indexOf(`const ${mapName}`));
  const body = block.slice(0, block.indexOf("};"));
  // Match the catalogue key each entry POINTS AT, not the entry's own left-hand
  // side: the map's job is to be the one place a task id becomes a key, and it
  // is the key that has to exist.
  const keys = new Set<string>();
  for (const match of body.matchAll(/"(aiCert\.(?:job|site)\.[a-z0-9_.]+)"/g)) {
    keys.add(match[1]);
  }
  expect(
    keys.size,
    `${mapName} yielded no keys — the pattern has stopped
    matching the map, which would let this test pass while every name was
    missing`,
  ).toBeGreaterThan(0);
  return keys;
}

function catalogueKeys(prefix: string): Set<string> {
  return new Set(Object.keys(en).filter((key) => key.startsWith(prefix)));
}

describe("the certification card's job names", () => {
  it("maps exactly the job names the catalogue carries", () => {
    const mapped = mapKeys("JOB_NAME");
    const catalogued = catalogueKeys("aiCert.job.");

    // Both directions. A catalogue key with no map entry is a job whose wording
    // exists and never renders; a map entry with no catalogue key is a lookup
    // that resolves to nothing.
    expect([...catalogued].filter((key) => !mapped.has(key))).toEqual([]);
    expect([...mapped].filter((key) => !catalogued.has(key))).toEqual([]);
  });

  it("maps exactly the site names the catalogue carries", () => {
    const mapped = mapKeys("SITE_NAME");
    const catalogued = catalogueKeys("aiCert.site.");

    expect([...catalogued].filter((key) => !mapped.has(key))).toEqual([]);
    expect([...mapped].filter((key) => !catalogued.has(key))).toEqual([]);
  });
});
