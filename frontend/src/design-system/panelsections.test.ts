// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

// A record tab past Overview draws its plain panels as flat sections, and a
// toned panel keeps its box. The list of tones to keep is read from the rules
// that declare a tone, so a new tone is never flattened by being missed.

const sheet = readFileSync(
  join(dirname(fileURLToPath(import.meta.url)), "panel.css"),
  "utf8",
);

// Every panel class that sets a tone: its band, or the AI ground.
function toneClasses(): string[] {
  const banded = [...sheet.matchAll(/\.(panel-[a-z]+) \{\s*--panelToneEdge/g)];
  return [...banded.map((match) => match[1] ?? ""), "panel-ai"];
}

// The `:not(...)` list of each record-section rule, as written.
function sectionExclusions(): string[] {
  const start = sheet.indexOf("A record tab past Overview");
  return [...sheet.slice(start).matchAll(/\.panel:not\(([^)]*)\)/g)].map(
    (match) => match[1] ?? "",
  );
}

describe("record tab sections", () => {
  it("find every toned panel class in panel.css", () => {
    expect(toneClasses().length).toBeGreaterThan(5);
  });

  it("leave every toned panel its box, in each of their rules", () => {
    const lists = sectionExclusions();
    expect(lists).toHaveLength(3);
    for (const list of lists) {
      const kept = list.split(",").map((part) => part.trim());
      for (const tone of toneClasses()) {
        expect(kept).toContain(`.${tone}`);
      }
    }
  });
});
